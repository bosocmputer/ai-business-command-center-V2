package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/google/uuid"
)

type engagementAPIFake struct {
	settings engagement.Settings
	input    engagement.Settings
	err      error
}

func (fake *engagementAPIFake) Get(context.Context, uuid.UUID) (engagement.Settings, error) {
	return fake.settings, fake.err
}

func (fake *engagementAPIFake) Put(_ context.Context, _ []byte, _ string, _ uuid.UUID, input engagement.Settings) (engagement.Settings, error) {
	fake.input = input
	return fake.settings, fake.err
}

func engagementRequest(method, tenantID, body string) *http.Request {
	request := httptest.NewRequest(method, "/api/v1/admin/tenants/"+tenantID+"/engagement", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", "csrf-token")
	return request
}

func engagementHandler(api EngagementAPI) http.Handler {
	return NewHandler(Dependencies{
		Readiness:  readinessFunc(func(context.Context) error { return nil }),
		AdminAuth:  &fakeAdminAuth{admin: auth.AuthenticatedAdmin{TokenHash: []byte("actor")}},
		Engagement: api,
	})
}

func TestAdminReadsAndSavesEngagementSettings(t *testing.T) {
	tenantID := uuid.New().String()
	saved := engagement.Defaults()
	saved.AlertChannel, saved.Version = engagement.ChannelLine, 3
	api := &engagementAPIFake{settings: saved}
	handler := engagementHandler(api)

	read := httptest.NewRecorder()
	handler.ServeHTTP(read, engagementRequest(http.MethodGet, tenantID, ""))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"alertChannel":"LINE"`) {
		t.Fatalf("status=%d body=%s", read.Code, read.Body.String())
	}

	write := httptest.NewRecorder()
	handler.ServeHTTP(write, engagementRequest(http.MethodPut, tenantID,
		`{"alertChannel":"LINE","digestEnabled":false,"weeklySummaryEnabled":true,"weeklySummaryWeekday":2,"weeklySummaryHour":9,"inactivityNoticeEnabled":true,"inactivityQuietDays":7,"version":2}`))
	if write.Code != http.StatusOK || api.input.Version != 2 || api.input.DigestEnabled || !api.input.WeeklySummaryEnabled || api.input.InactivityQuietDays != 7 {
		t.Fatalf("status=%d input=%+v body=%s", write.Code, api.input, write.Body.String())
	}
}

func TestEngagementMapsConflictInvalidAndMissingShop(t *testing.T) {
	tenantID := uuid.New().String()
	body := `{"alertChannel":"LINE","digestEnabled":true,"weeklySummaryEnabled":false,"weeklySummaryWeekday":1,"weeklySummaryHour":8,"inactivityNoticeEnabled":true,"inactivityQuietDays":5,"version":1}`
	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"conflict": {engagement.ErrConflict, http.StatusConflict, "VERSION_CONFLICT"},
		"invalid":  {engagement.ErrInvalid, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
		"missing":  {engagement.ErrNotFound, http.StatusNotFound, "TENANT_NOT_FOUND"},
	} {
		response := httptest.NewRecorder()
		engagementHandler(&engagementAPIFake{err: tc.err}).ServeHTTP(response, engagementRequest(http.MethodPut, tenantID, body))
		if response.Code != tc.status || !strings.Contains(response.Body.String(), `"code":"`+tc.code+`"`) {
			t.Fatalf("%s: status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
}

func TestEngagementRefusesUnknownFieldsAndBadJSON(t *testing.T) {
	response := httptest.NewRecorder()
	engagementHandler(&engagementAPIFake{}).ServeHTTP(response, engagementRequest(http.MethodPut, uuid.New().String(), `{"alertChannel":"LINE","surprise":1}`))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
