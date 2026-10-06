package service

import (
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type CreateFormulaInput struct {
	Name                  string
	Style                 string
	TargetOriginalGravity float64
	TargetFinalGravity    float64
	MaxFermentationTempC  float64
	MinimumDays           int
	MaximumDays           int
	CustomValues          map[string]any
}

type UpdateCustomFieldsInput struct {
	ExpectedVersion *int64
	Values          map[string]any
}

type SetFormulaStepsInput struct {
	ExpectedVersion *int64
	Steps           []domain.FormulaStep
}

type VersionInput struct {
	ExpectedVersion *int64
}

type CreateExtensionFieldInput struct {
	EntityType domain.ExtensionEntity
	Name       string
	Type       domain.ExtensionFieldType
	Required   bool
}

type UpdateExtensionFieldInput struct {
	ExpectedVersion *int64
	Name            string
	Type            domain.ExtensionFieldType
	Required        bool
}

type CreateVesselInput struct {
	Code         string
	CapacityL    float64
	CustomValues map[string]any
}

type CreateRunInput struct {
	Code           string
	FormulaID      string
	PlannedVolumeL float64
	Notes          string
	CustomValues   map[string]any
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
