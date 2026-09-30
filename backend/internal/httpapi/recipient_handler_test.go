package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

type fakeRecipientAPI struct {
	item         recipient.Recipient
	revokeErr    error
	revokeCalls  int
	reissueCalls int
	queryInput   recipient.QueryInput
	aiChatCalls  int
	aiChatValue  bool
	aiChatErr    error
}

func (fake *fakeRecipientAPI) CreateInvitation(context.Context, []byte, string, string, uuid.UUID, string) (recipient.Recipient, error) {
	return fake.item, nil
}

func (fake *fakeRecipientAPI) ReissueInvitation(context.Context, []byte, string, string, uuid.UUID, uuid.UUID) (recipient.Recipient, error) {
	fake.reissueCalls++
	return fake.item, nil
}

func (fake *fakeRecipientAPI) List(context.Context, uuid.UUID, int, string) (recipient.RecipientPage, error) {
	return recipient.RecipientPage{Data: []recipient.Recipient{fake.item}}, nil
}

func (fake *fakeRecipientAPI) GetForTenant(context.Context, uuid.UUID, uuid.UUID) (recipient.Recipient, error) {
	return fake.item, nil
}

func (fake *fakeRecipientAPI) PermissionDependencies(context.Context, uuid.UUID, uuid.UUID) (recipient.PermissionDependencies, error) {
	return recipient.PermissionDependencies{RecipientID: fake.item.ID, PermissionsVersion: fake.item.PermissionsVersion, Items: []recipient.PermissionDependency{}}, nil
}

func (fake *fakeRecipientAPI) ScheduleRecipientOptions(context.Context, uuid.UUID, recipient.ScheduleRecipientOptionsInput) (recipient.ScheduleRecipientOptions, error) {
	return recipient.ScheduleRecipientOptions{Data: []recipient.ScheduleRecipientOption{}}, nil
}

func (fake *fakeRecipientAPI) Query(_ context.Context, _ uuid.UUID, input recipient.QueryInput) (recipient.QueryResult, error) {
	fake.queryInput = input
	return recipient.QueryResult{Data: []recipient.Recipient{fake.item}, Page: input.Page, PageSize: input.PageSize, Total: 1}, nil
}

func TestAdminQueriesRecipientsWithExactPaginationAndFilters(t *testing.T) {
	tenantID := uuid.New()
	api := &fakeRecipientAPI{item: recipient.Recipient{ID: uuid.New(), DisplayName: "ผู้บริหาร"}}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Recipients: api})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/"+tenantID.String()+"/recipients/query", strings.NewReader(`{"search":"ผู้","status":"ACTIVE","permissionState":"WITH_REPORTS","page":2,"pageSize":25}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "admin-csrf")
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || api.queryInput.Page != 2 || api.queryInput.PageSize != 25 || api.queryInput.Status != recipient.StatusActive || api.queryInput.PermissionState != "WITH_REPORTS" || !strings.Contains(response.Body.String(), `"total":1`) {
		t.Fatalf("status=%d input=%+v body=%s", response.Code, api.queryInput, response.Body.String())
	}
}

func (fake *fakeRecipientAPI) ReplacePermissions(context.Context, []byte, string, uuid.UUID, uuid.UUID, []report.Key, int) (recipient.Recipient, error) {
	return fake.item, nil
}

func (fake *fakeRecipientAPI) SetAIChat(_ context.Context, _ []byte, _ string, _ uuid.UUID, _ uuid.UUID, enabled bool) (recipient.Recipient, error) {
	fake.aiChatCalls++
	fake.aiChatValue = enabled
	if fake.aiChatErr != nil {
		return recipient.Recipient{}, fake.aiChatErr
	}
	updated := fake.item
	updated.AIChatEnabled = enabled
	return updated, nil
}

func (fake *fakeRecipientAPI) Revoke(context.Context, []byte, string, uuid.UUID, uuid.UUID) error {
	fake.revokeCalls++
	return fake.revokeErr
}

func TestAdminRevokesTenantRecipientWithCSRFGuard(t *testing.T) {
	tenantID, recipientID := uuid.New(), uuid.New()
	api := &fakeRecipientAPI{}
	handler := NewHandler(Dependencies{
		Readiness:  readinessFunc(func(context.Context) error { return nil }),
		AdminAuth:  &fakeAdminAuth{},
		Recipients: api,
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tenants/"+tenantID.String()+"/recipients/"+recipientID.String(), nil)
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	request.Header.Set("X-CSRF-Token", "admin-csrf")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || api.revokeCalls != 1 || response.Body.Len() != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, api.revokeCalls, response.Body.String())
	}
}

