package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/provision"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AssistantInstanceAPI interface {
	Overview(context.Context, uuid.UUID) (provision.Overview, error)
	Create(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool) (provision.Instance, error)
	Stop(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID) (provision.Instance, error)
}

// registerAssistantInstanceRoutes lets an admin create, see and stop the machine (the containers) of a shop's assistant. Creating one or
// stopping one needs the admin's password again. The assistant token is made and kept sealed on the server side; no answer ever carries it.
func registerAssistantInstanceRoutes(router chi.Router, adminAuth AdminAuthenticator, instances AssistantInstanceAPI) {
	path := "/api/v1/admin/tenants/{tenantId}/assistant/instance"
	router.Get(path, func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		overview, err := instances.Overview(request.Context(), tenantID)
		if handleAssistantInstanceError(response, request, err) {
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, overview)
	})
	router.Post(path, func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		var input struct {
			RecipientID   string `json:"recipientId"`
			NamesVisible  *bool  `json:"namesVisible"`
			AdminPassword string `json:"adminPassword"`
		}
		if err := decodeJSON(response, request, &input); err != nil || input.NamesVisible == nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Assistant instance input is invalid.", false)
			return
		}
		recipientID, err := uuid.Parse(input.RecipientID)
		if err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Recipient ID must be a UUID.", false)
			return
		}
		if !confirmAdminPassword(response, request, adminAuth, admin, input.AdminPassword) {
			return
		}
		instance, err := instances.Create(request.Context(), admin.TokenHash, requestID(request), tenantID, recipientID, *input.NamesVisible)
		if handleAssistantInstanceError(response, request, err) {
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusAccepted, provision.Overview{Instance: &instance})
	})
	router.Post(path+"/stop", func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return
		}
		var input struct {
			AdminPassword string `json:"adminPassword"`
		}
		if err := decodeJSON(response, request, &input); err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Assistant instance input is invalid.", false)
			return
		}
		if !confirmAdminPassword(response, request, adminAuth, admin, input.AdminPassword) {
			return
		}
		instance, err := instances.Stop(request.Context(), admin.TokenHash, requestID(request), tenantID)
		if handleAssistantInstanceError(response, request, err) {
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusAccepted, provision.Overview{Instance: &instance})
	})
}

func handleAssistantInstanceError(response http.ResponseWriter, request *http.Request, err error) bool {
	if err == nil {
		return false
	}
	var invalid *provision.ValidationError
	var settingsInvalid *assistantcfg.ValidationError
	switch {
	case errors.As(err, &invalid):
		writeValidationProblem(response, request, &tenant.ValidationError{Field: invalid.Field, Code: invalid.Code, Message: "Assistant instance input is invalid."})
	case errors.As(err, &settingsInvalid):
		writeValidationProblem(response, request, &tenant.ValidationError{Field: settingsInvalid.Field, Code: settingsInvalid.Code, Message: "Assistant setting is invalid."})
	case errors.Is(err, provision.ErrNotFound), errors.Is(err, assistantcfg.ErrTenantNotFound), errors.Is(err, agent.ErrRecipientNotFound):
		writeProblem(response, request, http.StatusNotFound, "NOT_FOUND", "Shop, recipient or assistant was not found.", false)
	case errors.Is(err, provision.ErrAlreadyExists):
		writeProblem(response, request, http.StatusConflict, "ALREADY_EXISTS", "ร้านนี้มีเลขา AI ที่สร้างไว้แล้ว", false)
	case errors.Is(err, provision.ErrHasManualAssistant):
		writeProblem(response, request, http.StatusConflict, "HAS_MANUAL_ASSISTANT", "ร้านนี้มีเลขา AI ที่ตั้งไว้ด้วยมือบนเซิร์ฟเวอร์แล้ว สร้างซ้ำไม่ได้", false)
	case errors.Is(err, agent.ErrAIChatDisabled):
		writeProblem(response, request, http.StatusConflict, "AI_CHAT_DISABLED", "ต้องเปิดสิทธิ์ “คุยกับผู้ช่วย AI” ให้ผู้รับนี้ก่อน", false)
	case errors.Is(err, assistantcfg.ErrVersionConflict):
		writeProblem(response, request, http.StatusConflict, "VERSION_CONFLICT", "Assistant settings changed since they were loaded. Reload before trying again.", false)
	case errors.Is(err, assistantcfg.ErrHostInUse):
		writeProblem(response, request, http.StatusConflict, "HOST_IN_USE", "Assistant host is used by another shop.", false)
	default:
		writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to process the assistant instance.", true)
	}
	return true
}
