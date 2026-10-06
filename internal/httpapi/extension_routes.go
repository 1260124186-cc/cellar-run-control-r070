package httpapi

import (
	"net/http"
	"strconv"

	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/domain"
	"github.com/1260124186-cc/solo-0016-cellar-run-control/internal/service"
)

type createExtensionFieldRequest struct {
	EntityType string `json:"entity_type"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Required   bool   `json:"required"`
}

type updateExtensionFieldRequest struct {
	EntityType string `json:"entity_type"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Required   bool   `json:"required"`
}

func registerExtensionRoutes(mux *http.ServeMux, app *service.Service) {
	mux.HandleFunc("POST /v1/extension-fields", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[createExtensionFieldRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		field, err := app.CreateExtensionField(service.CreateExtensionFieldInput{
			EntityType: domain.NormalizeEntityKind(body.EntityType),
			Name:       body.Name,
			Type:       domain.NormalizeFieldType(body.Type),
			Required:   body.Required,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusCreated, field)
	})

	mux.HandleFunc("GET /v1/extension-fields", func(response http.ResponseWriter, request *http.Request) {
		fields, err := app.ListExtensionFields(request.URL.Query().Get("entity_type"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"extension_fields": fields})
	})

	mux.HandleFunc("GET /v1/extension-fields/{id}", func(response http.ResponseWriter, request *http.Request) {
		field, err := app.GetExtensionField(request.PathValue("id"))
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, field)
	})

	mux.HandleFunc("PUT /v1/extension-fields/{id}", func(response http.ResponseWriter, request *http.Request) {
		body, err := decodeJSON[updateExtensionFieldRequest](response, request)
		if !handleDecode(response, err) {
			return
		}
		expected, err := expectedRevisionHeader(request)
		if !handleDecode(response, err) {
			return
		}
		field, err := app.UpdateExtensionField(request.PathValue("id"), service.UpdateExtensionFieldInput{
			ExpectedRevision: expected,
			Name:             body.Name,
			Type:             domain.NormalizeFieldType(body.Type),
			Required:         body.Required,
		})
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, field)
	})

	mux.HandleFunc("POST /v1/extension-fields/{id}/deactivate", func(response http.ResponseWriter, request *http.Request) {
		expected, err := expectedRevisionHeader(request)
		if !handleDecode(response, err) {
			return
		}
		field, err := app.DeactivateExtensionField(request.PathValue("id"), expected)
		if err != nil {
			writeDomainError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, field)
	})
}

func expectedRevisionHeader(request *http.Request) (*int64, error) {
	value := request.Header.Get("X-Expected-Revision")
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 1 {
		return nil, domain.FieldError(domain.CodeInvalidInput,
			"X-Expected-Revision", "must be a positive integer")
	}
	return &parsed, nil
}
