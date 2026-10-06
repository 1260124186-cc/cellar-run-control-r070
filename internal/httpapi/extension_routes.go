package httpapi

import (
	"net/http"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type createExtensionFieldRequest struct {
	EntityType domain.ExtensionEntity    `json:"entity_type"`
	Name       string                    `json:"name"`
	Type       domain.ExtensionFieldType `json:"type"`
	Required   bool                      `json:"required"`
}

type updateExtensionFieldRequest struct {
	Name     string                    `json:"name"`
	Type     domain.ExtensionFieldType `json:"type"`
	Required bool                      `json:"required"`
}

func registerExtensionRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/extension-fields", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createExtensionFieldRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		field, err := app.CreateExtensionField(service.CreateExtensionFieldInput{
			EntityType: body.EntityType,
			Name:       body.Name,
			Type:       body.Type,
			Required:   body.Required,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, field)
	})

	mux.HandleFunc("GET /v1/extension-fields", func(response http.ResponseWriter, request *http.Request) {
		entity := domain.ExtensionEntity(request.URL.Query().Get("entity_type"))
		fields, err := app.ListExtensionFields(entity)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"fields": fields})
	})

	mux.HandleFunc("PUT /v1/extension-fields/{id}", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[updateExtensionFieldRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		field, err := app.UpdateExtensionField(request.PathValue("id"), service.UpdateExtensionFieldInput{
			ExpectedVersion: expected,
			Name:            body.Name,
			Type:            body.Type,
			Required:        body.Required,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, field)
	})

	mux.HandleFunc("POST /v1/extension-fields/{id}/deactivate", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedVersion(request)
		if !handleDecode(response, err) {
			return
		}
		field, err := app.DeactivateExtensionField(request.PathValue("id"),
			service.VersionInput{ExpectedVersion: expected})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, field)
	})
}
