package alert

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
)

// EventType is what the assistant's webhook route filters on.
const EventType = "aibcc.alert"

// SendError carries a short code, never the response body or the message, so nothing sensitive reaches a log or the table.
type SendError struct{ Code string }

func (err *SendError) Error() string { return "alert delivery failed: " + err.Code }

// WebhookSender posts an alert to the assistant's webhook, signed the way Hermes checks it: the header
// X-Webhook-Signature-V2 is the hex HMAC-SHA256 of "<unix seconds>.<body>", with the time in X-Webhook-Timestamp,
// which Hermes refuses when it is more than five minutes off. X-Request-ID makes a repeat of the same alert harmless.
type WebhookSender struct {
	BaseURL string
	Secret  string
	Routes  []string
	Client  *http.Client
	Now     func() time.Time
}

type webhookBody struct {
	EventType string `json:"event_type"`
	Message   string `json:"message"`
	AlertID   string `json:"alertId"`
	Rule      string `json:"rule"`
}

// Sign gives the signature header for a body sent at a moment.
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Send delivers to every route. It succeeds when at least one route took the alert. A route the assistant does not
// have answers 404 and is skipped, so a list of routes longer than the people to tell does no harm.
func (sender *WebhookSender) Send(ctx context.Context, event agent.AlertEvent) error {
	body, err := json.Marshal(webhookBody{EventType: EventType, Message: event.Message, AlertID: event.ID.String(), Rule: string(event.Rule)})
	if err != nil {
		return &SendError{Code: "ENCODE_FAILED"}
	}
	client := sender.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	now := time.Now
	if sender.Now != nil {
		now = sender.Now
	}
	accepted, missing := 0, 0
	lastCode := "NO_ROUTES"
	for _, route := range sender.Routes {
		timestamp := now().Unix()
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, sender.BaseURL+"/webhooks/"+route, bytes.NewReader(body))
		if requestErr != nil {
			return &SendError{Code: "BAD_URL"}
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Webhook-Timestamp", strconv.FormatInt(timestamp, 10))
		request.Header.Set("X-Webhook-Signature-V2", Sign(sender.Secret, timestamp, body))
		request.Header.Set("X-Request-ID", event.ID.String()+":"+route)
		response, doErr := client.Do(request)
		if doErr != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			lastCode = "UNREACHABLE"
			continue
		}
		_ = response.Body.Close()
		switch {
		case response.StatusCode >= 200 && response.StatusCode < 300:
			accepted++
		case response.StatusCode == http.StatusNotFound:
			missing++
		default:
			lastCode = fmt.Sprintf("HTTP_%d", response.StatusCode)
		}
	}
	switch {
	case accepted > 0:
		return nil
	case missing > 0 && missing == len(sender.Routes):
		return &SendError{Code: "NO_ROUTE"}
	default:
		return &SendError{Code: lastCode}
	}
}
