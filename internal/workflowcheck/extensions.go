package workflowcheck

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
)

type extensionFieldView struct {
	ID         string `json:"id"`
	EntityType string `json:"entity_type"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Required   bool   `json:"required"`
	State      string `json:"state"`
	Version    int64  `json:"version"`
}

type extensionCustomFieldView struct {
	FieldID string `json:"field_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	State   string `json:"state"`
	Value   any    `json:"value"`
}

type extensionEntityView struct {
	ID           string                     `json:"id"`
	Version      int64                      `json:"version"`
	CustomFields []extensionCustomFieldView `json:"custom_fields"`
}

func extensionFieldRegistration(api *client, dataDir string) error {
	grain, err := createExtensionField(api, "formula", "Grain Batch", "text", true)
	if err != nil {
		return err
	}
	barrel, err := createExtensionField(api, "vessel", "Barrel Number", "text", true)
	if err != nil {
		return err
	}
	allergens, err := createExtensionField(api, "run", "Allergen Statement", "text", false)
	if err != nil {
		return err
	}

	duplicate, err := api.call("POST", "/v1/extension-fields", map[string]any{
		"entity_type": "formula",
		"name":        "  grain   batch  ",
		"type":        "text",
		"required":    false,
	}, nil)
	if err != nil {
		return err
	}
	if err := requireCode(duplicate, 409, string(domain.CodeExtensionConflict),
		"create duplicate active field name"); err != nil {
		return err
	}

	formula, err := createExtensionFormula(api, grain.ID)
	if err != nil {
		return err
	}
	draftPayload := createFormulaPayload("Extension Formula Draft")
	draftPayload["custom_fields"] = map[string]any{grain.ID: "Grain batch retained"}
	draftResponse, err := api.call("POST", "/v1/formulas", draftPayload, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(draftResponse, 201, "create second formula with old field"); err != nil {
		return err
	}
	var draftFormula extensionEntityView
	if err := draftResponse.decode(&draftFormula); err != nil {
		return err
	}
	vessel, err := createExtensionVessel(api, barrel.ID)
	if err != nil {
		return err
	}

	patchResponse, err := api.call("PATCH", "/v1/vessels/"+vessel.ID+"/custom-fields",
		map[string]any{"custom_fields": map[string]any{barrel.ID: "BARREL-8"}}, ptrInt64(vessel.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(patchResponse, 200, "update vessel custom field"); err != nil {
		return err
	}
	if err := patchResponse.decode(&vessel); err != nil {
		return err
	}
	if vessel.Version != 2 {
		return errInvalidWorkflow("vessel custom field update did not change the aggregate version")
	}
	if err := requireCustomField(vessel.CustomFields, barrel, "Barrel Number",
		"active", "BARREL-8"); err != nil {
		return err
	}

	renamed, err := updateExtensionField(api, grain.ID, grain.Version,
		"Grain Batch Number", "text", true)
	if err != nil {
		return err
	}
	if err := requireStatus(renamed, 200, "rename field definition"); err != nil {
		return err
	}
	if err := renamed.decode(&grain); err != nil {
		return err
	}
	if grain.Name != "Grain Batch Number" || grain.Version != 2 {
		return errInvalidWorkflow("field definition rename was not published")
	}

	stalePublish, err := updateExtensionField(api, grain.ID, grain.Version-1,
		"Stale Field Rename", "text", true)
	if err != nil {
		return err
	}
	if err := requireCode(stalePublish, 412, string(domain.CodeRevisionConflict),
		"publish a stale field definition"); err != nil {
		return err
	}

	typeChange, err := updateExtensionField(api, grain.ID, grain.Version,
		"Grain Batch Number", "number", true)
	if err != nil {
		return err
	}
	if err := requireCode(typeChange, 409, string(domain.CodeExtensionTypeInUse),
		"change type used by stored values"); err != nil {
		return err
	}

	madeRequired, err := updateExtensionField(api, allergens.ID, allergens.Version,
		"Allergen Statement", "text", true)
	if err != nil {
		return err
	}
	if err := requireStatus(madeRequired, 200, "make an unused field required"); err != nil {
		return err
	}
	if err := madeRequired.decode(&allergens); err != nil {
		return err
	}

	formula, err = approveExtensionFormula(api, formula)
	if err != nil {
		return err
	}
	run, err := createExtensionRun(api, formula.ID, allergens.ID)
	if err != nil {
		return err
	}

	deactivated, err := api.call("POST", "/v1/extension-fields/"+grain.ID+"/deactivate",
		nil, ptrInt64(grain.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(deactivated, 200, "deactivate old field definition"); err != nil {
		return err
	}
	if err := deactivated.decode(&grain); err != nil {
		return err
	}

	inactiveWrite, err := api.call("PATCH",
		"/v1/formulas/"+draftFormula.ID+"/custom-fields",
		map[string]any{"custom_fields": map[string]any{grain.ID: "old field"}}, nil)
	if err != nil {
		return err
	}
	if err := requireCode(inactiveWrite, 409, string(domain.CodeExtensionFieldState),
		"write through an inactive field"); err != nil {
		return err
	}

	replacement, err := createExtensionField(api, "formula", "Grain Batch", "text", false)
	if err != nil {
		return err
	}
	if replacement.ID == grain.ID {
		return errInvalidWorkflow("reused field name created a replacement definition")
	}

	if err := requireListedEntity("formulas", formula.ID, func(entity extensionEntityView) error {
		return requireCustomField(entity.CustomFields, grain, "Grain Batch Number",
			"inactive", "Grain batch 2026-10")
	}, api); err != nil {
		return err
	}

	if err := api.reopen(dataDir); err != nil {
		return fmt.Errorf("restart service: %w", err)
	}
	if formula, err = getExtensionEntity(api, "/v1/formulas/"+formula.ID, "formula"); err != nil {
		return err
	}
	if err := requireCustomField(formula.CustomFields, grain, "Grain Batch Number",
		"inactive", "Grain batch 2026-10"); err != nil {
		return err
	}
	if err := requireCustomField(formula.CustomFields, replacement, "Grain Batch",
		"active", nil); err != nil {
		return err
	}
	if vessel, err = getExtensionEntity(api, "/v1/vessels/"+vessel.ID, "vessel"); err != nil {
		return err
	}
	if err := requireCustomField(vessel.CustomFields, barrel, "Barrel Number",
		"active", "BARREL-8"); err != nil {
		return err
	}
	if run, err = getExtensionEntity(api, "/v1/runs/"+run.ID, "run"); err != nil {
		return err
	}
	if err := requireCustomField(run.CustomFields, allergens, "Allergen Statement",
		"active", "Contains wheat"); err != nil {
		return err
	}
	if err := requireListedEntity("vessels", vessel.ID, func(entity extensionEntityView) error {
		return requireCustomField(entity.CustomFields, barrel, "Barrel Number",
			"active", "BARREL-8")
	}, api); err != nil {
		return err
	}
	if err := requireListedEntity("runs", run.ID, func(entity extensionEntityView) error {
		return requireCustomField(entity.CustomFields, allergens, "Allergen Statement",
			"active", "Contains wheat")
	}, api); err != nil {
		return err
	}

	if err := requireConcurrentPublication(api); err != nil {
		return err
	}
	return nil
}

func createExtensionField(
	api *client,
	entity, name, fieldType string,
	required bool,
) (extensionFieldView, error) {
	response, err := api.call("POST", "/v1/extension-fields", map[string]any{
		"entity_type": entity,
		"name":        name,
		"type":        fieldType,
		"required":    required,
	}, nil)
	if err != nil {
		return extensionFieldView{}, err
	}
	if err := requireStatus(response, 201, "create "+name+" field"); err != nil {
		return extensionFieldView{}, err
	}
	var field extensionFieldView
	if err := response.decode(&field); err != nil {
		return extensionFieldView{}, err
	}
	return field, nil
}

func updateExtensionField(
	api *client,
	id string,
	version int64,
	name, fieldType string,
	required bool,
) (response, error) {
	return api.call("PUT", "/v1/extension-fields/"+id, map[string]any{
		"name":     name,
		"type":     fieldType,
		"required": required,
	}, ptrInt64(version))
}

func createExtensionFormula(api *client, grainID string) (extensionEntityView, error) {
	payload := createFormulaPayload("Extension Formula")
	payload["custom_fields"] = map[string]any{grainID: 42}
	wrongType, err := api.call("POST", "/v1/formulas", payload, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireCode(wrongType, 400, string(domain.CodeInvalidInput),
		"create formula with wrong custom field type"); err != nil {
		return extensionEntityView{}, err
	}

	missingRequired, err := api.call("POST", "/v1/formulas", createFormulaPayload("Extension Formula"), nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireCode(missingRequired, 400, string(domain.CodeInvalidInput),
		"create formula without required custom field"); err != nil {
		return extensionEntityView{}, err
	}

	payload = createFormulaPayload("Extension Formula")
	payload["custom_fields"] = map[string]any{grainID: "Grain batch 2026-10"}
	created, err := api.call("POST", "/v1/formulas", payload, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(created, 201, "create formula with custom field"); err != nil {
		return extensionEntityView{}, err
	}
	var formula extensionEntityView
	if err := created.decode(&formula); err != nil {
		return extensionEntityView{}, err
	}
	return formula, nil
}

func createExtensionVessel(api *client, barrelID string) (extensionEntityView, error) {
	missingRequired, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-EXT-1",
		"capacity_liters": 240.0,
	}, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireCode(missingRequired, 400, string(domain.CodeInvalidInput),
		"create vessel without required custom field"); err != nil {
		return extensionEntityView{}, err
	}
	created, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-EXT-1",
		"capacity_liters": 240.0,
		"custom_fields":   map[string]any{barrelID: "BARREL-7"},
	}, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(created, 201, "create vessel with custom field"); err != nil {
		return extensionEntityView{}, err
	}
	var vessel extensionEntityView
	if err := created.decode(&vessel); err != nil {
		return extensionEntityView{}, err
	}
	return vessel, nil
}

func approveExtensionFormula(api *client, formula extensionEntityView) (extensionEntityView, error) {
	steps, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(formula.Version))
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(steps, 200, "set extension formula steps"); err != nil {
		return extensionEntityView{}, err
	}
	if err := steps.decode(&formula); err != nil {
		return extensionEntityView{}, err
	}
	approved, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(formula.Version))
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(approved, 200, "approve extension formula"); err != nil {
		return extensionEntityView{}, err
	}
	if err := approved.decode(&formula); err != nil {
		return extensionEntityView{}, err
	}
	return formula, nil
}

func createExtensionRun(
	api *client,
	formulaID, allergensID string,
) (extensionEntityView, error) {
	missingRequired, err := api.call("POST", "/v1/runs", createRunPayload("RUN-EXT-1", formulaID), nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireCode(missingRequired, 400, string(domain.CodeInvalidInput),
		"create run without required custom field"); err != nil {
		return extensionEntityView{}, err
	}
	payload := createRunPayload("RUN-EXT-1", formulaID)
	payload["custom_fields"] = map[string]any{allergensID: "Contains wheat"}
	created, err := api.call("POST", "/v1/runs", payload, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(created, 201, "create run with custom field"); err != nil {
		return extensionEntityView{}, err
	}
	var run extensionEntityView
	if err := created.decode(&run); err != nil {
		return extensionEntityView{}, err
	}
	return run, nil
}

func getExtensionEntity(api *client, path, operation string) (extensionEntityView, error) {
	response, err := api.call("GET", path, nil, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(response, 200, "read "+operation+" after restart"); err != nil {
		return extensionEntityView{}, err
	}
	var entity extensionEntityView
	if err := response.decode(&entity); err != nil {
		return extensionEntityView{}, err
	}
	return entity, nil
}

func requireListedEntity(
	collection, id string,
	require func(extensionEntityView) error,
	api *client,
) error {
	response, err := api.call("GET", "/v1/"+collection, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(response, 200, "list "+collection+" with custom fields"); err != nil {
		return err
	}
	var body struct {
		Formulas []extensionEntityView `json:"formulas"`
		Vessels  []extensionEntityView `json:"vessels"`
		Runs     []extensionEntityView `json:"runs"`
	}
	if err := response.decode(&body); err != nil {
		return err
	}
	var entities []extensionEntityView
	switch collection {
	case "formulas":
		entities = body.Formulas
	case "vessels":
		entities = body.Vessels
	case "runs":
		entities = body.Runs
	}
	for _, entity := range entities {
		if entity.ID == id {
			return require(entity)
		}
	}
	return errInvalidWorkflow("listed collection omitted entity " + id)
}

func requireConcurrentPublication(api *client) error {
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := api.call("POST", "/v1/extension-fields", map[string]any{
				"entity_type": "run",
				"name":        "Concurrent Statement",
				"type":        "text",
				"required":    false,
			}, nil)
			if err != nil {
				results <- err
				return
			}
			acceptedSuccess := response.status == 201 && response.errorCode() == ""
			acceptedConflict := response.status == 409 &&
				response.errorCode() == string(domain.CodeExtensionConflict)
			if !acceptedSuccess && !acceptedConflict {
				results <- fmt.Errorf("concurrent publication returned status %d code %q",
					response.status, response.errorCode())
			}
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			return err
		}
	}
	return nil
}

func requireCustomField(
	fields []extensionCustomFieldView,
	definition extensionFieldView,
	expectedName, expectedState string,
	expectedValue any,
) error {
	for _, field := range fields {
		if field.FieldID != definition.ID {
			continue
		}
		if field.Name != expectedName || field.State != expectedState {
			return errInvalidWorkflow("historical custom field did not use its current definition")
		}
		encodedValue, err := json.Marshal(field.Value)
		if err != nil {
			return err
		}
		encodedExpected, err := json.Marshal(expectedValue)
		if err != nil {
			return err
		}
		if string(encodedValue) != string(encodedExpected) {
			return errInvalidWorkflow("historical custom field value was rewritten")
		}
		return nil
	}
	return errInvalidWorkflow("historical custom field was omitted from the entity view")
}