func TestAdminReissuesPendingRecipientInvitationWithMutationGuards(t *testing.T) {
	tenantID, recipientID := uuid.New(), uuid.New()
	api := &fakeRecipientAPI{item: recipient.Recipient{ID: recipientID, Status: recipient.StatusPending, InvitationURL: "https://dashboard.nextstep-soft.com/app/invite?ref=new"}}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Recipients: api})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tenants/"+tenantID.String()+"/recipients/"+recipientID.String()+"/invitation", nil)
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	request.Header.Set("X-CSRF-Token", "admin-csrf")
	request.Header.Set("Idempotency-Key", "recipient-reissue-test")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || api.reissueCalls != 1 || !strings.Contains(response.Body.String(), `"invitationUrl":"https://dashboard.nextstep-soft.com/app/invite?ref=new"`) {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, api.reissueCalls, response.Body.String())
	}
}

func TestAdminRecipientRevokeReturnsActiveScheduleDependencies(t *testing.T) {
	tenantID, recipientID := uuid.New(), uuid.New()
	api := &fakeRecipientAPI{revokeErr: &recipient.RecipientInUseError{ScheduleNames: []string{"รายงานเช้า"}}}
	handler := NewHandler(Dependencies{
		Readiness:  readinessFunc(func(context.Context) error { return nil }),
		AdminAuth:  &fakeAdminAuth{},
		Recipients: api,
	})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/tenants/"+tenantID.String()+"/recipients/"+recipientID.String(), nil)
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	request.Header.Set("X-CSRF-Token", "admin-csrf")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"RECIPIENT_IN_USE"`) || !strings.Contains(response.Body.String(), "รายงานเช้า") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func aiChatRequest(tenantID, recipientID uuid.UUID, body string, csrf bool) *http.Request {
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/tenants/"+tenantID.String()+"/recipients/"+recipientID.String()+"/ai-chat", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	if csrf {
		request.Header.Set("X-CSRF-Token", "admin-csrf")
	}
	return request
}

func TestAdminSetsRecipientAIChatWithMutationGuards(t *testing.T) {
	tenantID, recipientID := uuid.New(), uuid.New()
	api := &fakeRecipientAPI{item: recipient.Recipient{ID: recipientID, DisplayName: "เจ้าของร้าน"}}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Recipients: api})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, aiChatRequest(tenantID, recipientID, `{"enabled":true}`, true))
	if response.Code != http.StatusOK || api.aiChatCalls != 1 || !api.aiChatValue || !strings.Contains(response.Body.String(), `"aiChatEnabled":true`) {
		t.Fatalf("enable: status=%d calls=%d body=%s", response.Code, api.aiChatCalls, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, aiChatRequest(tenantID, recipientID, `{"enabled":false}`, true))
	if response.Code != http.StatusOK || api.aiChatValue || !strings.Contains(response.Body.String(), `"aiChatEnabled":false`) {
		t.Fatalf("disable: status=%d value=%v body=%s", response.Code, api.aiChatValue, response.Body.String())
	}

	before := api.aiChatCalls
	for name, request := range map[string]*http.Request{
		"missing enabled":     aiChatRequest(tenantID, recipientID, `{}`, true),
		"non-boolean enabled": aiChatRequest(tenantID, recipientID, `{"enabled":"yes"}`, true),
		"unknown field":       aiChatRequest(tenantID, recipientID, `{"enabled":true,"role":"admin"}`, true),
	} {
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code == http.StatusOK || api.aiChatCalls != before {
			t.Fatalf("%s: status=%d, service called=%v", name, response.Code, api.aiChatCalls != before)
		}
	}
}

func TestAdminAIChatRejectsInvalidCSRF(t *testing.T) {
	tenantID, recipientID := uuid.New(), uuid.New()
	api := &fakeRecipientAPI{}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{csrfErr: auth.ErrInvalidCSRF}, Recipients: api})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, aiChatRequest(tenantID, recipientID, `{"enabled":true}`, true))
	if response.Code != http.StatusForbidden || api.aiChatCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, api.aiChatCalls, response.Body.String())
	}
}

func TestAdminAIChatReportsUnknownRecipient(t *testing.T) {
	tenantID, recipientID := uuid.New(), uuid.New()
	api := &fakeRecipientAPI{aiChatErr: recipient.ErrRecipientNotFound}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Recipients: api})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, aiChatRequest(tenantID, recipientID, `{"enabled":true}`, true))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "RECIPIENT_NOT_FOUND") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
