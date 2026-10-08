package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

type agentTestHasher struct{}

func (agentTestHasher) HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

type agentTestStore struct {
	principal agent.Principal
	valid     string
	permitted []report.Key
	used      int
	issueErr  error
	issued    int
	revoked   int
}

func (store *agentTestStore) Authenticate(_ context.Context, hash []byte, _ time.Time) (agent.Principal, error) {
	if string(hash) == string(agentTestHasher{}.HashToken(store.valid)) {
		return store.principal, nil
	}
	return agent.Principal{}, agent.ErrUnauthorized
}
func (store *agentTestStore) TouchToken(context.Context, uuid.UUID, time.Time) error { return nil }
func (store *agentTestStore) CallsSince(context.Context, uuid.UUID, time.Time) (int, error) {
	return store.used, nil
}
func (store *agentTestStore) PreparingSince(context.Context, uuid.UUID, time.Time) (int, error) {
	return 0, nil
}
func (store *agentTestStore) RecordCall(context.Context, agent.Call, time.Time) error { return nil }
func (store *agentTestStore) PermittedReports(context.Context, agent.Principal, time.Time) ([]report.Key, error) {
	return store.permitted, nil
}
func (store *agentTestStore) LatestDelivery(context.Context, agent.Principal, report.Key) (agent.Delivered, error) {
	return agent.Delivered{}, agent.ErrNoData
}
func (store *agentTestStore) IssueToken(_ context.Context, _ []byte, _ string, _, _ uuid.UUID, namesVisible bool, _ []byte, expiresAt, now time.Time) (agent.TokenInfo, error) {
	store.issued++
	return agent.TokenInfo{Status: "ACTIVE", NamesVisible: namesVisible, CreatedAt: &now, ExpiresAt: &expiresAt}, store.issueErr
}
func (store *agentTestStore) RevokeToken(context.Context, []byte, string, uuid.UUID, uuid.UUID, time.Time) error {
	store.revoked++
	return nil
}
func (store *agentTestStore) TokenInfo(context.Context, uuid.UUID, uuid.UUID, time.Time) (agent.TokenInfo, error) {
	return agent.TokenInfo{Status: "NONE"}, nil
}

type agentTestSnapshots struct{}

func (agentTestSnapshots) GetExactSnapshotForPeriod(context.Context, uuid.UUID, report.Key, report.Period, time.Time) (viewer.DashboardSnapshot, error) {
	return viewer.DashboardSnapshot{}, report.ErrRunNotFound
}
func (agentTestSnapshots) RevalidateSnapshot(context.Context, uuid.UUID, report.Key, report.Period, time.Time) (viewer.ReportRevalidation, error) {
	return viewer.ReportRevalidation{Disposition: viewer.RevalidationMissingRefreshing, RetryAfter: 60}, nil
}

func agentTestHandler(store *agentTestStore, enabled bool) http.Handler {
	return agentTestHandlerWith(store, enabled, &fakeAdminAuth{})
}

func agentTestHandlerWith(store *agentTestStore, enabled bool, admin *fakeAdminAuth) http.Handler {
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	service := agent.NewService(store, agentTestSnapshots{}, agentTestHasher{}, bytes.NewReader(bytes.Repeat([]byte{3}, 64)), agent.Alias(agentTestHasher{}), func() time.Time { return now }, agent.Config{})
	return NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: admin, Agent: service, AgentEnabled: enabled})
}

func newAgentStore() *agentTestStore {
	return &agentTestStore{
		valid:     "abcc_token",
		principal: agent.Principal{TokenID: uuid.New(), TenantID: uuid.New(), RecipientID: uuid.New(), ShopName: "ร้านทดสอบ", Timezone: "Asia/Bangkok"},
		permitted: []report.Key{report.SalesGoodsServices},
	}
}

