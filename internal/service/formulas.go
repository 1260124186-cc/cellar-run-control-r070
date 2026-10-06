package service

import (
	"sort"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateFormula(input CreateFormulaInput) (FormulaView, error) {
	now := s.clock.Now()
	formula := domain.Formula{
		Name:                  input.Name,
		Style:                 input.Style,
		TargetOriginalGravity: input.TargetOriginalGravity,
		TargetFinalGravity:    input.TargetFinalGravity,
		MaxFermentationTempC:  input.MaxFermentationTempC,
		MinimumDays:           input.MinimumDays,
		MaximumDays:           input.MaximumDays,
		Steps:                 []domain.FormulaStep{},
		CustomValues:          domain.CloneCustomValues(input.CustomValues),
		State:                 domain.FormulaDraft,
		Version:               1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	formula.Normalize()
	if err := formula.Validate(); err != nil {
		return FormulaView{}, err
	}
	id, err := generateEntityID("formula")
	if err != nil {
		return FormulaView{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	formula.ID = id
	nameKey := domain.NameKey(formula.Name)

	var created FormulaView
	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if _, exists := snapshot.NameIndex[nameKey]; exists {
			return domain.NewError(domain.CodeFormulaNameConflict,
				"a formula with the same normalized name already exists")
		}
		if err := domain.ValidateCustomValues(domain.ExtensionEntityFormula,
			snapshot.ExtensionFields, formula.CustomValues, true); err != nil {
			return err
		}
		snapshot.Formulas[formula.ID] = formula.Clone()
		snapshot.NameIndex[nameKey] = formula.ID
		created = formulaView(*snapshot, formula.Clone())
		return nil
	})
	if err != nil {
		return FormulaView{}, err
	}
	return created, nil
}

func (s *Service) SetFormulaSteps(
	id string,
	input SetFormulaStepsInput,
) (FormulaView, error) {
	var updated FormulaView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.SetSteps(input.Steps); err != nil {
			return err
		}
		formula.UpdatedAt = s.clock.Now()
		snapshot.Formulas[id] = formula
		updated = formulaView(*snapshot, formula.Clone())
		return nil
	})
	if err != nil {
		return FormulaView{}, err
	}
	return updated, nil
}

func (s *Service) UpdateFormulaCustomFields(
	id string,
	input UpdateCustomFieldsInput,
) (FormulaView, error) {
	var updated FormulaView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		merged := domain.MergeCustomValues(formula.CustomValues, input.Values)
		if err := domain.ValidateCustomValues(domain.ExtensionEntityFormula,
			snapshot.ExtensionFields, merged, true); err != nil {
			return err
		}
		if err := formula.UpdateCustomValues(merged, s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formulaView(*snapshot, formula.Clone())
		return nil
	})
	if err != nil {
		return FormulaView{}, err
	}
	return updated, nil
}

func (s *Service) ApproveFormula(id string, input VersionInput) (FormulaView, error) {
	var updated FormulaView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := formula.Approve(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formulaView(*snapshot, formula.Clone())
		return nil
	})
	if err != nil {
		return FormulaView{}, err
	}
	return updated, nil
}

func (s *Service) RetireFormula(id string, input VersionInput) (FormulaView, error) {
	var updated FormulaView
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		if err := s.checkExpected(formula.Version, input.ExpectedVersion); err != nil {
			return err
		}
		for _, run := range snapshot.Runs {
			if run.FormulaID == id && run.State != domain.RunCompleted &&
				run.State != domain.RunAborted {
				return domain.NewError(domain.CodeFormulaInUse,
					"formula is attached to a run that is still open")
			}
		}
		if err := formula.Retire(s.clock.Now()); err != nil {
			return err
		}
		snapshot.Formulas[id] = formula
		updated = formulaView(*snapshot, formula.Clone())
		return nil
	})
	if err != nil {
		return FormulaView{}, err
	}
	return updated, nil
}

func (s *Service) GetFormula(id string) (FormulaView, error) {
	var found FormulaView
	err := s.store.View(func(snapshot domain.Snapshot) error {
		formula, ok := snapshot.Formulas[id]
		if !ok {
			return domain.NewError(domain.CodeFormulaNotFound, "formula was not found")
		}
		found = formulaView(snapshot, formula.Clone())
		return nil
	})
	if err != nil {
		return FormulaView{}, err
	}
	return found, nil
}

func (s *Service) ListFormulas() ([]FormulaView, error) {
	var formulas []FormulaView
	err := s.store.View(func(snapshot domain.Snapshot) error {
		formulas = make([]FormulaView, 0, len(snapshot.Formulas))
		for _, formula := range snapshot.Formulas {
			formulas = append(formulas, formulaView(snapshot, formula.Clone()))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(formulas, func(i, j int) bool {
		if formulas[i].Name == formulas[j].Name {
			return formulas[i].ID < formulas[j].ID
		}
		return formulas[i].Name < formulas[j].Name
	})
	return formulas, nil
}

func timestampPointer(value time.Time) *time.Time {
	copy := value.UTC()
	return &copy
}
