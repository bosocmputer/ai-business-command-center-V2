package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/provision"
	"github.com/google/uuid"
)

type fakeInstances struct {
	created []string
	stopped int
	err     error
}

func (fake *fakeInstances) Overview(context.Context, uuid.UUID) (provision.Overview, error) {
	return provision.Overview{ManualHost: "assistant"}, fake.err
}
func (fake *fakeInstances) Create(_ context.Context, _ []byte, _ string, _, recipientID uuid.UUID, namesVisible bool) (provision.Instance, error) {
	if fake.err != nil {
		return provision.Instance{}, fake.err
	}
	fake.created = append(fake.created, recipientID.String())
	return provision.Instance{Name: "s9d36e282", Host: "assistant-s9d36e282", Status: provision.StatusPending}, nil
}
func (fake *fakeInstances) Stop(context.Context, []byte, string, uuid.UUID) (provision.Instance, error) {
	if fake.err != nil {
		return provision.Instance{}, fake.err
	}
	fake.stopped++
	return provision.Instance{Status: provision.StatusStopping}, nil
}

func instanceHandler(api *fakeInstances, adminAuth *fakeAdminAuth) http.Handler {
	return NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: adminAuth, AssistantInstances: api})
}

const (
	instancePath = assistantPath + "/instance"
	someone      = "22222222-2222-2222-2222-222222222222"
)

func TestCreatingAnInstanceNeedsThePasswordAndNeverReturnsAToken(t *testing.T) {
	api, adminAuth := &fakeInstances{}, &fakeAdminAuth{confirmErr: auth.ErrInvalidCredentials}
	handler := instanceHandler(api, adminAuth)
	body := `{"recipientId":"` + someone + `","namesVisible":true,"adminPassword":"nope"}`
	if response := assistantRequest(handler, http.MethodPost, instancePath, body); response.Code != http.StatusForbidden || len(api.created) != 0 {
		t.Fatalf("a wrong password creates nothing: %d %v", response.Code, api.created)
	}
	adminAuth.confirmErr = nil
	response := assistantRequest(handler, http.MethodPost, instancePath, body)
	if response.Code != http.StatusAccepted || len(api.created) != 1 || api.created[0] != someone {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(strings.ToLower(response.Body.String()), "abcc_") || !strings.Contains(response.Body.String(), `"status":"PENDING"`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("the answer says what happens and carries no token: %s", response.Body.String())
	}
}

func TestInstanceInputIsCheckedBeforeThePasswordIsUsed(t *testing.T) {
	api, adminAuth := &fakeInstances{}, &fakeAdminAuth{}
	handler := instanceHandler(api, adminAuth)
	for _, body := range []string{`{}`, `{"recipientId":"not-a-uuid","namesVisible":true,"adminPassword":"p"}`, `{"recipientId":"` + someone + `","adminPassword":"p"}`, `{"recipientId":"` + someone + `","namesVisible":true,"adminPassword":"p","extra":1}`} {
		if response := assistantRequest(handler, http.MethodPost, instancePath, body); response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %d", body, response.Code)
		}
	}
	if len(adminAuth.confirmed) != 0 || len(api.created) != 0 {
		t.Fatal("a malformed request must not use an attempt of the password")
	}
}

func TestInstanceProblemsAreAnsweredInPlainWords(t *testing.T) {
	cases := []struct {
		err  error
		code int
		word string
	}{
		{provision.ErrAlreadyExists, http.StatusConflict, "ALREADY_EXISTS"},
		{provision.ErrHasManualAssistant, http.StatusConflict, "HAS_MANUAL_ASSISTANT"},
		{agent.ErrAIChatDisabled, http.StatusConflict, "AI_CHAT_DISABLED"},
		{agent.ErrRecipientNotFound, http.StatusNotFound, "NOT_FOUND"},
		{errors.New("database down: secret=abc"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, test := range cases {
		api := &fakeInstances{err: test.err}
		response := assistantRequest(instanceHandler(api, &fakeAdminAuth{}), http.MethodPost, instancePath, `{"recipientId":"`+someone+`","namesVisible":false,"adminPassword":"p"}`)
		if response.Code != test.code || !strings.Contains(response.Body.String(), test.word) || strings.Contains(response.Body.String(), "secret=abc") {
			t.Fatalf("%v = %d %s", test.err, response.Code, response.Body.String())
		}
	}
}

func TestStoppingAnInstanceNeedsThePasswordAndTheOverviewNeedsOnlyASession(t *testing.T) {
	api, adminAuth := &fakeInstances{}, &fakeAdminAuth{confirmErr: auth.ErrInvalidCredentials}
	handler := instanceHandler(api, adminAuth)
	if response := assistantRequest(handler, http.MethodPost, instancePath+"/stop", `{"adminPassword":"nope"}`); response.Code != http.StatusForbidden || api.stopped != 0 {
		t.Fatalf("stop with a wrong password = %d", response.Code)
	}
	adminAuth.confirmErr = nil
	if response := assistantRequest(handler, http.MethodPost, instancePath+"/stop", `{"adminPassword":"yes"}`); response.Code != http.StatusAccepted || api.stopped != 1 {
		t.Fatalf("stop = %d", response.Code)
	}
	if response := assistantRequest(handler, http.MethodGet, instancePath, ""); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"manualHost":"assistant"`) {
		t.Fatalf("overview = %d %s", response.Code, response.Body.String())
	}
	if response := assistantRequest(instanceHandler(api, &fakeAdminAuth{authErr: auth.ErrInvalidSession}), http.MethodGet, instancePath, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("no session = %d", response.Code)
	}
}
