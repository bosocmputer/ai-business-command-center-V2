package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
)

type fakeLineWebhook struct {
	err       error
	body      string
	signature string
}

func (fake *fakeLineWebhook) HandleWebhook(_ context.Context, body []byte, signature string) error {
	fake.body, fake.signature = string(body), signature
	return fake.err
}

func TestLineWebhookRoute(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "accepted", wantStatus: http.StatusOK},
		{name: "not configured", err: recipient.ErrWebhookNotConfigured, wantStatus: http.StatusServiceUnavailable, wantCode: "LINE_WEBHOOK_NOT_CONFIGURED"},
		{name: "bad signature", err: recipient.ErrWebhookSignatureInvalid, wantStatus: http.StatusUnauthorized, wantCode: "LINE_SIGNATURE_INVALID"},
		{name: "bad payload", err: line.ErrWebhookPayloadInvalid, wantStatus: http.StatusBadRequest, wantCode: "VALIDATION_ERROR"},
		{name: "store failure", err: errors.New("database down"), wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			webhook := &fakeLineWebhook{err: tc.err}
			handler := NewHandler(Dependencies{LineWebhook: webhook})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(`{"events":[]}`))
			request.Header.Set("X-Line-Signature", "sig")
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tc.wantStatus, recorder.Body.String())
			}
			if tc.wantCode != "" && !strings.Contains(recorder.Body.String(), tc.wantCode) {
				t.Fatalf("body %q does not contain %s", recorder.Body.String(), tc.wantCode)
			}
			if webhook.body != `{"events":[]}` || webhook.signature != "sig" {
				t.Fatalf("raw body or signature not forwarded: %+v", webhook)
			}
		})
	}
}

func TestLineWebhookRejectsOversizedBody(t *testing.T) {
	webhook := &fakeLineWebhook{}
	handler := NewHandler(Dependencies{LineWebhook: webhook})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(strings.Repeat("x", lineWebhookBodyLimit+1)))
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge || webhook.body != "" {
		t.Fatalf("oversized body: status %d, forwarded %d bytes", recorder.Code, len(webhook.body))
	}
}
