package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/executionmode"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ExecutionModeAPI interface {
	List(context.Context, uuid.UUID) ([]executionmode.Item, error)
	Set(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, key report.Key, mode report.ExecutionMode, reason string) (executionmode.Item, error)
	Measure(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID) ([]executionmode.Measurement, error)
}

func registerExecutionModeRoutes(router interface {
	Get(string, http.HandlerFunc)
	Put(string, http.HandlerFunc)
	Post(string, http.HandlerFunc)
}, adminAuth AdminAuthenticator, modes ExecutionModeAPI) {
	router.Get("/api/v1/admin/tenants/{tenantId}/report-modes", func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		items, err := modes.List(request.Context(), tenantID)
		if handleExecutionModeError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"data": items})
	})

	router.Put("/api/v1/admin/tenants/{tenantId}/report-modes/{reportKey}", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		key := report.Key(chi.URLParam(request, "reportKey"))
		if _, known := report.DefinitionFor(key); !known {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Report key is invalid.", false)
			return
		}
		var input struct {
			Mode   report.ExecutionMode `json:"mode"`
			Reason string               `json:"reason"`
		}
		if err := decodeJSON(response, request, &input); err != nil || !input.Mode.Valid() {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Report mode input is invalid.", false)
			return
		}
		item, err := modes.Set(request.Context(), admin.TokenHash, requestID(request), tenantID, key, input.Mode, input.Reason)
		if handleExecutionModeError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, item)
	})

	router.Post("/api/v1/admin/tenants/{tenantId}/report-modes/measure", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		results, err := modes.Measure(request.Context(), admin.TokenHash, requestID(request), tenantID)
		if handleExecutionModeError(response, request, err) {
			return
		}
		writeJSON(response, http.StatusOK, map[string]any{"data": results})
	})
}

func handleExecutionModeError(response http.ResponseWriter, request *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, executionmode.ErrUnsupported):
		writeProblem(response, request, http.StatusUnprocessableEntity, "REPORT_MODE_UNSUPPORTED", "This report cannot be split into chunks.", false)
	case errors.Is(err, executionmode.ErrTenantNotFound):
		writeProblem(response, request, http.StatusNotFound, "NOT_FOUND", "Tenant was not found.", false)
	case errors.Is(err, executionmode.ErrTenantBusy):
		writeProblem(response, request, http.StatusConflict, "TENANT_BUSY", "A report is running for this shop. Try again when it finishes.", true)
	case errors.Is(err, sml.ErrConnectionNotConfigured):
		writeProblem(response, request, http.StatusConflict, "SML_NOT_CONFIGURED", "This shop has no SML connection yet.", false)
	default:
		writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Report modes are unavailable.", true)
	}
	return true
}
