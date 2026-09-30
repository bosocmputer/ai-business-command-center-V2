package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/monitor"
)

type fakeMonitorAPI struct {
	sample     *monitor.Sample
	points     []monitor.HistoryPoint
	historyErr error
	minutes    int
}

func (f *fakeMonitorAPI) Current() (monitor.Sample, bool) {
	if f.sample == nil {
		return monitor.Sample{}, false
	}
	return *f.sample, true
}

func (f *fakeMonitorAPI) History(_ context.Context, minutes int) ([]monitor.HistoryPoint, error) {
	f.minutes = minutes
	return f.points, f.historyErr
}

func monitorRequest(handler http.Handler, path string, authenticated bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if authenticated {
		request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestAdminMonitorCurrentAndHistory(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	api := &fakeMonitorAPI{
		sample: &monitor.Sample{At: now, Host: monitor.HostStats{Cores: 4, CPUPercent: 12.5}, ContainersAvailable: true, Containers: []monitor.ContainerStats{{Name: "api", CPUPercent: 1}}},
		points: []monitor.HistoryPoint{{At: now, CPUPercent: 10, Containers: []monitor.ContainerStats{}}},
	}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Monitor: api})

	response := monitorRequest(handler, "/api/v1/admin/monitor/current", true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cpuPercent":12.5`) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("current: status=%d cache=%q body=%s", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
	}

	response = monitorRequest(handler, "/api/v1/admin/monitor/history", true)
	if response.Code != http.StatusOK || api.minutes != monitor.DefaultHistoryMinutes || !strings.Contains(response.Body.String(), `"minutes":60`) {
		t.Fatalf("default history: status=%d minutes=%d body=%s", response.Code, api.minutes, response.Body.String())
	}
	if response = monitorRequest(handler, "/api/v1/admin/monitor/history?minutes=360", true); response.Code != http.StatusOK || api.minutes != 360 {
		t.Fatalf("history 360: status=%d minutes=%d", response.Code, api.minutes)
	}
}

func TestAdminMonitorRejectsBadInputAndReportsFailures(t *testing.T) {
	api := &fakeMonitorAPI{}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: &fakeAdminAuth{}, Monitor: api})

	if response := monitorRequest(handler, "/api/v1/admin/monitor/current", true); response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "MONITOR_WARMING_UP") {
		t.Fatalf("no sample yet: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := monitorRequest(handler, "/api/v1/admin/monitor/history?minutes=abc", true); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-numeric minutes status=%d", response.Code)
	}
	api.historyErr = monitor.ErrInvalidRange
	if response := monitorRequest(handler, "/api/v1/admin/monitor/history?minutes=99999", true); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("out of range status=%d", response.Code)
	}
	api.historyErr = errors.New("db down secret-detail")
	response := monitorRequest(handler, "/api/v1/admin/monitor/history", true)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret-detail") {
		t.Fatalf("store failure: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAdminMonitorRequiresAdminSession(t *testing.T) {
	api := &fakeMonitorAPI{sample: &monitor.Sample{}}
	for name, adminAuth := range map[string]*fakeAdminAuth{
		"no cookie":         {},
		"invalid session":   {authErr: auth.ErrInvalidSession},
		"password rotation": {admin: auth.AuthenticatedAdmin{MustRotatePassword: true}},
	} {
		handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: adminAuth, Monitor: api})
		response := monitorRequest(handler, "/api/v1/admin/monitor/current", name != "no cookie")
		if response.Code != http.StatusUnauthorized && response.Code != http.StatusForbidden {
			t.Fatalf("%s: status=%d, want 401/403", name, response.Code)
		}
	}
}
