package domain

import "time"

type Snapshot struct {
	Revision         int64                      `json:"revision"`
	UpdatedAt        time.Time                  `json:"updated_at"`
	Formulas         map[string]Formula         `json:"formulas"`
	Vessels          map[string]Vessel          `json:"vessels"`
	Runs             map[string]FermentationRun `json:"runs"`
	ExtensionFields  map[string]ExtensionField  `json:"extension_fields"`
	NameIndex        map[string]string          `json:"formula_name_index"`
	VesselIndex      map[string]string          `json:"vessel_code_index"`
	RunIndex         map[string]string          `json:"run_code_index"`
	ExtensionNameIdx map[string]string          `json:"extension_field_name_index"`
}

func EmptySnapshot() Snapshot {
	return Snapshot{
		Revision:         0,
		Formulas:         map[string]Formula{},
		Vessels:          map[string]Vessel{},
		Runs:             map[string]FermentationRun{},
		ExtensionFields:  map[string]ExtensionField{},
		NameIndex:        map[string]string{},
		VesselIndex:      map[string]string{},
		RunIndex:         map[string]string{},
		ExtensionNameIdx: map[string]string{},
	}
}

func (s Snapshot) Clone() Snapshot {
	cloned := EmptySnapshot()
	cloned.Revision = s.Revision
	cloned.UpdatedAt = s.UpdatedAt
	for key, value := range s.Formulas {
		cloned.Formulas[key] = value.Clone()
	}
	for key, value := range s.Vessels {
		cloned.Vessels[key] = value.Clone()
	}
	for key, value := range s.Runs {
		cloned.Runs[key] = value.Clone()
	}
	for key, value := range s.ExtensionFields {
		if value.DeactivatedAt != nil {
			stamp := *value.DeactivatedAt
			value.DeactivatedAt = &stamp
		}
		cloned.ExtensionFields[key] = value
	}
	for key, value := range s.NameIndex {
		cloned.NameIndex[key] = value
	}
	for key, value := range s.VesselIndex {
		cloned.VesselIndex[key] = value
	}
	for key, value := range s.RunIndex {
		cloned.RunIndex[key] = value
	}
	for key, value := range s.ExtensionNameIdx {
		cloned.ExtensionNameIdx[key] = value
	}
	return cloned
}

func (s Snapshot) ExtensionFieldHasValues(fieldID string) bool {
	for _, formula := range s.Formulas {
		if _, exists := formula.CustomValues[fieldID]; exists {
			return true
		}
	}
	for _, vessel := range s.Vessels {
		if _, exists := vessel.CustomValues[fieldID]; exists {
			return true
		}
	}
	for _, run := range s.Runs {
		if _, exists := run.CustomValues[fieldID]; exists {
			return true
		}
	}
	return false
}
