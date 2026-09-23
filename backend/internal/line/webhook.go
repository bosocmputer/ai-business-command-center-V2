package line

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// WebhookEventType values the dashboard acts on. LINE sends many more; every
// other type is accepted and ignored so the platform never retries them.
const (
	WebhookEventFollow   = "follow"
	WebhookEventUnfollow = "unfollow"
)

var ErrWebhookPayloadInvalid = errors.New("LINE webhook payload is invalid")

type WebhookSource struct {
	Type   string `json:"type"`
	UserID string `json:"userId"`
}

type WebhookEvent struct {
	Type           string        `json:"type"`
	Timestamp      int64         `json:"timestamp"`
	WebhookEventID string        `json:"webhookEventId"`
	Source         WebhookSource `json:"source"`
}

type Webhook struct {
	Destination string         `json:"destination"`
	Events      []WebhookEvent `json:"events"`
}

// VerifyWebhookSignature checks X-Line-Signature, the base64 HMAC-SHA256 of the
// raw request body keyed by the Messaging API channel secret.
func VerifyWebhookSignature(channelSecret string, body []byte, signature string) bool {
	if channelSecret == "" || signature == "" {
		return false
	}
	provided, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(channelSecret))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}

// ParseWebhook decodes a signature-verified body. Unknown fields are allowed
// because LINE adds event properties without versioning the endpoint.
func ParseWebhook(body []byte) (Webhook, error) {
	var webhook Webhook
	if err := json.Unmarshal(body, &webhook); err != nil {
		return Webhook{}, ErrWebhookPayloadInvalid
	}
	return webhook, nil
}
