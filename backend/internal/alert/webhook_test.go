package alert

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/google/uuid"
)

const testSecret = "0123456789abcdef0123456789abcdef"

// The signature must be exactly what Hermes recomputes: hex HMAC-SHA256 over "<timestamp>.<body>".
func TestSignMatchesWhatHermesChecks(t *testing.T) {
	body := []byte(`{"event_type":"aibcc.alert"}`)
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte("1790000000." + string(body)))
	if got, want := Sign(testSecret, 1790000000, body), hex.EncodeToString(mac.Sum(nil)); got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
	if Sign(testSecret, 1790000001, body) == Sign(testSecret, 1790000000, body) || Sign("another-secret-another-secret-00000", 1790000000, body) == Sign(testSecret, 1790000000, body) {
		t.Fatal("the time and the secret must both change the signature")
	}
}

type received struct {
	route, requestID, timestamp, signature, contentType string
	body                                                []byte
}

func webhookServer(t *testing.T, statuses map[string]int) (*httptest.Server, *[]received) {
	t.Helper()
	var mutex sync.Mutex
	list := make([]received, 0)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mutex.Lock()
		list = append(list, received{route: request.URL.Path, requestID: request.Header.Get("X-Request-ID"), timestamp: request.Header.Get("X-Webhook-Timestamp"),
			signature: request.Header.Get("X-Webhook-Signature-V2"), contentType: request.Header.Get("Content-Type"), body: body})
		mutex.Unlock()
		status := statuses[request.URL.Path]
		if status == 0 {
			status = http.StatusOK
		}
		response.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return server, &list
}

func TestTheWebhookCarriesASignedJSONBodyToEveryRoute(t *testing.T) {
	server, list := webhookServer(t, nil)
	at := time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)
	sender := &WebhookSender{BaseURL: server.URL, Secret: testSecret, Routes: []string{"alert-1", "alert-2"}, Now: func() time.Time { return at }}
	event := agent.AlertEvent{ID: uuid.New(), Rule: agent.AlertAROverdue, Message: "ทดสอบ ข้อความไทย"}
	if err := sender.Send(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	if len(*list) != 2 || (*list)[0].route != "/webhooks/alert-1" || (*list)[1].route != "/webhooks/alert-2" {
		t.Fatalf("requests = %+v", *list)
	}
	for _, got := range *list {
		if got.contentType != "application/json" || got.timestamp != strconv.FormatInt(at.Unix(), 10) || got.signature != Sign(testSecret, at.Unix(), got.body) {
			t.Errorf("headers = %+v", got)
		}
		if got.requestID != event.ID.String()+":"+got.route[len("/webhooks/"):] {
			t.Errorf("request id = %s", got.requestID)
		}
		var body map[string]string
		if err := json.Unmarshal(got.body, &body); err != nil || body["event_type"] != EventType || body["message"] != "ทดสอบ ข้อความไทย" || body["alertId"] != event.ID.String() || body["rule"] != "ar_overdue" {
			t.Errorf("body = %s %v", got.body, err)
		}
	}
}

func TestOneAcceptingRouteIsEnoughAndAMissingRouteIsSkipped(t *testing.T) {
	server, list := webhookServer(t, map[string]int{"/webhooks/alert-2": http.StatusNotFound, "/webhooks/alert-3": http.StatusInternalServerError})
	sender := &WebhookSender{BaseURL: server.URL, Secret: testSecret, Routes: []string{"alert-1", "alert-2", "alert-3"}}
	if err := sender.Send(t.Context(), agent.AlertEvent{ID: uuid.New()}); err != nil {
		t.Fatalf("one route took it: %v", err)
	}
	if len(*list) != 3 {
		t.Fatalf("every route is tried: %d", len(*list))
	}
}

func TestDeliveryFailuresCarryOnlyAShortCode(t *testing.T) {
	cases := map[string]struct {
		statuses map[string]int
		want     string
	}{
		"every route missing": {map[string]int{"/webhooks/a": 404, "/webhooks/b": 404}, "NO_ROUTE"},
		"server error":        {map[string]int{"/webhooks/a": 500, "/webhooks/b": 502}, "HTTP_502"},
		"bad signature":       {map[string]int{"/webhooks/a": 401, "/webhooks/b": 401}, "HTTP_401"},
	}
	for name, c := range cases {
		server, _ := webhookServer(t, c.statuses)
		sender := &WebhookSender{BaseURL: server.URL, Secret: testSecret, Routes: []string{"a", "b"}}
		var sendError *SendError
		if err := sender.Send(t.Context(), agent.AlertEvent{ID: uuid.New(), Message: "secret figures 123"}); !errors.As(err, &sendError) || sendError.Code != c.want {
			t.Errorf("%s: err = %v, want code %s", name, err, c.want)
		} else if len(err.Error()) > 60 {
			t.Errorf("%s: the error text must stay short: %q", name, err.Error())
		}
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()
	sender := &WebhookSender{BaseURL: url, Secret: testSecret, Routes: []string{"a"}}
	var sendError *SendError
	if err := sender.Send(t.Context(), agent.AlertEvent{ID: uuid.New()}); !errors.As(err, &sendError) || sendError.Code != "UNREACHABLE" {
		t.Errorf("unreachable: %v", err)
	}
	if err := (&WebhookSender{BaseURL: url, Secret: testSecret}).Send(t.Context(), agent.AlertEvent{ID: uuid.New()}); !errors.As(err, &sendError) || sendError.Code != "NO_ROUTES" {
		t.Errorf("no routes: %v", err)
	}
}
