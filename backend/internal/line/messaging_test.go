package line

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMessagingClientPersistsRetryIdentityAndClassifiesResponses(t *testing.T) {
	retryKey := uuid.New()
	for _, test := range []struct {
		name        string
		status      int
		wantOutcome PushOutcome
		wantCode    string
	}{
		{name: "accepted", status: http.StatusOK, wantOutcome: PushAccepted, wantCode: "LINE_PUSH_ACCEPTED"},
		{name: "duplicate accepted", status: http.StatusConflict, wantOutcome: PushAccepted, wantCode: "LINE_PUSH_ALREADY_ACCEPTED"},
		{name: "rate limited", status: http.StatusTooManyRequests, wantOutcome: PushRetryable, wantCode: "LINE_PUSH_RATE_LIMITED"},
		{name: "server unavailable", status: http.StatusServiceUnavailable, wantOutcome: PushRetryable, wantCode: "LINE_PUSH_UNAVAILABLE"},
		{name: "permanent", status: http.StatusBadRequest, wantOutcome: PushPermanent, wantCode: "LINE_PUSH_REJECTED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if got := request.Header.Get("X-Line-Retry-Key"); got != retryKey.String() {
					t.Errorf("X-Line-Retry-Key = %q", got)
				}
				if got := request.Header.Get("Authorization"); got != "Bearer "+testAccessToken() {
					t.Errorf("Authorization = %q", got)
				}
				var body struct {
					To       string            `json:"to"`
					Messages []json.RawMessage `json:"messages"`
				}
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.To != "U123" || len(body.Messages) != 1 {
					t.Errorf("push body = %+v, %v", body, err)
				}
				response.Header().Set("X-Line-Request-Id", "line-request-123")
				response.Header().Set("Retry-After", "12")
				response.WriteHeader(test.status)
			}))
			defer server.Close()
			client := NewMessagingClient(testAccessToken(), server.URL, time.Second)

			result := client.Push(context.Background(), "U123", retryKey, json.RawMessage(`{"type":"flex"}`))

			if result.Outcome != test.wantOutcome || result.SafeCode != test.wantCode || result.ProviderRequestID != "line-request-123" {
				t.Fatalf("Push() = %+v", result)
			}
			if test.status == http.StatusTooManyRequests && result.RetryAfter != 12*time.Second {
				t.Fatalf("RetryAfter = %s", result.RetryAfter)
			}
		})
	}
}

func TestMessagingClientTreatsNetworkFailureAsUncertain(t *testing.T) {
	client := NewMessagingClient(testAccessToken(), "http://127.0.0.1:1/unavailable", 50*time.Millisecond)
	result := client.Push(context.Background(), "U123", uuid.New(), json.RawMessage(`{"type":"flex"}`))
	if result.Outcome != PushRetryable || !result.Uncertain || result.SafeCode != "LINE_PUSH_UNCERTAIN" {
		t.Fatalf("Push() = %+v", result)
	}
}

func testAccessToken() string { return "test-access-token-value-that-is-long-enough" }

func TestReplySendsTheTokenAndMessageAndRefusesBadInput(t *testing.T) {
	var gotAuth, gotBody string
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		gotAuth, gotBody = request.Header.Get("Authorization"), string(body)
		response.WriteHeader(status)
	}))
	defer server.Close()
	client := NewMessagingClient(strings.Repeat("t", 40), DefaultPushEndpoint, time.Second)
	client.replyEndpoint = server.URL
	message := json.RawMessage(`{"type":"text","text":"hi"}`)
	if err := client.Reply(context.Background(), "reply-token-1", message); err != nil || gotAuth != "Bearer "+strings.Repeat("t", 40) || !strings.Contains(gotBody, `"replyToken":"reply-token-1"`) {
		t.Fatalf("err=%v auth ok=%v body=%s", err, gotAuth != "", gotBody)
	}
	status = http.StatusBadRequest
	if err := client.Reply(context.Background(), "reply-token-1", message); err == nil {
		t.Fatal("a refused reply must be an error")
	}
	if err := client.Reply(context.Background(), "", message); err == nil {
		t.Fatal("an empty reply token must be refused")
	}
	if err := client.Reply(context.Background(), "reply-token-1", json.RawMessage(`{broken`)); err == nil {
		t.Fatal("invalid JSON must be refused")
	}
}
