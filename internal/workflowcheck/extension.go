package workflowcheck

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/clock"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/httpapi"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/storage"
)

type extensionFieldView struct {
	ID         string            `json:"id"`
	EntityType domain.EntityKind `json:"entity_type"`
	Name       string            `json:"name"`
	Type       domain.FieldType  `json:"type"`
	Required   bool              `json:"required"`
	Active     bool              `json:"active"`
	Used       bool              `json:"used"`
	Revision   int64             `json:"revision"`
}

type extensionListResponse struct {
	ExtensionFields []extensionFieldView `json:"extension_fields"`
}

type extensionEntityView struct {
	ID           string                             `json:"id"`
	Code         string                             `json:"code"`
	State        string                             `json:"state"`
	Version      int64                              `json:"version"`
	CustomFields map[string]domain.CustomFieldValue `json:"custom_fields"`
}

func extensionFields(dataDir string) error {
	api, closeServer, err := startExtensionServer(dataDir)
	if err != nil {
		return err
	}

	formulaField, err := createExtensionField(api, "formula", "Grain Batch", "text", true)
	if err != nil {
		closeServer()
		return err
	}
	vesselField, err := createExtensionField(api, "vessel", "Barrel Number", "text", true)
	if err != nil {
		closeServer()
		return err
	}
	runField, err := createExtensionField(api, "run", "Allergen Declaration", "boolean", true)
	if err != nil {
		closeServer()
		return err
	}
	duplicate, err := api.call("POST", "/v1/extension-fields", map[string]any{
		"entity_type": "formula",
		"name":        "  grain   batch ",
		"type":        "text",
		"required":    false,
	}, nil)
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(duplicate, 409, string(domain.CodeExtensionConflict),
		"create duplicate extension field"); err != nil {
		closeServer()
		return err
	}
	if err := verifyConcurrentExtensionPublishing(api); err != nil {
		closeServer()
		return err
	}

	missingRequired, err := api.call("POST", "/v1/formulas",
		createFormulaPayload("Extension Formula"), nil)
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(missingRequired, 400, string(domain.CodeInvalidInput),
		"create formula without required custom field"); err != nil {
		closeServer()
		return err
	}

	unknownField, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-EXT-1",
		"capacity_liters": 250,
		"custom_fields": map[string]any{
			"field_unknown": "x",
		},
	}, nil)
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(unknownField, 400, string(domain.CodeInvalidInput),
		"create vessel with unregistered field"); err != nil {
		closeServer()
		return err
	}

	formula, err := createExtensionFormula(api, formulaField.ID)
	if err != nil {
		closeServer()
		return err
	}
	stepsResponse, err := api.call("PUT", "/v1/formulas/"+formula.ID+"/steps",
		formulaStepPayload(), ptrInt64(formula.Version))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireStatus(stepsResponse, 200, "set extension formula steps"); err != nil {
		closeServer()
		return err
	}
	if err := stepsResponse.decode(&formula); err != nil {
		closeServer()
		return err
	}
	approveResponse, err := api.call("POST", "/v1/formulas/"+formula.ID+"/approve",
		nil, ptrInt64(formula.Version))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireStatus(approveResponse, 200, "approve extension formula"); err != nil {
		closeServer()
		return err
	}
	if err := approveResponse.decode(&formula); err != nil {
		closeServer()
		return err
	}

	vessel, err := createExtensionVessel(api, vesselField.ID)
	if err != nil {
		closeServer()
		return err
	}
	run, err := createExtensionRun(api, formula.ID, runField.ID)
	if err != nil {
		closeServer()
		return err
	}
	startedRun, startedVessel, err := reserveAndStart(api, run.ID, vessel.ID, run.Version)
	if err != nil {
		closeServer()
		return err
	}
	run.Version = startedRun.Version
	vessel.Version = startedVessel.Version

	immutableFormula, err := api.call("PATCH", "/v1/formulas/"+formula.ID+"/custom-fields",
		customFieldPatch(formulaField.ID, "batch-2"), ptrInt64(formula.Version))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(immutableFormula, 409, string(domain.CodeFormulaState),
		"patch approved formula custom fields"); err != nil {
		closeServer()
		return err
	}

	patchedVessel, err := api.call("PATCH", "/v1/vessels/"+vessel.ID+"/custom-fields",
		customFieldPatch(vesselField.ID, "B-4568"), ptrInt64(vessel.Version))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireStatus(patchedVessel, 200, "patch vessel custom field"); err != nil {
		closeServer()
		return err
	}
	if err := patchedVessel.decode(&vessel); err != nil {
		closeServer()
		return err
	}

	wrongType, err := api.call("PATCH", "/v1/runs/"+run.ID+"/custom-fields",
		customFieldPatch(runField.ID, "wheat"), ptrInt64(run.Version))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(wrongType, 400, string(domain.CodeInvalidInput),
		"patch used custom field with the wrong type"); err != nil {
		closeServer()
		return err
	}

	usedTypeChange, err := api.callWithRevision("PUT", "/v1/extension-fields/"+runField.ID, map[string]any{
		"entity_type": "run",
		"name":        runField.Name,
		"type":        "text",
		"required":    true,
	}, nil, ptrInt64(1))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(usedTypeChange, 409, string(domain.CodeExtensionConflict),
		"change type after custom values were written"); err != nil {
		closeServer()
		return err
	}

	unused, err := createTypedExtensionField(api, "vessel", "Sanitized", "text", false)
	if err != nil {
		closeServer()
		return err
	}
	changedType, err := api.callWithRevision("PUT", "/v1/extension-fields/"+unused.ID, map[string]any{
		"entity_type": "vessel",
		"name":        "Sanitized",
		"type":        "boolean",
		"required":    false,
	}, nil, ptrInt64(1))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireStatus(changedType, 200, "change unused field type"); err != nil {
		closeServer()
		return err
	}
	deactivatedResponse, err := api.callWithRevision("POST",
		"/v1/extension-fields/"+unused.ID+"/deactivate", nil, nil, ptrInt64(2))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireStatus(deactivatedResponse, 200, "deactivate unused field"); err != nil {
		closeServer()
		return err
	}
	if err := deactivatedResponse.decode(&unused); err != nil {
		closeServer()
		return err
	}
	inactiveWrite, err := api.call("PATCH", "/v1/vessels/"+vessel.ID+"/custom-fields",
		customFieldPatch(unused.ID, true), ptrInt64(vessel.Version))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(inactiveWrite, 400, string(domain.CodeInvalidInput),
		"write a deactivated field"); err != nil {
		closeServer()
		return err
	}

	staleUpdate, err := api.callWithRevision("PUT", "/v1/extension-fields/"+formulaField.ID, map[string]any{
		"entity_type": "formula",
		"name":        "Grain Batch",
		"type":        "text",
		"required":    false,
	}, nil, ptrInt64(99))
	if err != nil {
		closeServer()
		return err
	}
	if err := requireCode(staleUpdate, 412, string(domain.CodeRevisionConflict),
		"publish stale field revision"); err != nil {
		closeServer()
		return err
	}

	closeServer()
	return verifyExtensionRestart(dataDir, formula, vessel, run, formulaField, vesselField, runField, unused)
}

