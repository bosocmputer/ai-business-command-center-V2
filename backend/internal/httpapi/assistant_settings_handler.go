package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AssistantSettingsAPI interface {
	View(context.Context, uuid.UUID) (assistantcfg.AdminView, error)
	Update(context.Context, []byte, string, uuid.UUID, assistantcfg.UpdateInput) (assistantcfg.AdminView, error)
	SetSecret(context.Context, []byte, string, uuid.UUID, assistantcfg.Field, string) (assistantcfg.AdminView, error)
	ClearSecret(context.Context, []byte, string, uuid.UUID, assistantcfg.Field) (assistantcfg.AdminView, error)
	GlobalView(context.Context) (assistantcfg.GlobalView, error)
	SetGlobalSecret(context.Context, []byte, string, assistantcfg.Field, string) (assistantcfg.GlobalView, error)
}

// registerAssistantSettingsRoutes opens the per-shop assistant settings to the admin. Nothing here ever returns a secret. Setting or
// clearing one needs the admin's password again, on top of the session and the CSRF token.
func registerAssistantSettingsRoutes(router chi.Router, adminAuth AdminAuthenticator, settings AssistantSettingsAPI) {
	confirm := func(response http.ResponseWriter, request *http.Request, admin auth.AuthenticatedAdmin, password string) bool {
		if len(password) < 1 || len(password) > auth.MaximumAdminPasswordBytes {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Admin password is required.", false)
			return false
		}
		switch err := adminAuth.ConfirmPassword(request.Context(), admin, password); {
		case err == nil:
			return true
		case errors.Is(err, auth.ErrLoginLocked):
			writeProblem(response, request, http.StatusTooManyRequests, "PASSWORD_CONFIRMATION_LOCKED", "Too many wrong passwords. Try again later.", true)
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeProblem(response, request, http.StatusForbidden, "PASSWORD_CONFIRMATION_FAILED", "The admin password is not correct.", false)
		default:
			writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to confirm the password.", true)
		}
		return false
	}
	field := func(response http.ResponseWriter, request *http.Request) (assistantcfg.Field, bool) {
		parsed, ok := assistantcfg.ParseField(chi.URLParam(request, "field"))
		if !ok {
			writeProblem(response, request, http.StatusNotFound, "NOT_FOUND", "Unknown assistant secret.", false)
		}
		return parsed, ok
	}

	router.Get("/api/v1/admin/tenants/{tenantId}/assistant", func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		view, err := settings.View(request.Context(), tenantID)
		if handleAssistantSettingsError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, view)
	})

	router.Put("/api/v1/admin/tenants/{tenantId}/assistant", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		var input struct {
			Enabled  *bool   `json:"enabled"`
			IsTest   *bool   `json:"isTest"`
			ModelKey *string `json:"modelKey"`
			LineMode *string `json:"lineMode"`
			Version  int     `json:"version"`
		}
		if err := decodeJSON(response, request, &input); err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Assistant settings input is invalid.", false)
			return
		}
		view, err := settings.Update(request.Context(), admin.TokenHash, requestID(request), tenantID, assistantcfg.UpdateInput{
			Enabled: input.Enabled, IsTest: input.IsTest, ModelKey: input.ModelKey, LineMode: input.LineMode, Version: input.Version,
		})
		if handleAssistantSettingsError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, view)
	})

	router.Put("/api/v1/admin/tenants/{tenantId}/assistant/secrets/{field}", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		name, ok := field(response, request)
		if !ok {
			return
		}
		var input struct {
			Value         string `json:"value"`
			AdminPassword string `json:"adminPassword"`
		}
		if err := decodeJSON(response, request, &input); err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Secret input is invalid.", false)
			return
		}
		// The format is checked before the password so a typo does not use up an attempt.
		if err := assistantcfg.ValidateSecret(name, input.Value); handleAssistantSettingsError(response, request, err) {
			return
		}
		if !confirm(response, request, admin, input.AdminPassword) {
			return
		}
		view, err := settings.SetSecret(request.Context(), admin.TokenHash, requestID(request), tenantID, name, input.Value)
		if handleAssistantSettingsError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, view)
	})

	router.Post("/api/v1/admin/tenants/{tenantId}/assistant/secrets/{field}/clear", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		name, ok := field(response, request)
		if !ok {
			return
		}
		var input struct {
			AdminPassword string `json:"adminPassword"`
		}
		if err := decodeJSON(response, request, &input); err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Input is invalid.", false)
			return
		}
		if !confirm(response, request, admin, input.AdminPassword) {
			return
		}
		view, err := settings.ClearSecret(request.Context(), admin.TokenHash, requestID(request), tenantID, name)
		if handleAssistantSettingsError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, view)
	})

	router.Get("/api/v1/admin/assistant/global", func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		view, err := settings.GlobalView(request.Context())
		if handleAssistantSettingsError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, view)
	})

	router.Put("/api/v1/admin/assistant/global/secrets/{field}", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		name, ok := field(response, request)
		if !ok {
			return
		}
		var input struct {
			Value         string `json:"value"`
			AdminPassword string `json:"adminPassword"`
		}
		if err := decodeJSON(response, request, &input); err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Secret input is invalid.", false)
			return
		}
		if name != assistantcfg.FieldLineSecret && name != assistantcfg.FieldLineToken {
			writeValidationProblem(response, request, &tenant.ValidationError{Field: string(name), Code: "NOT_A_GLOBAL_FIELD", Message: "Only the LINE channel is shared between shops."})
			return
		}
		if err := assistantcfg.ValidateSecret(name, input.Value); handleAssistantSettingsError(response, request, err) {
			return
		}
		if !confirm(response, request, admin, input.AdminPassword) {
			return
		}
		view, err := settings.SetGlobalSecret(request.Context(), admin.TokenHash, requestID(request), name, input.Value)
		if handleAssistantSettingsError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, view)
	})
}

func handleAssistantSettingsError(response http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	var invalid *assistantcfg.ValidationError
	switch {
	case errors.As(err, &invalid):
		// Only the field and a code travel back, never the value that was refused.
		writeValidationProblem(response, request, &tenant.ValidationError{Field: invalid.Field, Code: invalid.Code, Message: "Assistant setting is invalid."})
	case errors.Is(err, assistantcfg.ErrTenantNotFound):
		writeProblem(response, request, http.StatusNotFound, "NOT_FOUND", "Shop was not found.", false)
	case errors.Is(err, assistantcfg.ErrVersionConflict):
		writeProblem(response, request, http.StatusConflict, "VERSION_CONFLICT", "Assistant settings changed since they were loaded. Reload before saving again.", false)
	default:
		writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to process the assistant settings.", false)
	}
	return true
}
