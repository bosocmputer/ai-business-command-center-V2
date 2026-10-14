package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type EngagementAPI interface {
	Get(context.Context, uuid.UUID) (engagement.Settings, error)
	Put(context.Context, []byte, string, uuid.UUID, engagement.Settings) (engagement.Settings, error)
}

// registerEngagementRoutes opens the per-shop options for looking after the owner to the admin. They hold no secret, so a saved change needs
// the session and the CSRF token (like the refresh policy), and writes an audit record.
func registerEngagementRoutes(router chi.Router, adminAuth AdminAuthenticator, settings EngagementAPI) {
	router.Get("/api/v1/admin/tenants/{tenantId}/engagement", func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		current, err := settings.Get(request.Context(), tenantID)
		if handleEngagementError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, current)
	})

	router.Put("/api/v1/admin/tenants/{tenantId}/engagement", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		if !isJSONRequest(request) {
			writeProblem(response, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json.", false)
			return
		}
		var input engagement.Settings
		if err := decodeJSON(response, request, &input); err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Engagement settings are invalid.", false)
			return
		}
		saved, err := settings.Put(request.Context(), admin.TokenHash, requestID(request), tenantID, input)
		if handleEngagementError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, saved)
	})
}

func handleEngagementError(response http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, engagement.ErrInvalid):
		writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Engagement settings are invalid.", false)
	case errors.Is(err, engagement.ErrConflict):
		writeProblem(response, request, http.StatusConflict, "VERSION_CONFLICT", "Engagement settings changed in another session.", false)
	case errors.Is(err, engagement.ErrNotFound):
		writeProblem(response, request, http.StatusNotFound, "TENANT_NOT_FOUND", "Tenant was not found.", false)
	default:
		writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to manage engagement settings.", false)
	}
	return true
}