func verifyConcurrentExtensionPublishing(api *client) error {
	field, err := createTypedExtensionField(api, "run", "Concurrent Release", "number", false)
	if err != nil {
		return err
	}
	start := make(chan struct{})
	var waiter sync.WaitGroup
	results := make(chan response, 2)
	for range 2 {
		waiter.Add(1)
		go func() {
			defer waiter.Done()
			<-start
			response, callErr := api.callWithRevision("PUT",
				"/v1/extension-fields/"+field.ID, map[string]any{
					"entity_type": "run",
					"name":        "Concurrent Release",
					"type":        "text",
					"required":    false,
				}, nil, ptrInt64(1))
			if callErr == nil {
				results <- response
			}
		}()
	}
	close(start)
	waiter.Wait()
	close(results)
	var statuses []int
	for response := range results {
		statuses = append(statuses, response.status)
	}
	if len(statuses) != 2 || statuses[0] == statuses[1] ||
		(statuses[0] != 200 && statuses[1] != 200) ||
		(statuses[0] != 412 && statuses[1] != 412) {
		return errInvalidWorkflow("concurrent extension publication did not produce one success and one revision conflict")
	}
	return nil
}

func startExtensionServer(dataDir string) (*client, func(), error) {
	store, err := storage.Open(dataDir)
	if err != nil {
		return nil, nil, err
	}
	app := service.New(store, clock.System{})
	server := httptest.NewServer(httpapi.NewHandler(app))
	api := newClient(server.URL)
	return api, server.Close, nil
}

func createExtensionField(
	api *client,
	entityType string,
	name string,
	fieldType string,
	required bool,
) (extensionFieldView, error) {
	return createTypedExtensionField(api, entityType, name, fieldType, required)
}

