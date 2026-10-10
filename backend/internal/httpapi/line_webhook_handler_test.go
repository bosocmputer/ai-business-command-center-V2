package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestLineWebhookIsPassedOnOnlyAfterTheSignatureChecksOut(t *testing.T) {
	received := make(chan [2]string, 4)
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		buffer := new(strings.Builder)
		_, _ = io.Copy(buffer, request.Body)
		received <- [2]string{buffer.String(), request.Header.Get("X-Line-Signature")}
	}))
	defer target.Close()

	accepted := NewHandler(Dependencies{LineWebhook: &fakeLineWebhook{}, LineWebhookForwardURL: target.URL})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(`{"events":[1]}`))
	request.Header.Set("X-Line-Signature", "sig")
	recorder := httptest.NewRecorder()
	accepted.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	select {
	case got := <-received:
		if got[0] != `{"events":[1]}` || got[1] != "sig" {
			t.Fatalf("the body and signature must be passed on untouched: %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a verified webhook was not passed on")
	}

	refused := NewHandler(Dependencies{LineWebhook: &fakeLineWebhook{err: recipient.ErrWebhookSignatureInvalid}, LineWebhookForwardURL: target.URL})
	request = httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(`{"events":[2]}`))
	request.Header.Set("X-Line-Signature", "forged")
	recorder = httptest.NewRecorder()
	refused.ServeHTTP(recorder, request)
	select {
	case got := <-received:
		t.Fatalf("a webhook with a bad signature must never be passed on: %v", got)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestLineWebhookPassOnNeverChangesTheAnswerToLine(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := down.URL
	down.Close() // nothing listens any more
	handler := NewHandler(Dependencies{LineWebhook: &fakeLineWebhook{}, LineWebhookForwardURL: url})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(`{"events":[]}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("a dead assistant must not make LINE see a failure: %d", recorder.Code)
	}
}

type fakeLineGate struct{ bodies []string }

func (gate *fakeLineGate) Dispatch(body []byte) { gate.bodies = append(gate.bodies, string(body)) }

func TestLineWebhookGoesToTheFrontGateInsteadOfThePlainPassOn(t *testing.T) {
	passedOn := make(chan struct{}, 1)
	assistant := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { passedOn <- struct{}{} }))
	defer assistant.Close()
	gate := &fakeLineGate{}
	handler := NewHandler(Dependencies{LineWebhook: &fakeLineWebhook{}, LineWebhookForwardURL: assistant.URL, LineFrontGate: gate})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(`{"events":[]}`))
	request.Header.Set("X-Line-Signature", "sig")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || len(gate.bodies) != 1 || gate.bodies[0] != `{"events":[]}` {
		t.Fatalf("status=%d gate=%v", recorder.Code, gate.bodies)
	}
	select {
	case <-passedOn:
		t.Fatal("the plain pass-on must not run when the gate is on")
	case <-time.After(200 * time.Millisecond):
	}

	// A webhook that failed the signature check never reaches the gate.
	gate.bodies = nil
	handler = NewHandler(Dependencies{LineWebhook: &fakeLineWebhook{err: recipient.ErrWebhookSignatureInvalid}, LineFrontGate: gate})
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/line/webhook", strings.NewReader(`{"events":[]}`)))
	if recorder.Code != http.StatusUnauthorized || len(gate.bodies) != 0 {
		t.Fatalf("status=%d gate=%v", recorder.Code, gate.bodies)
	}
}
