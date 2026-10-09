package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/google/uuid"
)

type fakeAssistantSettings struct {
	setCalls    []string
	clearCalls  []string
	globalCalls []string
	err         error
}

func (fake *fakeAssistantSettings) View(context.Context, uuid.UUID) (assistantcfg.AdminView, error) {
	return assistantcfg.AdminView{ModelKey: "gemini-3.1-flash-lite", LineMode: "NONE", Secrets: map[string]assistantcfg.SecretState{"openrouterKey": {IsSet: true, Last4: "wxyz"}}}, fake.err
}

func (fake *fakeAssistantSettings) Update(_ context.Context, _ []byte, _ string, _ uuid.UUID, input assistantcfg.UpdateInput) (assistantcfg.AdminView, error) {
	return assistantcfg.AdminView{Enabled: input.Enabled != nil && *input.Enabled}, fake.err
}

func (fake *fakeAssistantSettings) SetSecret(_ context.Context, _ []byte, _ string, _ uuid.UUID, field assistantcfg.Field, value string) (assistantcfg.AdminView, error) {
	fake.setCalls = append(fake.setCalls, string(field)+"="+value)
	return assistantcfg.AdminView{}, fake.err
}

func (fake *fakeAssistantSettings) ClearSecret(_ context.Context, _ []byte, _ string, _ uuid.UUID, field assistantcfg.Field) (assistantcfg.AdminView, error) {
	fake.clearCalls = append(fake.clearCalls, string(field))
	return assistantcfg.AdminView{}, fake.err
}

func (fake *fakeAssistantSettings) GlobalView(context.Context) (assistantcfg.GlobalView, error) {
	return assistantcfg.GlobalView{}, fake.err
}

func (fake *fakeAssistantSettings) SetGlobalSecret(_ context.Context, _ []byte, _ string, field assistantcfg.Field, value string) (assistantcfg.GlobalView, error) {
	fake.globalCalls = append(fake.globalCalls, string(field)+"="+value)
	return assistantcfg.GlobalView{}, fake.err
}

func assistantHandler(api *fakeAssistantSettings, adminAuth *fakeAdminAuth) http.Handler {
	return NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: adminAuth, AssistantSettings: api})
}

func assistantRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	request.Header.Set("X-CSRF-Token", "token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

const (
	assistantPath = "/api/v1/admin/tenants/11111111-1111-1111-1111-111111111111/assistant"
	goodKey       = "sk-or-v1-0123456789abcdefghijklmnopqrstuvwxyz"
)

func TestAssistantSettingsAreShownWithoutAnySecret(t *testing.T) {
	handler := assistantHandler(&fakeAssistantSettings{}, &fakeAdminAuth{})
	response := assistantRequest(handler, http.MethodGet, assistantPath, "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"isSet":true`) || !strings.Contains(response.Body.String(), `"last4":"wxyz"`) {
		t.Fatalf("view = %d %s", response.Code, response.Body.String())
	}
}

func TestASecretIsSetOnlyAfterTheFormatAndThePasswordCheckOut(t *testing.T) {
	api, adminAuth := &fakeAssistantSettings{}, &fakeAdminAuth{}
	handler := assistantHandler(api, adminAuth)
	path := assistantPath + "/secrets/openrouter-key"

	bad := assistantRequest(handler, http.MethodPut, path, `{"value":"not-a-key","adminPassword":"correct password"}`)
	if bad.Code != http.StatusUnprocessableEntity || strings.Contains(bad.Body.String(), "not-a-key") || len(adminAuth.confirmed) != 0 || len(api.setCalls) != 0 {
		t.Fatalf("a malformed key must be refused before the password is used and must not be echoed: %d %s", bad.Code, bad.Body.String())
	}

	adminAuth.confirmErr = auth.ErrInvalidCredentials
	wrong := assistantRequest(handler, http.MethodPut, path, `{"value":"`+goodKey+`","adminPassword":"wrong"}`)
	if wrong.Code != http.StatusForbidden || len(api.setCalls) != 0 || strings.Contains(wrong.Body.String(), goodKey) {
		t.Fatalf("a wrong password must store nothing: %d %s", wrong.Code, wrong.Body.String())
	}
	adminAuth.confirmErr = auth.ErrLoginLocked
	if locked := assistantRequest(handler, http.MethodPut, path, `{"value":"`+goodKey+`","adminPassword":"x"}`); locked.Code != http.StatusTooManyRequests || len(api.setCalls) != 0 {
		t.Fatalf("locked = %d", locked.Code)
	}

	adminAuth.confirmErr = nil
	ok := assistantRequest(handler, http.MethodPut, path, `{"value":"`+goodKey+`","adminPassword":"correct password"}`)
	if ok.Code != http.StatusOK || len(api.setCalls) != 1 || api.setCalls[0] != "openrouter-key="+goodKey {
		t.Fatalf("set = %d %v", ok.Code, api.setCalls)
	}
	if strings.Contains(ok.Body.String(), goodKey) {
		t.Fatal("the answer must not carry the secret back")
	}
	if adminAuth.confirmed[len(adminAuth.confirmed)-1] != "correct password" {
		t.Errorf("the password given must be the one checked: %v", adminAuth.confirmed)
	}
}

func TestClearingASecretNeedsThePasswordToo(t *testing.T) {
	api, adminAuth := &fakeAssistantSettings{}, &fakeAdminAuth{confirmErr: auth.ErrInvalidCredentials}
	handler := assistantHandler(api, adminAuth)
	if response := assistantRequest(handler, http.MethodPost, assistantPath+"/secrets/telegram-bot-token/clear", `{"adminPassword":"nope"}`); response.Code != http.StatusForbidden || len(api.clearCalls) != 0 {
		t.Fatalf("clear without the right password = %d %v", response.Code, api.clearCalls)
	}
	adminAuth.confirmErr = nil
	if response := assistantRequest(handler, http.MethodPost, assistantPath+"/secrets/telegram-bot-token/clear", `{"adminPassword":"yes"}`); response.Code != http.StatusOK || len(api.clearCalls) != 1 {
		t.Fatalf("clear = %d %v", response.Code, api.clearCalls)
	}
}

func TestOnlyTheLineChannelIsSharedAndUnknownFieldsAreNotFound(t *testing.T) {
	api := &fakeAssistantSettings{}
	handler := assistantHandler(api, &fakeAdminAuth{})
	if response := assistantRequest(handler, http.MethodPut, "/api/v1/admin/assistant/global/secrets/openrouter-key", `{"value":"`+goodKey+`","adminPassword":"p"}`); response.Code != http.StatusUnprocessableEntity || len(api.globalCalls) != 0 {
		t.Fatalf("an OpenRouter key is never shared: %d", response.Code)
	}
	if response := assistantRequest(handler, http.MethodPut, "/api/v1/admin/assistant/global/secrets/line-channel-secret", `{"value":"0123456789abcdef0123456789abcdef","adminPassword":"p"}`); response.Code != http.StatusOK || len(api.globalCalls) != 1 {
		t.Fatalf("shared LINE secret = %d", response.Code)
	}
	if response := assistantRequest(handler, http.MethodPut, assistantPath+"/secrets/root-password", `{"value":"x","adminPassword":"p"}`); response.Code != http.StatusNotFound {
		t.Fatalf("unknown field = %d", response.Code)
	}
}

func TestAssistantSettingsNeedAnAdminAndACSRFTokenForChanges(t *testing.T) {
	api := &fakeAssistantSettings{}
	if response := assistantRequest(assistantHandler(api, &fakeAdminAuth{authErr: auth.ErrInvalidSession}), http.MethodGet, assistantPath, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("no session = %d", response.Code)
	}
	if response := assistantRequest(assistantHandler(api, &fakeAdminAuth{csrfErr: auth.ErrInvalidCSRF}), http.MethodPut, assistantPath, `{"enabled":true,"version":1}`); response.Code != http.StatusForbidden {
		t.Fatalf("bad CSRF = %d", response.Code)
	}
}

func TestAssistantSettingsErrorsAreMappedWithoutDetail(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
	}{
		"conflict":  {assistantcfg.ErrVersionConflict, http.StatusConflict},
		"not found": {assistantcfg.ErrTenantNotFound, http.StatusNotFound},
		"invalid":   {&assistantcfg.ValidationError{Field: "enabled", Code: "OPENROUTER_KEY_REQUIRED"}, http.StatusUnprocessableEntity},
		"other":     {context.DeadlineExceeded, http.StatusInternalServerError},
	}
	for name, tc := range cases {
		response := assistantRequest(assistantHandler(&fakeAssistantSettings{err: tc.err}, &fakeAdminAuth{}), http.MethodPut, assistantPath, `{"enabled":true,"version":1}`)
		if response.Code != tc.status {
			t.Errorf("%s: %d, want %d", name, response.Code, tc.status)
		}
		var body map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		if name == "other" && strings.Contains(response.Body.String(), "deadline") {
			t.Errorf("an internal error must not leak its text: %s", response.Body.String())
		}
	}
}