func createTypedExtensionField(
	api *client,
	entityType string,
	name string,
	fieldType string,
	required bool,
) (extensionFieldView, error) {
	response, err := api.call("POST", "/v1/extension-fields", map[string]any{
		"entity_type": entityType,
		"name":        name,
		"type":        fieldType,
		"required":    required,
	}, nil)
	if err != nil {
		return extensionFieldView{}, err
	}
	if err := requireStatus(response, 201, "create extension field"); err != nil {
		return extensionFieldView{}, err
	}
	var field extensionFieldView
	if err := response.decode(&field); err != nil {
		return extensionFieldView{}, err
	}
	return field, nil
}

func createExtensionFormula(api *client, fieldID string) (extensionEntityView, error) {
	payload := createFormulaPayload("Extension Formula")
	payload["custom_fields"] = map[string]any{fieldID: "malt-lot-42"}
	response, err := api.call("POST", "/v1/formulas", payload, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(response, 201, "create formula with custom field"); err != nil {
		return extensionEntityView{}, err
	}
	var formula extensionEntityView
	return formula, response.decode(&formula)
}

func createExtensionVessel(api *client, fieldID string) (extensionEntityView, error) {
	response, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-EXT-1",
		"capacity_liters": 250,
		"custom_fields": map[string]any{
			fieldID: "B-4567",
		},
	}, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(response, 201, "create vessel with custom field"); err != nil {
		return extensionEntityView{}, err
	}
	var vessel extensionEntityView
	return vessel, response.decode(&vessel)
}

func createExtensionRun(
	api *client,
	formulaID string,
	fieldID string,
) (extensionEntityView, error) {
	response, err := api.call("POST", "/v1/runs", map[string]any{
		"code":                  "RUN-EXT-ALLERGEN",
		"formula_id":            formulaID,
		"planned_volume_liters": 180,
		"notes":                 "extension check",
		"custom_fields": map[string]any{
			fieldID: true,
		},
	}, nil)
	if err != nil {
		return extensionEntityView{}, err
	}
	if err := requireStatus(response, 201, "create run with custom field"); err != nil {
		return extensionEntityView{}, err
	}
	var run extensionEntityView
	return run, response.decode(&run)
}

func reserveAndStart(
	api *client,
	runID string,
	vesselID string,
	runVersion int64,
) (extensionEntityView, extensionEntityView, error) {
	reserveResponse, err := api.call("POST", "/v1/runs/"+runID+"/reserve",
		map[string]any{"vessel_id": vesselID}, ptrInt64(runVersion))
	if err != nil {
		return extensionEntityView{}, extensionEntityView{}, err
	}
	if err := requireStatus(reserveResponse, 200, "reserve extension run"); err != nil {
		return extensionEntityView{}, extensionEntityView{}, err
	}
	var reserved pairView
	if err := reserveResponse.decode(&reserved); err != nil {
		return extensionEntityView{}, extensionEntityView{}, err
	}
	startResponse, err := api.call("POST", "/v1/runs/"+runID+"/start",
		nil, ptrInt64(reserved.Run.Version))
	if err != nil {
		return extensionEntityView{}, extensionEntityView{}, err
	}
	if err := requireStatus(startResponse, 200, "start extension run"); err != nil {
		return extensionEntityView{}, extensionEntityView{}, err
	}
	var started pairView
	if err := startResponse.decode(&started); err != nil {
		return extensionEntityView{}, extensionEntityView{}, err
	}
	return extensionEntityView{ID: started.Run.ID, Version: started.Run.Version},
		extensionEntityView{ID: started.Vessel.ID, Version: started.Vessel.Version}, nil
}

func customFieldPatch(id string, value any) map[string]any {
	return map[string]any{"custom_fields": map[string]any{id: value}}
}