func agentGet(handler http.Handler, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// The contract that lets the assistant be trusted with a refusal: a report the shop may not read and a report that
// does not exist answer with the same status and the same bytes.
func TestAgentAnswersAForbiddenReportExactlyLikeAMissingOne(t *testing.T) {
	handler := agentTestHandler(newAgentStore(), true)
	forbidden := agentGet(handler, "/api/v1/agent/reports/ar_aging", "abcc_token")
	missing := agentGet(handler, "/api/v1/agent/reports/no_such_report", "abcc_token")
	if forbidden.Code != http.StatusNotFound || forbidden.Code != missing.Code {
		t.Fatalf("status: forbidden=%d missing=%d", forbidden.Code, missing.Code)
	}
	if !bytes.Equal(forbidden.Body.Bytes(), missing.Body.Bytes()) {
		t.Fatalf("bodies differ:\n%s\n%s", forbidden.Body.String(), missing.Body.String())
	}
	if !strings.Contains(forbidden.Body.String(), "ไม่มีข้อมูลเรื่องนี้ให้ดู") {
		t.Fatalf("the refusal is plain Thai the assistant can pass on: %s", forbidden.Body.String())
	}
	for _, other := range []string{"/api/v1/agent/compare?reportKey=ar_aging&metric=x", "/api/v1/agent/deliveries/latest?reportKey=ar_aging", "/api/v1/agent/nothing"} {
		got := agentGet(handler, other, "abcc_token")
		if got.Code != http.StatusNotFound || !bytes.Equal(got.Body.Bytes(), forbidden.Body.Bytes()) {
			t.Errorf("%s: status=%d body=%s", other, got.Code, got.Body.String())
		}
	}
}

func TestAgentRefusesEveryBadTokenTheSameWay(t *testing.T) {
	handler := agentTestHandler(newAgentStore(), true)
	first := agentGet(handler, "/api/v1/agent/context", "")
	for name, token := range map[string]string{"no header": "", "wrong token": "abcc_other", "garbage": strings.Repeat("x", 400)} {
		got := agentGet(handler, "/api/v1/agent/context", token)
		if got.Code != http.StatusUnauthorized || !bytes.Equal(got.Body.Bytes(), first.Body.Bytes()) {
			t.Errorf("%s: status=%d body=%s", name, got.Code, got.Body.String())
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/agent/context", nil)
	request.Header.Set("Authorization", "Basic abc")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("a non-bearer header: %d", response.Code)
	}
}

func TestAgentContextReportAndPreparingShapes(t *testing.T) {
	handler := agentTestHandler(newAgentStore(), true)
	context := agentGet(handler, "/api/v1/agent/context", "abcc_token")
	var shop agent.ContextResponse
	if context.Code != http.StatusOK || json.Unmarshal(context.Body.Bytes(), &shop) != nil || shop.Shop != "ร้านทดสอบ" || len(shop.Reports) != 1 {
		t.Fatalf("context: %d %s", context.Code, context.Body.String())
	}
	if context.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("an answer about a shop's numbers must not be cached")
	}
	preparing := agentGet(handler, "/api/v1/agent/reports/sales_goods_services?dateFrom=2026-09-01&dateTo=2026-09-30", "abcc_token")
	var body agent.ReportResponse
	if preparing.Code != http.StatusOK || json.Unmarshal(preparing.Body.Bytes(), &body) != nil || body.Status != "PREPARING" || body.RetryAfterSeconds != 60 || body.Period == nil || body.Message == "" {
		t.Fatalf("a report that is not stored yet is PREPARING with a message and the period asked: %d %s", preparing.Code, preparing.Body.String())
	}
	invalid := agentGet(handler, "/api/v1/agent/reports/sales_goods_services?dateFrom=2026-09-30&dateTo=2026-09-01", "abcc_token")
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "INVALID_PERIOD") {
		t.Fatalf("invalid period: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestAgentRateLimitAndDisabledSwitch(t *testing.T) {
	store := newAgentStore()
	store.used = 60
	limited := agentGet(agentTestHandler(store, true), "/api/v1/agent/context", "abcc_token")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("limit: %d retry-after=%q", limited.Code, limited.Header().Get("Retry-After"))
	}
	off := agentGet(agentTestHandler(newAgentStore(), false), "/api/v1/agent/context", "abcc_token")
	if off.Code != http.StatusServiceUnavailable || !strings.Contains(off.Body.String(), "AGENT_DISABLED") {
		t.Fatalf("disabled: %d %s", off.Code, off.Body.String())
	}
}

func agentAdminRequest(method, tenantID, recipientID, body string, csrf bool) *http.Request {
	request := httptest.NewRequest(method, "/api/v1/admin/tenants/"+tenantID+"/recipients/"+recipientID+"/agent-token", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	if csrf {
		request.Header.Set("X-CSRF-Token", "admin-csrf")
	}
	return request
}

func TestAdminIssuesShowsAndRevokesAnAssistantToken(t *testing.T) {
	store := newAgentStore()
	handler := agentTestHandler(store, true)
	tenantID, recipientID := uuid.NewString(), uuid.NewString()

	issued := httptest.NewRecorder()
	handler.ServeHTTP(issued, agentAdminRequest(http.MethodPost, tenantID, recipientID, `{"namesVisible":true}`, true))
	var body agent.IssuedToken
	if issued.Code != http.StatusCreated || json.Unmarshal(issued.Body.Bytes(), &body) != nil || !strings.HasPrefix(body.Token, "abcc_") || !body.Info.NamesVisible {
		t.Fatalf("issue: %d %s", issued.Code, issued.Body.String())
	}
	if issued.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("a response that carries a token must not be cached")
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, agentAdminRequest(http.MethodGet, tenantID, recipientID, "", false))
	if status.Code != http.StatusOK || strings.Contains(status.Body.String(), "abcc_") || !strings.Contains(status.Body.String(), `"enabled":true`) {
		t.Fatalf("status must never repeat the token: %d %s", status.Code, status.Body.String())
	}

	revoked := httptest.NewRecorder()
	handler.ServeHTTP(revoked, agentAdminRequest(http.MethodDelete, tenantID, recipientID, "", true))
	if revoked.Code != http.StatusOK || store.revoked != 1 || !strings.Contains(revoked.Body.String(), `"status":"NONE"`) {
		t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body.String())
	}
}

