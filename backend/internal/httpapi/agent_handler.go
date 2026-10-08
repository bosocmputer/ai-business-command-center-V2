package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AgentAPI is what the assistant routes and the token admin routes need from the agent service.
type AgentAPI interface {
	Authenticate(ctx context.Context, rawToken string) (agent.Principal, error)
	Context(ctx context.Context, principal agent.Principal) (agent.ContextResponse, error)
	Report(ctx context.Context, principal agent.Principal, reportKey, dateFrom, dateTo string) (agent.ReportResponse, error)
	Compare(ctx context.Context, principal agent.Principal, reportKey, metric, aFrom, aTo, bFrom, bTo string) (agent.CompareResponse, error)
	LatestDelivery(ctx context.Context, principal agent.Principal, reportKey string) (agent.ReportResponse, error)
	Alerts(ctx context.Context, principal agent.Principal) (agent.AlertsResponse, error)
	SetAlert(ctx context.Context, principal agent.Principal, rule string, request agent.AlertSetRequest, requestID string) (agent.AlertView, error)
	IssueToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool) (agent.IssuedToken, error)
	RevokeToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID) error
	TokenInfo(ctx context.Context, tenantID, recipientID uuid.UUID) (agent.TokenInfo, error)
}

type agentPrincipalKey struct{}

// Agent errors are written by hand, not with writeProblem, because writeProblem carries a request id and the
// agent contract needs two answers to be identical byte for byte: an unknown report and a forbidden one.
func writeAgentJSON(response http.ResponseWriter, status int, body any) {
	response.Header().Set("Cache-Control", "no-store")
	writeJSON(response, status, body)
}

type agentError struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func registerAgentRoutes(router chi.Router, api AgentAPI, enabled bool) {
	router.Route("/api/v1/agent", func(group chi.Router) {
		group.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if !enabled {
					writeAgentJSON(response, http.StatusServiceUnavailable, agentError{Status: "AGENT_DISABLED", Message: "ผู้ช่วย AI ปิดใช้งานอยู่"})
					return
				}
				header := request.Header.Get("Authorization")
				token, found := strings.CutPrefix(header, "Bearer ")
				if !found {
					writeAgentJSON(response, http.StatusUnauthorized, agentError{Status: "UNAUTHORIZED", Message: agent.MessageUnauthorized})
					return
				}
				principal, err := api.Authenticate(request.Context(), token)
				var limited *agent.RateLimitedError
				switch {
				case errors.As(err, &limited):
					response.Header().Set("Retry-After", strconv.Itoa(int(limited.RetryAfter.Seconds())))
					writeAgentJSON(response, http.StatusTooManyRequests, agentError{Status: "RATE_LIMITED", Message: agent.MessageRateLimited})
					return
				case errors.Is(err, agent.ErrUnauthorized):
					writeAgentJSON(response, http.StatusUnauthorized, agentError{Status: "UNAUTHORIZED", Message: agent.MessageUnauthorized})
					return
				case err != nil:
					writeAgentJSON(response, http.StatusInternalServerError, agentError{Status: "ERROR", Message: "ระบบขัดข้อง ลองใหม่ภายหลัง"})
					return
				}
				next.ServeHTTP(response, request.WithContext(context.WithValue(request.Context(), agentPrincipalKey{}, principal)))
			})
		})
		group.Get("/context", func(response http.ResponseWriter, request *http.Request) {
			result, err := api.Context(request.Context(), agentPrincipal(request))
			respondAgent(response, result, err)
		})
		group.Get("/reports/{reportKey}", func(response http.ResponseWriter, request *http.Request) {
			query := request.URL.Query()
			result, err := api.Report(request.Context(), agentPrincipal(request), chi.URLParam(request, "reportKey"), query.Get("dateFrom"), query.Get("dateTo"))
			respondAgent(response, result, err)
		})
		group.Get("/compare", func(response http.ResponseWriter, request *http.Request) {
			query := request.URL.Query()
			result, err := api.Compare(request.Context(), agentPrincipal(request), query.Get("reportKey"), query.Get("metric"), query.Get("aFrom"), query.Get("aTo"), query.Get("bFrom"), query.Get("bTo"))
			respondAgent(response, result, err)
		})
		group.Get("/deliveries/latest", func(response http.ResponseWriter, request *http.Request) {
			result, err := api.LatestDelivery(request.Context(), agentPrincipal(request), request.URL.Query().Get("reportKey"))
			respondAgent(response, result, err)
		})
		group.Get("/alerts", func(response http.ResponseWriter, request *http.Request) {
			result, err := api.Alerts(request.Context(), agentPrincipal(request))
			respondAgent(response, result, err)
		})
		group.Put("/alerts/{ruleKey}", func(response http.ResponseWriter, request *http.Request) {
			var input agent.AlertSetRequest
			decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 4096))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				writeAgentJSON(response, http.StatusUnprocessableEntity, agentError{Status: "INVALID_ALERT", Message: "คำขอตั้งการเตือนไม่ถูกต้อง ต้องมี threshold (ตัวเลข) และ enabled (true หรือ false)"})
				return
			}
			result, err := api.SetAlert(request.Context(), agentPrincipal(request), chi.URLParam(request, "ruleKey"), input, requestID(request))
			respondAgent(response, result, err)
		})
		group.NotFound(func(response http.ResponseWriter, _ *http.Request) {
			writeAgentJSON(response, http.StatusNotFound, agentError{Status: "NO_DATA", Message: agent.MessageNoData})
		})
	})
}

