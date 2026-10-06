package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

const stateFileName = "state.json"

type Store struct {
	mu       sync.RWMutex
	path     string
	snapshot domain.Snapshot
	now      func() time.Time
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	store := &Store{
		path:     filepath.Join(dataDir, stateFileName),
		snapshot: domain.EmptySnapshot(),
		now:      func() time.Time { return time.Now().UTC() },
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	var loaded domain.Snapshot
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return fmt.Errorf("decode state: %w", err)
	}
	normalized, err := normalizeSnapshot(loaded)
	if err != nil {
		return err
	}
	s.snapshot = normalized
	return nil
}

func normalizeSnapshot(value domain.Snapshot) (domain.Snapshot, error) {
	if value.Revision < 0 {
		return domain.Snapshot{}, fmt.Errorf("state revision cannot be negative")
	}
	if value.Formulas == nil {
		value.Formulas = map[string]domain.Formula{}
	}
	if value.Vessels == nil {
		value.Vessels = map[string]domain.Vessel{}
	}
	if value.Runs == nil {
		value.Runs = map[string]domain.FermentationRun{}
	}
	if value.ExtensionFields == nil {
		value.ExtensionFields = map[string]domain.ExtensionFieldDefinition{}
	}
	if value.NameIndex == nil {
		value.NameIndex = map[string]string{}
	}
	if value.VesselIndex == nil {
		value.VesselIndex = map[string]string{}
	}
	if value.RunIndex == nil {
		value.RunIndex = map[string]string{}
	}
	if err := validateExtensionDefinitions(value.ExtensionFields); err != nil {
		return domain.Snapshot{}, err
	}
	if err := validateStoredCustomFields(value); err != nil {
		return domain.Snapshot{}, err
	}
	markDefinitionsUsed(&value)
	if err := validateIndexes(value); err != nil {
		return domain.Snapshot{}, err
	}
	return value, nil
}

func validateExtensionDefinitions(definitions map[string]domain.ExtensionFieldDefinition) error {
	names := make(map[string]string, len(definitions))
	activeNames := make(map[string]string, len(definitions))
	for id, definition := range definitions {
		if definition.ID != id {
			return fmt.Errorf("extension field %s has a mismatched identifier", id)
		}
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("extension field %s is invalid: %w", id, err)
		}
		nameKey := string(definition.EntityType) + "\x00" + domain.NameKey(definition.Name)
		if existing, found := names[nameKey]; found && existing != id {
			return fmt.Errorf("extension field name %q is duplicated", definition.Name)
		}
		names[nameKey] = id
		if definition.Active {
			if existing, found := activeNames[nameKey]; found && existing != id {
				return fmt.Errorf("active extension field name %q is duplicated", definition.Name)
			}
			activeNames[nameKey] = id
		}
	}
	return nil
}

func validateStoredCustomFields(value domain.Snapshot) error {
	for id, formula := range value.Formulas {
		if err := validateEntityCustomFields(string(domain.EntityFormula), id,
			formula.CustomFields, value.ExtensionFields); err != nil {
			return err
		}
	}
	for id, vessel := range value.Vessels {
		if err := validateEntityCustomFields(string(domain.EntityVessel), id,
			vessel.CustomFields, value.ExtensionFields); err != nil {
			return err
		}
	}
	for id, run := range value.Runs {
		if err := validateEntityCustomFields(string(domain.EntityRun), id,
			run.CustomFields, value.ExtensionFields); err != nil {
			return err
		}
	}
	return nil
}