func TestAdminTokenRoutesNeedCSRFAndExplainTheirRefusals(t *testing.T) {
	store := newAgentStore()
	handler := agentTestHandler(store, true)
	tenantID, recipientID := uuid.NewString(), uuid.NewString()

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		rejecting := agentTestHandlerWith(store, true, &fakeAdminAuth{csrfErr: auth.ErrInvalidCSRF})
		noCSRF := httptest.NewRecorder()
		rejecting.ServeHTTP(noCSRF, agentAdminRequest(method, tenantID, recipientID, `{"namesVisible":false}`, true))
		if noCSRF.Code != http.StatusForbidden || store.issued != 0 || store.revoked != 0 {
			t.Fatalf("%s with an invalid CSRF token must be refused: %d", method, noCSRF.Code)
		}
	}
	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, agentAdminRequest(http.MethodPost, tenantID, recipientID, `{}`, true))
	if bad.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing namesVisible: %d", bad.Code)
	}
	store.issueErr = agent.ErrAIChatDisabled
	off := httptest.NewRecorder()
	handler.ServeHTTP(off, agentAdminRequest(http.MethodPost, tenantID, recipientID, `{"namesVisible":false}`, true))
	if off.Code != http.StatusConflict || !strings.Contains(off.Body.String(), "AI_CHAT_DISABLED") || !strings.Contains(off.Body.String(), "คุยกับผู้ช่วย AI") {
		t.Fatalf("assistant switched off must say what to do: %d %s", off.Code, off.Body.String())
	}
	disabled := agentTestHandler(newAgentStore(), false)
	got := httptest.NewRecorder()
	disabled.ServeHTTP(got, agentAdminRequest(http.MethodPost, tenantID, recipientID, `{"namesVisible":false}`, true))
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "AGENT_DISABLED") {
		t.Fatalf("no tokens while the whole assistant is off: %d %s", got.Code, got.Body.String())
	}
}

type agentTestAlerts struct {
	items map[agent.AlertRuleKey]agent.StoredAlert
}

func (store *agentTestAlerts) Alerts(context.Context, agent.Principal) ([]agent.StoredAlert, error) {
	list := make([]agent.StoredAlert, 0)
	for _, item := range store.items {
		list = append(list, item)
	}
	return list, nil
}
func (store *agentTestAlerts) UpsertAlert(_ context.Context, _ agent.Principal, rule agent.AlertRuleKey, threshold string, enabled bool, _ string, _ time.Time) (agent.StoredAlert, error) {
	if store.items == nil {
		store.items = map[agent.AlertRuleKey]agent.StoredAlert{}
	}
	item := agent.StoredAlert{Rule: rule, Threshold: threshold, Enabled: enabled}
	store.items[rule] = item
	return item, nil
}

