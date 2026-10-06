package service

import (
	"encoding/json"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type RunVesselView struct {
	Run    RunView    `json:"run"`
	Vessel VesselView `json:"vessel"`
}

type FormulaView struct {
	domain.Formula
	CustomFields []domain.CustomFieldView `json:"custom_fields"`
}

type VesselView struct {
	domain.Vessel
	CustomFields []domain.CustomFieldView `json:"custom_fields"`
}

type RunView struct {
	domain.FermentationRun
	CustomFields []domain.CustomFieldView `json:"custom_fields"`
}

func (v FormulaView) MarshalJSON() ([]byte, error) {
	return marshalEntityWithCustomFields(v.Formula, v.CustomFields)
}

func (v VesselView) MarshalJSON() ([]byte, error) {
	return marshalEntityWithCustomFields(v.Vessel, v.CustomFields)
}

func (v RunView) MarshalJSON() ([]byte, error) {
	return marshalEntityWithCustomFields(v.FermentationRun, v.CustomFields)
}

func marshalEntityWithCustomFields(entity any, customFields []domain.CustomFieldView) ([]byte, error) {
	encoded, err := json.Marshal(entity)
	if err != nil {
		return nil, err
	}
	var values map[string]any
	if err := json.Unmarshal(encoded, &values); err != nil {
		return nil, err
	}
	delete(values, "custom_values")
	if customFields == nil {
		customFields = []domain.CustomFieldView{}
	}
	values["custom_fields"] = customFields
	return json.Marshal(values)
}

func formulaView(snapshot domain.Snapshot, formula domain.Formula) FormulaView {
	return FormulaView{
		Formula: formula,
		CustomFields: domain.BuildCustomFieldViews(
			domain.ExtensionEntityFormula, snapshot.ExtensionFields, formula.CustomValues),
	}
}

func vesselView(snapshot domain.Snapshot, vessel domain.Vessel) VesselView {
	return VesselView{
		Vessel: vessel,
		CustomFields: domain.BuildCustomFieldViews(
			domain.ExtensionEntityVessel, snapshot.ExtensionFields, vessel.CustomValues),
	}
}

func runView(snapshot domain.Snapshot, run domain.FermentationRun) RunView {
	return RunView{
		FermentationRun: run,
		CustomFields: domain.BuildCustomFieldViews(
			domain.ExtensionEntityRun, snapshot.ExtensionFields, run.CustomValues),
	}
}
