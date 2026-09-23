package line

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	body := []byte(`{"destination":"U1","events":[]}`)

	if !VerifyWebhookSignature(secret, body, sign(secret, body)) {
		t.Fatal("valid signature rejected")
	}
	cases := map[string]string{
		"tampered body":   sign(secret, []byte(`{"destination":"U2","events":[]}`)),
		"other secret":    sign("fedcba9876543210fedcba9876543210", body),
		"not base64":      "%%%",
		"empty signature": "",
	}
	for name, signature := range cases {
		if VerifyWebhookSignature(secret, body, signature) {
			t.Fatalf("%s: signature accepted", name)
		}
	}
	if VerifyWebhookSignature("", body, sign("", body)) {
		t.Fatal("empty channel secret must never verify")
	}
}

func TestParseWebhook(t *testing.T) {
	webhook, err := ParseWebhook([]byte(`{"destination":"Ubot","events":[{"type":"unfollow","timestamp":1790133315806,"webhookEventId":"01H","source":{"type":"user","userId":"U123"},"mode":"active"}]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(webhook.Events) != 1 || webhook.Events[0].Type != WebhookEventUnfollow || webhook.Events[0].Source.UserID != "U123" || webhook.Events[0].Timestamp != 1790133315806 {
		t.Fatalf("unexpected webhook: %+v", webhook)
	}
	if _, err := ParseWebhook([]byte(`not json`)); !errors.Is(err, ErrWebhookPayloadInvalid) {
		t.Fatalf("invalid JSON error = %v", err)
	}
}
