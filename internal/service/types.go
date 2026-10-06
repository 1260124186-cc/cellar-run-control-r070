package service

import (
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type CreateFormulaInput struct {
	Name                  string         `json:"name"`
	Style                 string         `json:"style"`
	TargetOriginalGravity float64        `json:"target_original_gravity"`
	TargetFinalGravity    float64        `json:"target_final_gravity"`
	MaxFermentationTempC  float64        `json:"max_fermentation_temp_c"`
	MinimumDays           int            `json:"minimum_days"`
	MaximumDays           int            `json:"maximum_days"`
	CustomFields          map[string]any `json:"custom_fields"`
}

type SetFormulaStepsInput struct {
	ExpectedVersion *int64
	Steps           []domain.FormulaStep
}

type VersionInput struct {
	ExpectedVersion *int64
}

type CreateVesselInput struct {
	Code         string
	CapacityL    float64
	CustomFields map[string]any
}

type CreateRunInput struct {
	Code           string
	FormulaID      string
	PlannedVolumeL float64
	Notes          string
	CustomFields   map[string]any
}

type PatchCustomFieldsInput struct {
	ExpectedVersion *int64
	Fields          map[string]*any
}

type CreateExtensionFieldInput struct {
	EntityType domain.EntityKind
	Name       string
	Type       domain.FieldType
	Required   bool
}

type UpdateExtensionFieldInput struct {
	ExpectedRevision *int64
	Name             string
	Type             domain.FieldType
	Required         bool
}

type ReserveRunInput struct {
	ExpectedVersion *int64
	VesselID        string
}

type ObservationInput struct {
	Sequence     int
	Gravity      float64
	TemperatureC float64
	ObservedAt   time.Time
	Note         string
}

type FinishCleaningInput struct {
	ExpectedVersion *int64
}
