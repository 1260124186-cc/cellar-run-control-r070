package domain

import (
	"math"
	"strings"
	"time"
)

type EntityKind string

type FieldType string

const (
	EntityFormula EntityKind = "formula"
	EntityVessel  EntityKind = "vessel"
	EntityRun     EntityKind = "run"
)

const (
	FieldText    FieldType = "text"
	FieldNumber  FieldType = "number"
	FieldBoolean FieldType = "boolean"
	FieldDate    FieldType = "date"
)

const MaxExtensionFieldsPerEntity = 100

type ExtensionFieldDefinition struct {
	ID            string     `json:"id"`
	EntityType    EntityKind `json:"entity_type"`
	Name          string     `json:"name"`
	Type          FieldType  `json:"type"`
	Required      bool       `json:"required"`
	Active        bool       `json:"active"`
	Used          bool       `json:"used"`
	Revision      int64      `json:"revision"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeactivatedAt *time.Time `json:"deactivated_at,omitempty"`
}

type CustomFieldValue struct {
	Value              any       `json:"value"`
	Type               FieldType `json:"type"`
	DefinitionRevision int64     `json:"definition_revision"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func NormalizeEntityKind(value string) EntityKind {
	return EntityKind(strings.ToLower(strings.TrimSpace(value)))
}

func (kind EntityKind) Valid() bool {
	return kind == EntityFormula || kind == EntityVessel || kind == EntityRun
}

func NormalizeFieldType(value string) FieldType {
	return FieldType(strings.ToLower(strings.TrimSpace(value)))
}

func (fieldType FieldType) Valid() bool {
	return fieldType == FieldText || fieldType == FieldNumber ||
		fieldType == FieldBoolean || fieldType == FieldDate
}

func (definition ExtensionFieldDefinition) Validate() error {
	if !definition.EntityType.Valid() {
		return FieldError(CodeInvalidInput, "entity_type", "must be formula, vessel, or run")
	}
	if definition.Name == "" {
		return FieldError(CodeInvalidInput, "name", "is required")
	}
	if len([]rune(definition.Name)) > 80 {
		return FieldError(CodeInvalidInput, "name", "must not exceed 80 characters")
	}
	if !definition.Type.Valid() {
		return FieldError(CodeInvalidInput, "type",
			"must be text, number, boolean, or date")
	}
	if definition.Revision < 1 {
		return NewError(CodeInvalidInput, "extension field revision must be positive")
	}
	return nil
}

func (definition *ExtensionFieldDefinition) Update(
	name string,
	fieldType FieldType,
	required bool,
	now time.Time,
) error {
	if !definition.Active {
		return NewError(CodeExtensionState, "only an active field can be updated")
	}
	stamp := now.UTC()
	definition.Name = NormalizeName(name)
	definition.Type = fieldType
	definition.Required = required
	definition.Revision++
	definition.UpdatedAt = stamp
	return definition.Validate()
}

func (definition *ExtensionFieldDefinition) MarkUsed() {
	definition.Used = true
}

func (definition *ExtensionFieldDefinition) Deactivate(now time.Time) error {
	if !definition.Active {
		return NewError(CodeExtensionState, "extension field is already inactive")
	}
	stamp := now.UTC()
	definition.Active = false
	definition.DeactivatedAt = &stamp
	definition.UpdatedAt = stamp
	definition.Revision++
	return nil
}

func NewCustomFieldValue(
	definition ExtensionFieldDefinition,
	value any,
	now time.Time,
) (CustomFieldValue, error) {
	if err := ValidateCustomValue(value, definition.Type); err != nil {
		return CustomFieldValue{}, err
	}
	return CustomFieldValue{
		Value:              value,
		Type:               definition.Type,
		DefinitionRevision: definition.Revision,
		UpdatedAt:          now.UTC(),
	}, nil
}

func ValidateCustomValue(value any, fieldType FieldType) error {
	switch fieldType {
	case FieldText:
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return FieldError(CodeInvalidInput, "custom_fields", "must be non-empty text")
		}
		if len([]rune(text)) > 1000 {
			return FieldError(CodeInvalidInput, "custom_fields",
				"text must not exceed 1000 characters")
		}
	case FieldNumber:
		number, ok := value.(float64)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return FieldError(CodeInvalidInput, "custom_fields", "must be a finite number")
		}
	case FieldBoolean:
		if _, ok := value.(bool); !ok {
			return FieldError(CodeInvalidInput, "custom_fields", "must be a boolean")
		}
	case FieldDate:
		text, ok := value.(string)
		if !ok {
			return FieldError(CodeInvalidInput, "custom_fields", "must be an RFC3339 date string")
		}
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return FieldError(CodeInvalidInput, "custom_fields", "must be an RFC3339 date string")
		}
	default:
		return FieldError(CodeInvalidInput, "custom_fields", "has an unsupported field type")
	}
	return nil
}

func ValidateCustomFields(
	entityType EntityKind,
	values map[string]CustomFieldValue,
	definitions map[string]ExtensionFieldDefinition,
) error {
	for id, value := range values {
		definition, ok := definitions[id]
		if !ok {
			return FieldError(CodeInvalidInput, "custom_fields."+id,
				"is not a registered extension field")
		}
		if definition.EntityType != entityType {
			return FieldError(CodeInvalidInput, "custom_fields."+id,
				"does not belong to this entity type")
		}
		if value.DefinitionRevision < 1 || value.DefinitionRevision > definition.Revision {
			return FieldError(CodeInvalidInput, "custom_fields."+id,
				"references an unknown field definition revision")
		}
		if err := ValidateCustomValue(value.Value, value.Type); err != nil {
			return FieldError(CodeInvalidInput, "custom_fields."+id, err.Error())
		}
	}

	for id, definition := range definitions {
		if definition.EntityType != entityType || !definition.Active {
			continue
		}
		value, exists := values[id]
		if definition.Required && !exists {
			return FieldError(CodeInvalidInput, "custom_fields."+id, "is required")
		}
		if exists && value.Type != definition.Type {
			return FieldError(CodeInvalidInput, "custom_fields."+id,
				"uses an older field type and must be supplied using the current type before this entity can be modified")
		}
	}
	return nil
}

func CloneCustomFieldValues(values map[string]CustomFieldValue) map[string]CustomFieldValue {
	if values == nil {
		return nil
	}
	cloned := make(map[string]CustomFieldValue, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
