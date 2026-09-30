package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/auth"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/executionmode"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

type fakeModesAPI struct {
	items      []executionmode.Item
	setErr     error
	measureErr error
	results    []executionmode.Measurement
	setKey     report.Key
	setMode    report.ExecutionMode
	setReason  string
}

func (f *fakeModesAPI) List(context.Context, uuid.UUID) ([]executionmode.Item, error) {
	return f.items, nil
}
func (f *fakeModesAPI) Set(_ context.Context, _ []byte, _ string, _ uuid.UUID, key report.Key, mode report.ExecutionMode, reason string) (executionmode.Item, error) {
	f.setKey, f.setMode, f.setReason = key, mode, reason
	if f.setErr != nil {
		return executionmode.Item{}, f.setErr
	}
	return executionmode.Item{ReportKey: key, Mode: mode, Source: report.ModeSourceManual, Chunkable: true}, nil
}
func (f *fakeModesAPI) Measure(context.Context, []byte, string, uuid.UUID) ([]executionmode.Measurement, error) {
	return f.results, f.measureErr
}

func modesRequest(handler http.Handler, method, path, body string, csrf bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: adminSessionCookie, Value: "admin-session"})
	if csrf {
		request.Header.Set("X-CSRF-Token", "token")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func newModesHandler(api *fakeModesAPI, adminAuth *fakeAdminAuth) http.Handler {
	return NewHandler(Dependencies{Readiness: readinessFunc(func(context.Context) error { return nil }), AdminAuth: adminAuth, ExecutionModes: api})
}

func TestReportModesListSetAndMeasure(t *testing.T) {
	tenant := uuid.New().String()
	units := 4200
	api := &fakeModesAPI{
		items:   []executionmode.Item{{ReportKey: report.StockBalance, Label: "สต็อกคงเหลือ", Chunkable: true, Mode: report.ModeChunked, Source: report.ModeSourceAutoSwitched}},
		results: []executionmode.Measurement{{ReportKey: report.StockBalance, Units: &units, RecommendedMode: report.ModeChunked, Applied: true}},
	}
	handler := newModesHandler(api, &fakeAdminAuth{})

	list := modesRequest(handler, http.MethodGet, "/api/v1/admin/tenants/"+tenant+"/report-modes", "", false)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"source":"AUTO_SWITCHED"`) || !strings.Contains(list.Body.String(), `"chunkable":true`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	set := modesRequest(handler, http.MethodPut, "/api/v1/admin/tenants/"+tenant+"/report-modes/stock_balance", `{"mode":"DIRECT","reason":"ร้านเล็กลง"}`, true)
	if set.Code != http.StatusOK || api.setKey != report.StockBalance || api.setMode != report.ModeDirect || api.setReason != "ร้านเล็กลง" {
		t.Fatalf("set status=%d key=%s mode=%s body=%s", set.Code, api.setKey, api.setMode, set.Body.String())
	}

	measure := modesRequest(handler, http.MethodPost, "/api/v1/admin/tenants/"+tenant+"/report-modes/measure", "", true)
	if measure.Code != http.StatusOK || !strings.Contains(measure.Body.String(), `"units":4200`) || !strings.Contains(measure.Body.String(), `"applied":true`) {
		t.Fatalf("measure status=%d body=%s", measure.Code, measure.Body.String())
	}
}

func TestReportModesRejectBadInputAndMapErrors(t *testing.T) {
	tenant := uuid.New().String()
	api := &fakeModesAPI{}
	handler := newModesHandler(api, &fakeAdminAuth{})
	put := func(key, body string) *httptest.ResponseRecorder {
		return modesRequest(handler, http.MethodPut, "/api/v1/admin/tenants/"+tenant+"/report-modes/"+key, body, true)
	}

	for name, response := range map[string]*httptest.ResponseRecorder{
		"unknown report":   put("nope", `{"mode":"DIRECT"}`),
		"invalid mode":     put("stock_balance", `{"mode":"FAST"}`),
		"missing mode":     put("stock_balance", `{}`),
		"unknown field":    put("stock_balance", `{"mode":"DIRECT","extra":1}`),
		"malformed tenant": modesRequest(handler, http.MethodGet, "/api/v1/admin/tenants/not-a-uuid/report-modes", "", false),
	} {
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: status=%d body=%s", name, response.Code, response.Body.String())
		}
	}

	for name, test := range map[string]struct {
		setErr, measureErr error
		wantStatus         int
		wantCode           string
	}{
		"not chunkable":  {setErr: executionmode.ErrUnsupported, wantStatus: http.StatusUnprocessableEntity, wantCode: "REPORT_MODE_UNSUPPORTED"},
		"tenant missing": {setErr: executionmode.ErrTenantNotFound, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		"tenant busy":    {measureErr: executionmode.ErrTenantBusy, wantStatus: http.StatusConflict, wantCode: "TENANT_BUSY"},
		"no connection":  {measureErr: sml.ErrConnectionNotConfigured, wantStatus: http.StatusConflict, wantCode: "SML_NOT_CONFIGURED"},
	} {
		api.setErr, api.measureErr = test.setErr, test.measureErr
		var response *httptest.ResponseRecorder
		if test.setErr != nil {
			response = put("stock_balance", `{"mode":"CHUNKED"}`)
		} else {
			response = modesRequest(handler, http.MethodPost, "/api/v1/admin/tenants/"+tenant+"/report-modes/measure", "", true)
		}
		if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantCode) {
			t.Fatalf("%s: status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
}

func TestReportModesRequireAdminAndCSRFForChanges(t *testing.T) {
	tenant := uuid.New().String()
	api := &fakeModesAPI{}

	unauthenticated := newModesHandler(api, &fakeAdminAuth{authErr: auth.ErrInvalidSession})
	if response := modesRequest(unauthenticated, http.MethodGet, "/api/v1/admin/tenants/"+tenant+"/report-modes", "", false); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status=%d", response.Code)
	}

	badCSRF := newModesHandler(api, &fakeAdminAuth{csrfErr: auth.ErrInvalidCSRF})
	for name, response := range map[string]*httptest.ResponseRecorder{
		"set":     modesRequest(badCSRF, http.MethodPut, "/api/v1/admin/tenants/"+tenant+"/report-modes/stock_balance", `{"mode":"DIRECT"}`, true),
		"measure": modesRequest(badCSRF, http.MethodPost, "/api/v1/admin/tenants/"+tenant+"/report-modes/measure", "", true),
	} {
		if response.Code != http.StatusForbidden && response.Code != http.StatusUnauthorized {
			t.Fatalf("%s with a bad CSRF token: status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
	if api.setKey != "" {
		t.Fatal("a rejected request must not reach the service")
	}
}
