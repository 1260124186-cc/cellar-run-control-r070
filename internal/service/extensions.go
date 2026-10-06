package service

import (
	"sort"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

func (s *Service) CreateExtensionField(
	input CreateExtensionFieldInput,
) (domain.ExtensionFieldDefinition, error) {
	entityType := domain.NormalizeEntityKind(string(input.EntityType))
	name := domain.NormalizeName(input.Name)
	fieldType := domain.NormalizeFieldType(string(input.Type))
	candidate := domain.ExtensionFieldDefinition{
		EntityType: entityType,
		Name:       name,
		Type:       fieldType,
		Required:   input.Required,
		Active:     true,
		Revision:   1,
	}
	if err := candidate.Validate(); err != nil {
		return domain.ExtensionFieldDefinition{}, err
	}

	id, err := generateEntityID("field")
	if err != nil {
		return domain.ExtensionFieldDefinition{}, domain.NewError(domain.CodeStorage, err.Error())
	}
	candidate.ID = id
	now := s.clock.Now()
	candidate.CreatedAt = now
	candidate.UpdatedAt = now

	err = s.store.Execute(func(snapshot *domain.Snapshot) error {
		count := 0
		for _, definition := range snapshot.ExtensionFields {
			if definition.EntityType != entityType {
				continue
			}
			count++
			if domain.NameKey(definition.Name) == domain.NameKey(name) {
				return domain.NewError(domain.CodeExtensionConflict,
					"an extension field with the same normalized name already exists")
			}
		}
		if count >= domain.MaxExtensionFieldsPerEntity {
			return domain.NewError(domain.CodeInvalidInput,
				"an entity type cannot define more than 100 extension fields")
		}
		snapshot.ExtensionFields[id] = candidate
		return nil
	})
	if err != nil {
		return domain.ExtensionFieldDefinition{}, err
	}
	return candidate, nil
}

func (s *Service) UpdateExtensionField(
	id string,
	input UpdateExtensionFieldInput,
) (domain.ExtensionFieldDefinition, error) {
	var updated domain.ExtensionFieldDefinition
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		definition, ok := snapshot.ExtensionFields[id]
		if !ok {
			return domain.NewError(domain.CodeExtensionNotFound, "extension field was not found")
		}
		if input.ExpectedRevision != nil && *input.ExpectedRevision != definition.Revision {
			return domain.NewError(domain.CodeRevisionConflict,
				"the supplied extension field revision does not match the current revision")
		}
		newType := domain.NormalizeFieldType(string(input.Type))
		if newType != definition.Type && definition.Used {
			return domain.NewError(domain.CodeExtensionConflict,
				"cannot change an extension field type after values have been written")
		}
		if err := definition.Update(input.Name, newType, input.Required, s.clock.Now()); err != nil {
			return err
		}
		for otherID, other := range snapshot.ExtensionFields {
			if otherID == id || other.EntityType != definition.EntityType {
				continue
			}
			if domain.NameKey(other.Name) == domain.NameKey(definition.Name) {
				return domain.NewError(domain.CodeExtensionConflict,
					"an extension field with the same normalized name already exists")
			}
		}
		snapshot.ExtensionFields[id] = definition
		updated = definition
		return nil
	})
	if err != nil {
		return domain.ExtensionFieldDefinition{}, err
	}
	return updated, nil
}

func (s *Service) DeactivateExtensionField(
	id string,
	expected *int64,
) (domain.ExtensionFieldDefinition, error) {
	var updated domain.ExtensionFieldDefinition
	err := s.store.Execute(func(snapshot *domain.Snapshot) error {
		definition, ok := snapshot.ExtensionFields[id]
		if !ok {
			return domain.NewError(domain.CodeExtensionNotFound, "extension field was not found")
		}
		if expected != nil && *expected != definition.Revision {
			return domain.NewError(domain.CodeRevisionConflict,
				"the supplied extension field revision does not match the current revision")
		}
		if err := definition.Deactivate(s.clock.Now()); err != nil {
			return err
		}
		snapshot.ExtensionFields[id] = definition
		updated = definition
		return nil
	})
	if err != nil {
		return domain.ExtensionFieldDefinition{}, err
	}
	return updated, nil
}

func (s *Service) GetExtensionField(id string) (domain.ExtensionFieldDefinition, error) {
	var found domain.ExtensionFieldDefinition
	err := s.store.View(func(snapshot domain.Snapshot) error {
		definition, ok := snapshot.ExtensionFields[id]
		if !ok {
			return domain.NewError(domain.CodeExtensionNotFound, "extension field was not found")
		}
		found = definition
		return nil
	})
	if err != nil {
		return domain.ExtensionFieldDefinition{}, err
	}
	return found, nil
}

func (s *Service) ListExtensionFields(entityType string) ([]domain.ExtensionFieldDefinition, error) {
	kind := domain.NormalizeEntityKind(entityType)
	if entityType != "" && !kind.Valid() {
		return nil, domain.FieldError(domain.CodeInvalidInput, "entity_type",
			"must be formula, vessel, or run")
	}
	var fields []domain.ExtensionFieldDefinition
	err := s.store.View(func(snapshot domain.Snapshot) error {
		fields = make([]domain.ExtensionFieldDefinition, 0, len(snapshot.ExtensionFields))
		for _, definition := range snapshot.ExtensionFields {
			if entityType == "" || definition.EntityType == kind {
				fields = append(fields, definition)
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
