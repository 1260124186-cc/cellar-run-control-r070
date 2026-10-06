package service

import (
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) buildCustomFieldValues(
	entityType domain.EntityKind,
	inputs map[string]any,
	snapshot domain.Snapshot,
) (map[string]domain.CustomFieldValue, error) {
	values := make(map[string]domain.CustomFieldValue, len(inputs))
	now := s.clock.Now()
	for id, raw := range inputs {
		definition, ok := snapshot.ExtensionFields[id]
		if !ok {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				"is not a registered extension field")
		}
		if definition.EntityType != entityType {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				"does not belong to this entity type")
		}
		if !definition.Active {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				"cannot be written because the field is inactive")
		}
		value, err := domain.NewCustomFieldValue(definition, raw, now)
		if err != nil {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				customFieldErrorMessage(err))
		}
		values[id] = value
		definition.MarkUsed()
		snapshot.ExtensionFields[id] = definition
	}
	if err := domain.ValidateCustomFields(entityType, values, snapshot.ExtensionFields); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *Service) patchCustomFieldValues(
	current map[string]domain.CustomFieldValue,
	patch map[string]*any,
	entityType domain.EntityKind,
	snapshot domain.Snapshot,
) (map[string]domain.CustomFieldValue, error) {
	values := domain.CloneCustomFieldValues(current)
	if values == nil {
		values = map[string]domain.CustomFieldValue{}
	}
	now := s.clock.Now()
	for id, raw := range patch {
		definition, ok := snapshot.ExtensionFields[id]
		if !ok {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				"is not a registered extension field")
		}
		if definition.EntityType != entityType {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				"does not belong to this entity type")
		}
		if !definition.Active {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				"cannot be modified because the field is inactive")
		}
		if raw == nil {
			delete(values, id)
			continue
		}
		value, err := domain.NewCustomFieldValue(definition, *raw, now)
		if err != nil {
			return nil, domain.FieldError(domain.CodeInvalidInput, "custom_fields."+id,
				customFieldErrorMessage(err))
		}
		values[id] = value
		definition.MarkUsed()
		snapshot.ExtensionFields[id] = definition
	}
	if err := domain.ValidateCustomFields(entityType, values, snapshot.ExtensionFields); err != nil {
		return nil, err
	}
	return values, nil
}

func customFieldErrorMessage(err error) string {
	if domainErr, ok := err.(*domain.Error); ok {
		return domainErr.Message
	}
	return "must match the registered field type"
}
