package service

import (
	"sort"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateExtensionField(
	input CreateExtensionFieldInput,
) (domain.ExtensionField, error) {
	field := domain.ExtensionField{
		EntityType: input.EntityType,
		Name:       input.Name,
		Type:       input.Type,
		Required:   input.Required,
		State:      domain.ExtensionFieldActive,
		Version:    1,
	}
	field.Normalize()
	if err := field.Validate(); err != nil {
		return domain.ExtensionField{}, err
	}

	id, err := generateEntityID("extfield")
	if err != nil {
		return domain.ExtensionField{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	field.ID = id
	now := s.clock.Now()
	field.CreatedAt = now
	field.UpdatedAt = now
	nameKey := domain.ExtensionFieldIndexKey(field.EntityType, field.Name)

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		if _, exists := snapshot.ExtensionNameIdx[nameKey]; exists {
			return domain.NewError(domain.CodeExtensionConflict,
				"an active field with the same normalized name already exists")
		}
		snapshot.ExtensionFields[field.ID] = field
		snapshot.ExtensionNameIdx[nameKey] = field.ID
		return nil
	})
	if err != nil {
		return domain.ExtensionField{}, err
	}
	return field, nil
}

func (s *Service) UpdateExtensionField(
	id string,
	input UpdateExtensionFieldInput,
) (domain.ExtensionField, error) {
	var updated domain.ExtensionField
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		field, ok := snapshot.ExtensionFields[id]
		if !ok {
			return domain.NewError(domain.CodeExtensionNotFound, "field definition was not found")
		}
		if err := s.checkExpected(field.Version, input.ExpectedVersion); err != nil {
			return err
		}
		oldKey := domain.ExtensionFieldIndexKey(field.EntityType, field.Name)
		hasValues := snapshot.ExtensionFieldHasValues(id)
		if err := field.Update(input.Name, input.Type, input.Required, hasValues, s.clock.Now()); err != nil {
			return err
		}
		newKey := domain.ExtensionFieldIndexKey(field.EntityType, field.Name)
		if oldKey != newKey {
			if existing, exists := snapshot.ExtensionNameIdx[newKey]; exists && existing != id {
				return domain.NewError(domain.CodeExtensionConflict,
					"an active field with the same normalized name already exists")
			}
			delete(snapshot.ExtensionNameIdx, oldKey)
			snapshot.ExtensionNameIdx[newKey] = id
		}
		snapshot.ExtensionFields[id] = field
		updated = field
		return nil
	})
	if err != nil {
		return domain.ExtensionField{}, err
	}
	return updated, nil
}

func (s *Service) DeactivateExtensionField(
	id string,
	input VersionInput,
) (domain.ExtensionField, error) {
	var updated domain.ExtensionField
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		field, ok := snapshot.ExtensionFields[id]
		if !ok {
			return domain.NewError(domain.CodeExtensionNotFound, "field definition was not found")
		}
		if err := s.checkExpected(field.Version, input.ExpectedVersion); err != nil {
			return err
		}
		if err := field.Deactivate(s.clock.Now()); err != nil {
			return err
		}
		key := domain.ExtensionFieldIndexKey(field.EntityType, field.Name)
		delete(snapshot.ExtensionNameIdx, key)
		snapshot.ExtensionFields[id] = field
		updated = field
		return nil
	})
	if err != nil {
		return domain.ExtensionField{}, err
	}
	return updated, nil
}

func (s *Service) ListExtensionFields(
	entity domain.ExtensionEntity,
) ([]domain.ExtensionField, error) {
	if entity != "" && !entity.Valid() {
		return nil, domain.FieldError(domain.CodeInvalidInput, "entity_type",
			"must be formula, vessel, or run")
	}
	var fields []domain.ExtensionField
	err := s.store.View(func(snapshot domain.Snapshot) error {
		fields = make([]domain.ExtensionField, 0, len(snapshot.ExtensionFields))
		for _, field := range snapshot.ExtensionFields {
			if entity == "" || field.EntityType == entity {
				fields = append(fields, field)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].EntityType != fields[j].EntityType {
			return fields[i].EntityType < fields[j].EntityType
		}
		if fields[i].Name != fields[j].Name {
			return fields[i].Name < fields[j].Name
		}
		return fields[i].ID < fields[j].ID
	})
	return fields, nil
}