func verifyExtensionRestart(
	dataDir string,
	formula extensionEntityView,
	vessel extensionEntityView,
	run extensionEntityView,
	formulaField extensionFieldView,
	vesselField extensionFieldView,
	runField extensionFieldView,
	inactiveField extensionFieldView,
) error {
	api, closeServer, err := startExtensionServer(dataDir)
	if err != nil {
		return err
	}
	defer closeServer()

	if err := requireReadCustomFields(api, "/v1/formulas/"+formula.ID,
		formulaField.ID, "malt-lot-42"); err != nil {
		return err
	}
	if err := requireReadCustomFields(api, "/v1/vessels/"+vessel.ID,
		vesselField.ID, "B-4568"); err != nil {
		return err
	}
	if err := requireReadCustomFields(api, "/v1/runs/"+run.ID,
		runField.ID, true); err != nil {
		return err
	}
	if err := requireListedCustomFields(api, "/v1/formulas", "formulas",
		formula.ID, formulaField.ID, "malt-lot-42"); err != nil {
		return err
	}
	if err := requireListedCustomFields(api, "/v1/vessels", "vessels",
		vessel.ID, vesselField.ID, "B-4568"); err != nil {
		return err
	}
	if err := requireListedCustomFields(api, "/v1/runs", "runs",
		run.ID, runField.ID, true); err != nil {
		return err
	}
	if err := requireWritesUseRestartedDefinitions(api, vessel, vesselField.ID); err != nil {
		return err
	}

	definitionResponse, err := api.call("GET",
		"/v1/extension-fields/"+inactiveField.ID, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(definitionResponse, 200, "read deactivated definition after restart"); err != nil {
		return err
	}
	var restartedInactive extensionFieldView
	if err := definitionResponse.decode(&restartedInactive); err != nil {
		return err
	}
	if restartedInactive.Active || restartedInactive.Type != domain.FieldBoolean ||
		restartedInactive.Revision != 3 {
		return errInvalidWorkflow("deactivated or retyped definition was not retained")
	}

	listResponse, err := api.call("GET", "/v1/extension-fields", nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(listResponse, 200, "list definitions after restart"); err != nil {
		return err
	}
	var definitions extensionListResponse
	if err := listResponse.decode(&definitions); err != nil {
		return err
	}
	counts := map[string]int{}
	for _, field := range definitions.ExtensionFields {
		counts[string(field.EntityType)]++
		if field.ID == runField.ID && (!field.Active || field.Type != domain.FieldBoolean || !field.Used) {
			return errInvalidWorkflow("run field definition was not preserved after restart")
		}
	}
	if counts["formula"] != 1 || counts["vessel"] != 2 || counts["run"] != 2 {
		return errInvalidWorkflow("restarted definition registry does not contain all fields")
	}

	healthResponse, err := api.call("GET", "/healthz", nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(healthResponse, 200, "health after restart"); err != nil {
		return err
	}
	return nil
}

func requireWritesUseRestartedDefinitions(
	api *client,
	vessel extensionEntityView,
	vesselFieldID string,
) error {
	missingAfterRestart, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-EXT-AFTER",
		"capacity_liters": 220,
	}, nil)
	if err != nil {
		return err
	}
	if err := requireCode(missingAfterRestart, 400, string(domain.CodeInvalidInput),
		"create without required custom field after restart"); err != nil {
		return err
	}
	createdAfterRestart, err := api.call("POST", "/v1/vessels", map[string]any{
		"code":            "VSL-EXT-AFTER",
		"capacity_liters": 220,
		"custom_fields": map[string]any{
			vesselFieldID: "B-7777",
		},
	}, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(createdAfterRestart, 201,
		"create with required custom field after restart"); err != nil {
		return err
	}
	patchAfterRestart, err := api.call("PATCH", "/v1/vessels/"+vessel.ID+"/custom-fields",
		customFieldPatch(vesselFieldID, "B-4569"), ptrInt64(vessel.Version))
	if err != nil {
		return err
	}
	if err := requireStatus(patchAfterRestart, 200,
		"patch historical entity using restarted definition"); err != nil {
		return err
	}
	return nil
}

func requireReadCustomFields(
	api *client,
	path string,
	fieldID string,
	expected any,
) error {
	response, err := api.call("GET", path, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(response, 200, "read extension entity"); err != nil {
		return err
	}
	var entity extensionEntityView
	if err := response.decode(&entity); err != nil {
		return err
	}
	return requireCustomValue(entity.CustomFields, fieldID, expected)
}

func requireListedCustomFields(
	api *client,
	path string,
	key string,
	entityID string,
	fieldID string,
	expected any,
) error {
	response, err := api.call("GET", path, nil, nil)
	if err != nil {
		return err
	}
	if err := requireStatus(response, 200, "list extension entities"); err != nil {
		return err
	}
	var raw map[string][]extensionEntityView
	if err := json.Unmarshal(response.body, &raw); err != nil {
		return err
	}
	for _, entity := range raw[key] {
		if entity.ID == entityID {
			return requireCustomValue(entity.CustomFields, fieldID, expected)
		}
	}
	return fmt.Errorf("entity %s was absent from %s after restart", entityID, path)
}

func requireCustomValue(
	values map[string]domain.CustomFieldValue,
	fieldID string,
	expected any,
) error {
	value, ok := values[fieldID]
	if !ok {
		return errInvalidWorkflow("historical custom field was not readable")
	}
	if fmt.Sprint(value.Value) != fmt.Sprint(expected) {
		return errInvalidWorkflow("historical custom field value changed")
	}
	return nil
}