func agentAlertHandler(store *agentTestStore, alerts *agentTestAlerts) http.Handler {
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	service := agent.NewService(store, agentTestSnapshots{}, agentTestHasher{}, bytes.NewReader(bytes.Repeat([]byte{3}, 64)), agent.Alias(agentTestHasher{}), func() time.Time { return now }, agent.Config{}).ConfigureAlerts(alerts)
	return NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Agent: service, AgentEnabled: true})
}

func agentPut(handler http.Handler, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestAgentAlertsCanBeSetListedAndSwitchedOffOverHTTP(t *testing.T) {
	store := newAgentStore()
	store.permitted = []report.Key{report.ARAging, report.SalesGoodsServices}
	handler := agentAlertHandler(store, &agentTestAlerts{})

	set := agentPut(handler, "/api/v1/agent/alerts/ar_overdue", "abcc_token", `{"threshold":"500000"}`)
	if set.Code != http.StatusOK || !strings.Contains(set.Body.String(), `"threshold":"500000"`) || !strings.Contains(set.Body.String(), `"enabled":true`) {
		t.Fatalf("set: %d %s", set.Code, set.Body.String())
	}
	list := agentGet(handler, "/api/v1/agent/alerts", "abcc_token")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"rule":"ar_overdue"`) || !strings.Contains(list.Body.String(), `"threshold":"500000"`) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
	off := agentPut(handler, "/api/v1/agent/alerts/ar_overdue", "abcc_token", `{"enabled":false}`)
	if off.Code != http.StatusOK || !strings.Contains(off.Body.String(), `"threshold":"500000"`) || strings.Contains(off.Body.String(), `"enabled":true`) {
		t.Fatalf("off: %d %s", off.Code, off.Body.String())
	}
}

func TestAgentAlertErrorsAreThaiAndForbiddenRulesLookMissing(t *testing.T) {
	store := newAgentStore()
	store.permitted = []report.Key{report.SalesGoodsServices} // not ar_aging
	handler := agentAlertHandler(store, &agentTestAlerts{})

	bad := agentPut(handler, "/api/v1/agent/alerts/sales_drop", "abcc_token", `{"threshold":"150"}`)
	if bad.Code != http.StatusUnprocessableEntity || !strings.Contains(bad.Body.String(), "INVALID_ALERT") || !strings.Contains(bad.Body.String(), "เปอร์เซ็นต์") {
		t.Fatalf("bad threshold: %d %s", bad.Code, bad.Body.String())
	}
	malformed := agentPut(handler, "/api/v1/agent/alerts/sales_drop", "abcc_token", `{"threshold":40,"extra":1}`)
	if malformed.Code != http.StatusUnprocessableEntity || !strings.Contains(malformed.Body.String(), "INVALID_ALERT") {
		t.Fatalf("malformed: %d %s", malformed.Code, malformed.Body.String())
	}
	forbidden := agentPut(handler, "/api/v1/agent/alerts/ar_overdue", "abcc_token", `{"threshold":"1000"}`)
	missing := agentPut(handler, "/api/v1/agent/alerts/no_such_rule", "abcc_token", `{"threshold":"1000"}`)
	if forbidden.Code != http.StatusNotFound || !bytes.Equal(forbidden.Body.Bytes(), missing.Body.Bytes()) {
		t.Fatalf("a forbidden rule must answer like a missing one: %d %s / %d %s", forbidden.Code, forbidden.Body.String(), missing.Code, missing.Body.String())
	}
	if got := agentPut(handler, "/api/v1/agent/alerts/sales_drop", "abcc_other", `{"threshold":"40"}`); got.Code != http.StatusUnauthorized {
		t.Fatalf("a wrong token must not set anything: %d", got.Code)
	}
}

func TestAgentAlertsAnswerLikeNothingWhenNotConfigured(t *testing.T) {
	handler := agentTestHandler(newAgentStore(), true) // service built without alerts
	for _, got := range []*httptest.ResponseRecorder{agentGet(handler, "/api/v1/agent/alerts", "abcc_token"), agentPut(handler, "/api/v1/agent/alerts/sales_drop", "abcc_token", `{"threshold":"40"}`)} {
		if got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), "NO_DATA") {
			t.Fatalf("%d %s", got.Code, got.Body.String())
		}
	}
}
