package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewevent"
	"github.com/google/uuid"
)

type recordedEvents struct {
	mu     sync.Mutex
	events []viewevent.Event
}

func (r *recordedEvents) Record(event viewevent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recordedEvents) all() []viewevent.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]viewevent.Event(nil), r.events...)
}

func viewerCall(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: viewerSessionCookie, Value: "viewer-session"})
	if method != http.MethodGet {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-CSRF-Token", "viewer-csrf")
		request.Header.Set("Idempotency-Key", "viewer-event-001")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestViewerOpensAreRecordedWithoutPersonalDataOrValues(t *testing.T) {
	recipientID, tenantID, deliveryID := uuid.New(), uuid.New(), uuid.New()
	events := &recordedEvents{}
	authAPI := &fakeViewerAPI{
		authenticated:   viewer.AuthenticatedViewer{RecipientID: recipientID, DisplayName: "ชื่อที่ต้องไม่ถูกบันทึก"},
		deliveryContext: viewer.DeliveryContext{DeliveryID: deliveryID, TenantID: tenantID},
		deliveryReport:  viewer.DeliveryReportContext{DeliveryID: deliveryID, TenantID: tenantID},
	}
	reportAPI := &fakeViewerReportAPI{created: report.Run{ID: uuid.New(), TenantID: tenantID, ReportKey: report.StockBalance, Status: report.StatusQueued}}
	handler := NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), ViewerAuth: authAPI, ViewerReports: reportAPI, ViewEvents: events})
	base := "/api/v1/viewer/tenants/" + tenantID.String()

	calls := []struct {
		name         string
		method, path string
		body         string
		want         viewevent.Event
	}{
		{"card button", http.MethodPost, "/api/v1/viewer/delivery-contexts",
			`{"deliveryReference":"` + strings.Repeat("a", 40) + `","expectedTenantId":"` + tenantID.String() + `"}`,
			viewevent.Event{Kind: viewevent.CardOpen, DeliveryID: &deliveryID}},
		{"report from the card", http.MethodGet, base + "/deliveries/" + deliveryID.String() + "/reports/sales_goods_services", "",
			viewevent.Event{Kind: viewevent.ReportView, ReportKey: "sales_goods_services", DeliveryID: &deliveryID}},
		{"report page", http.MethodGet, base + "/reports/stock_balance/snapshots/latest?periodPreset=YESTERDAY", "",
			viewevent.Event{Kind: viewevent.ReportView, ReportKey: "stock_balance"}},
		{"overview", http.MethodGet, base + "/executive-overview", "",
			viewevent.Event{Kind: viewevent.OverviewView}},
		{"fresh report", http.MethodPost, base + "/reports/stock_balance/runs", `{"periodPreset":"YESTERDAY"}`,
			viewevent.Event{Kind: viewevent.RefreshRequest, ReportKey: "stock_balance"}},
	}
	for _, call := range calls {
		if response := viewerCall(handler, call.method, call.path, call.body); response.Code >= 300 {
			t.Fatalf("%s: status=%d body=%s", call.name, response.Code, response.Body.String())
		}
	}

	got := events.all()
	if len(got) != len(calls) {
		t.Fatalf("recorded %d events, want %d: %+v", len(got), len(calls), got)
	}
	for index, call := range calls {
		event := got[index]
		if event.Kind != call.want.Kind || event.ReportKey != call.want.ReportKey || event.TenantID != tenantID || event.RecipientID != recipientID || event.At.IsZero() {
			t.Fatalf("%s: event = %+v, want kind=%s report=%q", call.name, event, call.want.Kind, call.want.ReportKey)
		}
		if (call.want.DeliveryID == nil) != (event.DeliveryID == nil) || (event.DeliveryID != nil && *event.DeliveryID != deliveryID) {
			t.Fatalf("%s: delivery id = %v", call.name, event.DeliveryID)
		}
	}
}

func TestFailedOrUnauthenticatedOpensAreNotRecorded(t *testing.T) {
	tenantID := uuid.New()
	events := &recordedEvents{}
	base := "/api/v1/viewer/tenants/" + tenantID.String()

	denied := NewHandler(Dependencies{
		Readiness:     readinessFunc(func(context.Context) error { return nil }),
		ViewerAuth:    &fakeViewerAPI{authenticated: viewer.AuthenticatedViewer{RecipientID: uuid.New()}},
		ViewerReports: &fakeViewerReportAPI{getErr: viewer.ErrReportForbidden}, ViewEvents: events,
	})
	if response := viewerCall(denied, http.MethodGet, base+"/reports/stock_balance/snapshots/latest?periodPreset=YESTERDAY", ""); response.Code < 400 {
		t.Fatalf("a forbidden report must fail, status=%d", response.Code)
	}

	unauthenticated := NewHandler(Dependencies{
		Readiness:  readinessFunc(func(context.Context) error { return nil }),
		ViewerAuth: &fakeViewerAPI{authErr: viewer.ErrSessionInvalid}, ViewerReports: &fakeViewerReportAPI{}, ViewEvents: events,
	})
	if response := viewerCall(unauthenticated, http.MethodGet, base+"/executive-overview", ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", response.Code)
	}
	if got := events.all(); len(got) != 0 {
		t.Fatalf("recorded %+v for requests that were refused", got)
	}
}
