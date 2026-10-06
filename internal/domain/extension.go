package domain

import "time"

type ExtensionEntity string

const (
	ExtensionEntityFormula ExtensionEntity = "formula"
	ExtensionEntityVessel  ExtensionEntity = "vessel"
	ExtensionEntityRun     ExtensionEntity = "run"
)

type ExtensionFieldType string

const (
	ExtensionFieldText    ExtensionFieldType = "text"
	ExtensionFieldNumber  ExtensionFieldType = "number"
	ExtensionFieldBoolean ExtensionFieldType = "boolean"
)

type ExtensionFieldState string

const (
	ExtensionFieldActive   ExtensionFieldState = "active"
	ExtensionFieldInactive ExtensionFieldState = "inactive"
)

type ExtensionField struct {
	ID            string              `json:"id"`
	EntityType    ExtensionEntity     `json:"entity_type"`
	Name          string              `json:"name"`
	Type          ExtensionFieldType  `json:"type"`
	Required      bool                `json:"required"`
	State         ExtensionFieldState `json:"state"`
	Version       int64               `json:"version"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	DeactivatedAt *time.Time          `json:"deactivated_at,omitempty"`
}

type CustomFieldView struct {
	FieldID  string             `json:"field_id"`
	Name     string             `json:"name"`
	Type     ExtensionFieldType `json:"type"`
	Required bool               `json:"required"`
	State    ExtensionFieldState `json:"state"`
	Value    any                `json:"value,omitempty"`
}

func (e ExtensionEntity) Valid() bool {
	switch e {
	case ExtensionEntityFormula, ExtensionEntityVessel, ExtensionEntityRun:
		return true
	default:
		return false
	}
}

func (t ExtensionFieldType) Valid() bool {
	switch t {
	case ExtensionFieldText, ExtensionFieldNumber, ExtensionFieldBoolean:
		return true
	default:
		return false
	}
}

func ExtensionFieldIndexKey(entity ExtensionEntity, name string) string {
	return string(entity) + "\x00" + NameKey(name)
}

func (f *ExtensionField) Normalize() {
	f.Name = NormalizeName(f.Name)
}

func (f ExtensionField) Validate() error {
	if !f.EntityType.Valid() {
		return FieldError(CodeInvalidInput, "entity_type", "must be formula, vessel, or run")
	}
	if f.Name == "" {
		return FieldError(CodeInvalidInput, "name", "is required")
	}
	if len([]rune(f.Name)) > 80 {
		return FieldError(CodeInvalidInput, "name", "must not exceed 80 characters")
	}
	if !f.Type.Valid() {
		return FieldError(CodeInvalidInput, "type", "must be text, number, or boolean")
	}
	if f.State != ExtensionFieldActive && f.State != ExtensionFieldInactive {
		return FieldError(CodeInvalidInput, "state", "must be active or inactive")
	}
	if f.Version < 1 {
		return FieldError(CodeInvalidInput, "version", "must be positive")
	}
	return nil
}

func (f *ExtensionField) Update(
	name string,
	fieldType ExtensionFieldType,
	required bool,
	hasValues bool,
	now time.Time,
) error {
	if f.State != ExtensionFieldActive {
		return NewError(CodeExtensionFieldState, "only an active field definition can be updated")
	}
	next := ExtensionField{
		EntityType: f.EntityType,
		Name:       NormalizeName(name),
		Type:       fieldType,
		Required:   required,
		State:      ExtensionFieldActive,
		Version:    1,
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if f.Type != next.Type && hasValues {
		return NewError(CodeExtensionTypeInUse,
			"cannot change a field type while entities still contain values for the field")
	}
	f.Name = next.Name
	f.Type = next.Type
	f.Required = next.Required
	f.UpdatedAt = now.UTC()
	f.Version++
	return nil
}

func (f *ExtensionField) Deactivate(now time.Time) error {
	if f.State == ExtensionFieldInactive {
		return NewError(CodeExtensionFieldState, "field definition is already inactive")
	}
	stamp := now.UTC()
	f.State = ExtensionFieldInactive
	f.DeactivatedAt = &stamp
	f.UpdatedAt = stamp
	f.Version++
	return nil
}

func validateCustomValueType(definition ExtensionField, value any) error {
	if value == nil {
		return nil
	}
	switch definition.Type {
	case ExtensionFieldText:
		text, ok := value.(string)
		if !ok {
			return FieldError(CodeInvalidInput, "custom_fields."+definition.ID,
				"must be a text value")
		}
		if NormalizeName(text) == "" {
			return FieldError(CodeInvalidInput, "custom_fields."+definition.ID,
				"must not be blank")
		}
		if len([]rune(text)) > 1000 {
			return FieldError(CodeInvalidInput, "custom_fields."+definition.ID,
				"must not exceed 1000 characters")
		}
	case ExtensionFieldNumber:
		if _, ok := value.(float64); !ok {
			return FieldError(CodeInvalidInput, "custom_fields."+definition.ID,
				"must be a number")
		}
	case ExtensionFieldBoolean:
		if _, ok := value.(bool); !ok {
			return FieldError(CodeInvalidInput, "custom_fields."+definition.ID,
				"must be a boolean")
		}
	default:
		return NewError(CodeStorage, "field definition has an unsupported type")
	}
	return nil
}

// ValidateCustomValues validates values supplied to an active definition set.
// When requirePresent is true, every active required definition must have a
// non-null value after validation.
func ValidateCustomValues(
	entity ExtensionEntity,
	definitions map[string]ExtensionField,
	values map[string]any,
	requirePresent bool,
) error {
	for fieldID, value := range values {
		definition, ok := definitions[fieldID]
		if !ok {
			return FieldError(CodeInvalidInput, "custom_fields."+fieldID,
				"is not a registered field")
		}
		if definition.EntityType != entity {
			return FieldError(CodeInvalidInput, "custom_fields."+fieldID,
				"is registered for a different entity type")
		}
		if definition.State != ExtensionFieldActive {
			return FieldError(CodeExtensionFieldState, "custom_fields."+fieldID,
				"cannot write to an inactive field")
		}
		if err := validateCustomValueType(definition, value); err != nil {
			return err
		}
	}
	if requirePresent {
		for _, definition := range definitions {
			if definition.EntityType != entity || definition.State != ExtensionFieldActive ||
				!definition.Required {
				continue
			}
			value, present := values[definition.ID]
			if !present || value == nil {
				return FieldError(CodeInvalidInput, "custom_fields."+definition.ID,
					"is required")
			}
		}
	}
	return nil
}

// ValidateStoredCustomValues checks values already present in a snapshot. It
// deliberately does not apply required-field checks because definitions can
// become required after historical entities were created.
func ValidateStoredCustomValues(
	entity ExtensionEntity,
	definitions map[string]ExtensionField,
	values map[string]any,
) error {
	for fieldID, value := range values {
		definition, ok := definitions[fieldID]
		if !ok {
			return FieldError(CodeStorage, "custom_fields."+fieldID,
				"references a missing field definition")
		}
		if definition.EntityType != entity {
			return FieldError(CodeStorage, "custom_fields."+fieldID,
				"references a definition for another entity type")
		}
		if err := validateCustomValueType(definition, value); err != nil {
			return err
		}
	}
	return nil
}

func MergeCustomValues(existing, changes map[string]any) map[string]any {
	merged := CloneCustomValues(existing)
	if merged == nil {
		merged = map[string]any{}
	}
	for key, value := range changes {
		if value == nil {
			delete(merged, key)
			continue
		}
		merged[key] = cloneCustomValue(value)
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func CloneCustomValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneCustomValue(value)
	}
	return cloned
}

func cloneCustomValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, nested := range typed {
			cloned[key] = cloneCustomValue(nested)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, nested := range typed {
			cloned[index] = cloneCustomValue(nested)
		}
		return cloned
	default:
		return value
	}
}

func BuildCustomFieldViews(
	entity ExtensionEntity,
	definitions map[string]ExtensionField,
	values map[string]any,
) []CustomFieldView {
	views := make([]CustomFieldView, 0)
	for fieldID, definition := range definitions {
		if definition.EntityType != entity {
			continue
		}
		value, present := values[fieldID]
		if !present && definition.State != ExtensionFieldActive {
			continue
		}
		view := CustomFieldView{
			FieldID:  definition.ID,
			Name:     definition.Name,
			Type:     definition.Type,
			Required: definition.Required,
			State:    definition.State,
			Value:    value,
		}
		if !present {
			view.Value = nil
		}
		views = append(views, view)
	}
	sortCustomFieldViews(views)
	return views
}

func sortCustomFieldViews(views []CustomFieldView) {
	for index := 1; index < len(views); index++ {
		for previous := index; previous > 0; previous-- {
			if views[previous-1].Name < views[previous].Name ||
				(views[previous-1].Name == views[previous].Name &&
					views[previous-1].FieldID <= views[previous].FieldID) {
				break
			}
			views[previous-1], views[previous] = views[previous], views[previous-1]
		}
	}
}