func isInvalidAlert(err error) bool {
	var invalid *agent.InvalidAlertError
	return errors.As(err, &invalid)
}

func agentPrincipal(request *http.Request) agent.Principal {
	principal, _ := request.Context().Value(agentPrincipalKey{}).(agent.Principal)
	return principal
}

func respondAgent(response http.ResponseWriter, result any, err error) {
	switch {
	case err == nil:
		writeAgentJSON(response, http.StatusOK, result)
	case errors.Is(err, agent.ErrNoData), errors.Is(err, agent.ErrAlertsUnavailable):
		writeAgentJSON(response, http.StatusNotFound, agentError{Status: "NO_DATA", Message: agent.MessageNoData})
	case isInvalidAlert(err):
		var invalid *agent.InvalidAlertError
		_ = errors.As(err, &invalid)
		writeAgentJSON(response, http.StatusUnprocessableEntity, agentError{Status: "INVALID_ALERT", Message: invalid.Message})
	case errors.Is(err, agent.ErrInvalidPeriod):
		writeAgentJSON(response, http.StatusUnprocessableEntity, agentError{Status: "INVALID_PERIOD", Message: agent.MessageInvalidDates})
	default:
		writeAgentJSON(response, http.StatusInternalServerError, agentError{Status: "ERROR", Message: "ระบบขัดข้อง ลองใหม่ภายหลัง"})
	}
}

type agentTokenStatus struct {
	Enabled bool            `json:"enabled"`
	Info    agent.TokenInfo `json:"info"`
}

// registerAgentAdminRoutes lets an admin see, issue and revoke a recipient's assistant token. The token is in the
// response of the issue call only.
func registerAgentAdminRoutes(router chi.Router, adminAuth AdminAuthenticator, api AgentAPI, enabled bool) {
	path := "/api/v1/admin/tenants/{tenantId}/recipients/{recipientId}/agent-token"
	parse := func(response http.ResponseWriter, request *http.Request) (uuid.UUID, uuid.UUID, bool) {
		tenantID, ok := parseTenantID(response, request)
		if !ok {
			return uuid.Nil, uuid.Nil, false
		}
		recipientID, err := uuid.Parse(chi.URLParam(request, "recipientId"))
		if err != nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Recipient ID must be a UUID.", false)
			return uuid.Nil, uuid.Nil, false
		}
		return tenantID, recipientID, true
	}
	router.Get(path, func(response http.ResponseWriter, request *http.Request) {
		if _, ok := operationalAdmin(response, request, adminAuth, false); !ok {
			return
		}
		tenantID, recipientID, ok := parse(response, request)
		if !ok {
			return
		}
		info, err := api.TokenInfo(request.Context(), tenantID, recipientID)
		if err != nil {
			writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to read the assistant token.", true)
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, agentTokenStatus{Enabled: enabled, Info: info})
	})
	router.Post(path, func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, recipientID, ok := parse(response, request)
		if !ok {
			return
		}
		var input struct {
			NamesVisible *bool `json:"namesVisible"`
		}
		if err := decodeJSON(response, request, &input); err != nil || input.NamesVisible == nil {
			writeProblem(response, request, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Assistant token input is invalid.", false)
			return
		}
		if !enabled {
			writeProblem(response, request, http.StatusConflict, "AGENT_DISABLED", "ผู้ช่วย AI ยังปิดใช้งานอยู่ในระบบ จึงออกโทเคนไม่ได้", false)
			return
		}
		issued, err := api.IssueToken(request.Context(), admin.TokenHash, requestID(request), tenantID, recipientID, *input.NamesVisible)
		switch {
		case errors.Is(err, agent.ErrAIChatDisabled):
			writeProblem(response, request, http.StatusConflict, "AI_CHAT_DISABLED", "ต้องเปิดสิทธิ์ “คุยกับผู้ช่วย AI” ให้ผู้รับนี้ก่อนจึงจะออกโทเคนได้", false)
		case errors.Is(err, agent.ErrRecipientNotFound):
			writeProblem(response, request, http.StatusNotFound, "NOT_FOUND", "ไม่พบผู้รับที่ยืนยันแล้วในร้านนี้", false)
		case err != nil:
			writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to issue the assistant token.", true)
		default:
			response.Header().Set("Cache-Control", "no-store")
			writeJSON(response, http.StatusCreated, issued)
		}
	})
	router.Delete(path, func(response http.ResponseWriter, request *http.Request) {
		admin, ok := operationalAdmin(response, request, adminAuth, true)
		if !ok {
			return
		}
		tenantID, recipientID, ok := parse(response, request)
		if !ok {
			return
		}
		if err := api.RevokeToken(request.Context(), admin.TokenHash, requestID(request), tenantID, recipientID); err != nil {
			writeProblem(response, request, http.StatusInternalServerError, "INTERNAL_ERROR", "Unable to revoke the assistant token.", true)
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, agentTokenStatus{Enabled: enabled, Info: agent.TokenInfo{Status: "NONE"}})
	})
}