func validateEntityCustomFields(
	entityType string,
	entityID string,
	values map[string]domain.CustomFieldValue,
	definitions map[string]domain.ExtensionFieldDefinition,
) error {
	for fieldID, stored := range values {
		definition, ok := definitions[fieldID]
		if !ok {
			return fmt.Errorf("%s %s references missing extension field %s",
				entityType, entityID, fieldID)
		}
		if string(definition.EntityType) != entityType {
			return fmt.Errorf("%s %s references extension field for another entity type",
				entityType, entityID)
		}
		if stored.DefinitionRevision < 1 || stored.DefinitionRevision > definition.Revision {
			return fmt.Errorf("%s %s references invalid revision for extension field %s",
				entityType, entityID, fieldID)
		}
		if !stored.Type.Valid() {
			return fmt.Errorf("%s %s contains an unreadable extension value type",
				entityType, entityID)
		}
		if err := domain.ValidateCustomValue(stored.Value, stored.Type); err != nil {
			return fmt.Errorf("%s %s has invalid extension field %s: %w",
				entityType, entityID, fieldID, err)
		}
	}
	return nil
}

func markDefinitionsUsed(value *domain.Snapshot) {
	used := make(map[string]struct{})
	for _, formula := range value.Formulas {
		for fieldID := range formula.CustomFields {
			used[fieldID] = struct{}{}
		}
	}
	for _, vessel := range value.Vessels {
		for fieldID := range vessel.CustomFields {
			used[fieldID] = struct{}{}
		}
	}
	for _, run := range value.Runs {
		for fieldID := range run.CustomFields {
			used[fieldID] = struct{}{}
		}
	}
	for fieldID := range used {
		if definition, ok := value.ExtensionFields[fieldID]; ok && !definition.Used {
			definition.Used = true
			value.ExtensionFields[fieldID] = definition
		}
	}
}

func validateIndexes(value domain.Snapshot) error {
	expectedNames := make(map[string]string, len(value.Formulas))
	for id, formula := range value.Formulas {
		key := domain.NameKey(formula.Name)
		if key == "" {
			return fmt.Errorf("formula %s has an empty normalized name", id)
		}
		if existing, found := expectedNames[key]; found && existing != id {
			return fmt.Errorf("formula name index contains a duplicate key")
		}
		expectedNames[key] = id
	}
	if err := compareIndexes("formula name", value.NameIndex, expectedNames); err != nil {
		return err
	}

	expectedVessels := make(map[string]string, len(value.Vessels))
	for id, vessel := range value.Vessels {
		if existing, found := expectedVessels[vessel.Code]; found && existing != id {
			return fmt.Errorf("vessel index contains a duplicate code")
		}
		expectedVessels[vessel.Code] = id
	}
	if err := compareIndexes("vessel code", value.VesselIndex, expectedVessels); err != nil {
		return err
	}

	expectedRuns := make(map[string]string, len(value.Runs))
	for id, run := range value.Runs {
		if existing, found := expectedRuns[run.Code]; found && existing != id {
			return fmt.Errorf("run index contains a duplicate code")
		}
		expectedRuns[run.Code] = id
	}
	return compareIndexes("run code", value.RunIndex, expectedRuns)
}

func compareIndexes(label string, actual, expected map[string]string) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("%s index does not match its collection", label)
	}
	for key, expectedID := range expected {
		actualID, found := actual[key]
		if !found || actualID != expectedID {
			return fmt.Errorf("%s index does not match its collection", label)
		}
	}
	return nil
}

func (s *Store) Revision() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot.Revision
}

func (s *Store) View(visitor func(domain.Snapshot) error) error {
	s.mu.RLock()
	copied := s.snapshot.Clone()
	s.mu.RUnlock()
	return visitor(copied)
}

func (s *Store) Update(expected *int64, mutate func(*domain.Snapshot) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if expected != nil && *expected != s.snapshot.Revision {
		return domain.NewError(domain.CodeRevisionConflict,
			"the supplied revision does not match the current snapshot")
	}
	candidate := s.snapshot.Clone()
	if err := mutate(&candidate); err != nil {
		return err
	}
	candidate.Revision = s.snapshot.Revision + 1
	candidate.UpdatedAt = s.now()
	if err := writeSnapshot(s.path, candidate); err != nil {
		return domain.NewError(domain.CodeStorage, err.Error())
	}
	s.snapshot = candidate
	return nil
}

func (s *Store) Execute(mutator func(*domain.Snapshot) error) error {
	return s.Update(nil, mutator)
}
