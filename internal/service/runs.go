package service

import (
	"sort"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateRun(input CreateRunInput) (RunView, error) {
	now := s.clock.Now()
	code := domain.NormalizeRunCode(input.Code)
	if code == "" {
		generated, err := generatedRunCode()
		if err != nil {
			return RunView{}, domain.NewError(domain.CodeStorage, err.Error())
		}
		code = generated
	}
	run := domain.FermentationRun{
		Code:           code,
		FormulaID:      input.FormulaID,
		PlannedVolumeL: input.PlannedVolumeL,
		Notes:          input.Notes,
		CustomValues:   domain.CloneCustomValues(input.CustomValues),
		State:          domain.RunPlanned,
		Observations:   []domain.GravityObservation{},
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := run.Validate(); err != nil {
		return RunView{}, err
	}
	id, err := generateEntityID("run")
	if err != nil {
		return RunView{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	run.ID = id

	var created RunView
	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[run.FormulaID]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if formula.State != domain.FormulaApproved {
			return domain.NewError(domain.CodeFormulaState,
				"run requires an approved formula")
		}
		if _, exists := snapshot.RunIndex[run.Code]; exists {
			return domain.NewError(domain.CodeRunConflict,
				"a run with the same code already exists")
		}
		if err := domain.ValidateCustomValues(domain.ExtensionEntityRun,
			snapshot.ExtensionFields, run.CustomValues, true); err != nil {
			return err
		}
		snapshot.Runs[run.ID] = run.Clone()
		snapshot.RunIndex[run.Code] = run.ID
		created = runView(*snapshot, run.Clone())
		return nil
	})
	if err != nil {
		return RunView{}, err
	}
	return created, nil
}

func (s *Service) UpdateRunCustomFields(
	id string,
	input UpdateCustomFieldsInput,
) (RunView, error) {
	var updated RunView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		if err := s.checkExpected(run.Version, input.ExpectedVersion); err != nil {
			return err
		}
		merged := domain.MergeCustomValues(run.CustomValues, input.Values)
		if err := domain.ValidateCustomValues(domain.ExtensionEntityRun,
			snapshot.ExtensionFields, merged, true); err != nil {
			return err
		}
		if err := run.UpdateCustomValues(merged, s.clock.Now()); err != nil {
			return err
		}
		snapshot.Runs[id] = run
		updated = runView(*snapshot, run.Clone())
		return nil
	})
	if err != nil {
		return RunView{}, err
	}
	return updated, nil
}

func (s *Service) ReserveRun(
	id string,
	input ReserveRunInput,
) (RunVesselView, error) {
	var updated RunVesselView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		if err := s.checkExpected(run.Version, input.ExpectedVersion); err != nil {
			return err
		}
		vessel, ok := snapshot.Vessels[input.VesselID]
		if !ok {
			return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		if run.PlannedVolumeL > vessel.CapacityL {
			return domain.FieldError(domain.CodeInvalidInput,
				"planned_volume_liters", "exceeds vessel capacity")
		}
		if err := vessel.Reserve(run.ID, s.clock.Now()); err != nil {
			return err
		}
		if err := run.ReserveVessel(vessel.ID, s.clock.Now()); err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		snapshot.Vessels[vessel.ID] = vessel
		updated = RunVesselView{
			Run:    runView(*snapshot, run.Clone()),
			Vessel: vesselView(*snapshot, vessel.Clone()),
		}
		return nil
	})
	if err != nil {
		return RunVesselView{}, err
	}
	return updated, nil
}

func (s *Service) StartRun(
	id string,
	input VersionInput,
) (RunVesselView, error) {
	var updated RunVesselView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		if err := s.checkExpected(run.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if run.VesselID == nil {
			return domain.NewError(domain.CodeRunState, "run has no vessel")
		}
		vessel, ok := snapshot.Vessels[*run.VesselID]
		if !ok {
			return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
		}
		if err := vessel.StartRun(run.ID, s.clock.Now()); err != nil {
			return err
		}
		if err := run.StartFermentation(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Runs[id] = run.Clone()
		snapshot.Vessels[vessel.ID] = vessel
		updated = RunVesselView{
			Run:    runView(*snapshot, run.Clone()),
			Vessel: vesselView(*snapshot, vessel.Clone()),
		}
		return nil
	})
	if err != nil {
		return RunVesselView{}, err
	}
	return updated, nil
}

func (s *Service) AbortRun(
	id string,
	input VersionInput,
) (RunVesselView, error) {
	var updated RunVesselView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		if err := s.checkExpected(run.Version, input.ExpectedVersion); err != nil {
			return err
		}
		hadStarted := run.HadStarted()
		if err := run.Abort(s.clock.Now()); err != nil {
			return err
		}
		var updatedVessel VesselView
		if run.VesselID != nil {
			vessel, ok := snapshot.Vessels[*run.VesselID]
			if !ok {
				return domain.NewError(domain.CodeVesselNotFound, "vessel was not found")
			}
			if hadStarted {
				if err := vessel.StartCleaning(run.ID, s.clock.Now()); err != nil {
					return err
				}
			} else if err := vessel.ReleaseReservation(run.ID, s.clock.Now()); err != nil {
				return err
			}
			snapshot.Vessels[vessel.ID] = vessel
			updatedVessel = vesselView(*snapshot, vessel.Clone())
		}
		snapshot.Runs[id] = run.Clone()
		updated = RunVesselView{
			Run:    runView(*snapshot, run.Clone()),
			Vessel: updatedVessel,
		}
		return nil
	})
	if err != nil {
		return RunVesselView{}, err
	}
	return updated, nil
}

func (s *Service) GetRun(id string) (RunView, error) {
	var found RunView
	err := s.store.View(func(snapshot domain.Snapshot) error {
		run, ok := snapshot.Runs[id]
		if !ok {
			return domain.NewError(domain.CodeRunNotFound, "run was not found")
		}
		found = runView(snapshot, run.Clone())
		return nil
	})
	if err != nil {
		return RunView{}, err
	}
	return found, nil
}

func (s *Service) ListRuns() ([]RunView, error) {
	var runs []RunView
	err := s.store.View(func(snapshot domain.Snapshot) error {
		runs = make([]RunView, 0, len(snapshot.Runs))
		for _, run := range snapshot.Runs {
			runs = append(runs, runView(snapshot, run.Clone()))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].Code == runs[j].Code {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].Code < runs[j].Code
	})
	return runs, nil
}
