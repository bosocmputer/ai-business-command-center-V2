// Package linegate is the front door of the shared LINE channel. Every message on the channel reaches AI-BCC first; the gate
// finds out who is writing, which shops they may ask about, and passes the message on to that shop's assistant (and only that
// one). A person with several shops chooses one, and the choice is kept until they change it.
package linegate

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/google/uuid"
)

const (
	MessageNotEnabled  = "ยังไม่ได้เปิดใช้เลขา AI สำหรับบัญชีนี้ หรือสิทธิ์ใช้งานของร้านสิ้นสุดแล้ว กรุณาติดต่อผู้ดูแลระบบ"
	MessageChoose      = "ต้องการถามร้านไหน กดเลือกร้านด้านล่างได้เลย (พิมพ์ \"เปลี่ยนร้าน\" เมื่อต้องการสลับร้าน)"
	MessageUnavailable = "เลขา AI ของร้านนี้ยังไม่พร้อมตอบ ลองใหม่อีกครั้งในอีกสักครู่"
	MessageSelectFail  = "เลือกร้านนี้ไม่ได้ กรุณาลองใหม่อีกครั้ง"

	selectPrefix   = "gate=select&tenant="
	maxChoices     = 13 // LINE quick replies carry at most 13 buttons
	maxLabelRunes  = 20 // and a button label at most 20 characters
	maxShopRunes   = 60
	forwardTimeout = 10 * time.Second
	replyTimeout   = 8 * time.Second
)

var switchWords = map[string]bool{"เปลี่ยนร้าน": true, "เลือกร้าน": true, "สลับร้าน": true}

// Shop is one shop a person may ask about on the shared channel.
type Shop struct {
	TenantID uuid.UUID
	Name     string
	Host     string
}

// Store answers who may ask about what. Shops lists only shops where the person is active, has been given the assistant, the
// shop is within its access period, and its assistant is switched on for the shared channel.
type Store interface {
	Shops(ctx context.Context, lineHash []byte, now time.Time) (recipientID uuid.UUID, shops []Shop, err error)
	Selected(ctx context.Context, recipientID uuid.UUID) (uuid.UUID, bool, error)
	Select(ctx context.Context, recipientID, tenantID uuid.UUID, now time.Time) error
}

type Replier interface {
	Reply(ctx context.Context, replyToken string, messages ...json.RawMessage) error
}

// Forwarder hands one signed webhook to a shop's assistant.
type Forwarder interface {
	Forward(ctx context.Context, host string, body []byte, signature string) error
}

type hasher interface{ HashToken(string) []byte }

type Gate struct {
	secret    string
	store     Store
	tokens    hasher
	replier   Replier
	forwarder Forwarder
	logger    *slog.Logger
	now       func() time.Time
	slots     chan struct{}
}

func New(channelSecret string, store Store, tokens hasher, replier Replier, forwarder Forwarder, logger *slog.Logger, now func() time.Time) *Gate {
	return &Gate{secret: channelSecret, store: store, tokens: tokens, replier: replier, forwarder: forwarder, logger: logger, now: now, slots: make(chan struct{}, 16)}
}

// Dispatch handles a webhook that already passed the signature check, in the background so LINE gets its answer at once.
// When too many are in flight the webhook is dropped (and counted in the log) rather than queued without bound.
func (gate *Gate) Dispatch(body []byte) {
	select {
	case gate.slots <- struct{}{}:
	default:
		gate.warn("LINE gate dropped a webhook", "reason", "too many in flight")
		return
	}
	go func() {
		defer func() { <-gate.slots }()
		gate.Process(context.Background(), body)
	}()
}

// Process routes every event of one verified webhook. Nothing about the people or their words is logged.
func (gate *Gate) Process(ctx context.Context, body []byte) {
	var envelope struct {
		Destination string            `json:"destination"`
		Events      []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		gate.warn("LINE gate could not read a webhook")
		return
	}
	for _, raw := range envelope.Events {
		var event line.WebhookEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		gate.handle(ctx, envelope.Destination, raw, event)
	}
}

func (gate *Gate) handle(ctx context.Context, destination string, raw json.RawMessage, event line.WebhookEvent) {
	if event.Source.Type != "user" || len(event.Source.UserID) < 2 || len(event.Source.UserID) > 128 {
		return
	}
	isMessage := event.Type == line.WebhookEventMessage && event.Message != nil
	isPostback := event.Type == line.WebhookEventPostback && event.Postback != nil
	if !isMessage && !isPostback {
		return
	}
	now := gate.now().UTC()
	recipientID, shops, err := gate.store.Shops(ctx, gate.tokens.HashToken("line-user:"+event.Source.UserID), now)
	if err != nil {
		gate.warn("LINE gate could not look up the sender", "reason", "store error")
		gate.reply(ctx, event, text(MessageUnavailable))
		return
	}
	if len(shops) == 0 {
		if isMessage {
			gate.reply(ctx, event, text(MessageNotEnabled))
		}
		return
	}
	if isPostback && strings.HasPrefix(event.Postback.Data, "gate=") {
		gate.choose(ctx, event, recipientID, shops, now)
		return
	}
	if isMessage && event.Message.Type == "text" && len(shops) > 1 && switchWords[strings.TrimSpace(event.Message.Text)] {
		gate.reply(ctx, event, chooser(shops))
		return
	}
	target, ok := shops[0], true
	if len(shops) > 1 {
		target, ok = gate.selected(ctx, recipientID, shops)
		if !ok {
			if isMessage {
				gate.reply(ctx, event, chooser(shops))
			}
			return
		}
	}
	forwarded := raw
	if isMessage && event.Message.Type == "text" && len(shops) > 1 {
		forwarded = withShopPrefix(raw, target.Name)
	}
	gate.forward(ctx, destination, forwarded, event, target)
}

func (gate *Gate) selected(ctx context.Context, recipientID uuid.UUID, shops []Shop) (Shop, bool) {
	tenantID, found, err := gate.store.Selected(ctx, recipientID)
	if err != nil || !found {
		return Shop{}, false
	}
	for _, shop := range shops {
		if shop.TenantID == tenantID {
			return shop, true
		}
	}
	return Shop{}, false
}

// choose applies a tap on a shop button. The shop must be one the person may ask about right now; the id in the button is only
// a hint, never trusted.
func (gate *Gate) choose(ctx context.Context, event line.WebhookEvent, recipientID uuid.UUID, shops []Shop, now time.Time) {
	data := event.Postback.Data
	tenantID, err := uuid.Parse(strings.TrimPrefix(data, selectPrefix))
	if !strings.HasPrefix(data, selectPrefix) || err != nil {
		gate.reply(ctx, event, text(MessageSelectFail))
		return
	}
	for _, shop := range shops {
		if shop.TenantID != tenantID {
			continue
		}
		if err := gate.store.Select(ctx, recipientID, tenantID, now); err != nil {
			gate.warn("LINE gate could not keep a shop choice", "reason", "store error")
			gate.reply(ctx, event, text(MessageSelectFail))
			return
		}
		gate.reply(ctx, event, text(fmt.Sprintf("ตอนนี้ถามร้าน %s ได้เลย พิมพ์ \"เปลี่ยนร้าน\" เมื่อต้องการสลับร้าน", shop.Name)))
		return
	}
	gate.reply(ctx, event, text(MessageSelectFail))
}

func (gate *Gate) forward(ctx context.Context, destination string, raw json.RawMessage, event line.WebhookEvent, shop Shop) {
	body, err := json.Marshal(struct {
		Destination string            `json:"destination"`
		Events      []json.RawMessage `json:"events"`
	}{Destination: destination, Events: []json.RawMessage{raw}})
	if err != nil {
		return
	}
	forwardCtx, cancel := context.WithTimeout(ctx, forwardTimeout)
	defer cancel()
	err = gate.forwarder.Forward(forwardCtx, shop.Host, body, Sign(gate.secret, body))
	if err == nil {
		return
	}
	gate.warn("LINE gate could not pass a message on", "reason", classify(err))
	// A timeout may mean the assistant is busy and did take the message, so only a clear failure is told to the person.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return
	}
	gate.reply(ctx, event, text(MessageUnavailable))
}

func (gate *Gate) reply(ctx context.Context, event line.WebhookEvent, message json.RawMessage) {
	if event.ReplyToken == "" || gate.replier == nil {
		return
	}
	replyCtx, cancel := context.WithTimeout(ctx, replyTimeout)
	defer cancel()
	if err := gate.replier.Reply(replyCtx, event.ReplyToken, message); err != nil {
		gate.warn("LINE gate could not reply", "reason", "reply failed")
	}
}

func (gate *Gate) warn(message string, args ...any) {
	if gate.logger != nil {
		gate.logger.Warn(message, args...)
	}
}

func classify(err error) string {
	var netErr net.Error
	switch {
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	case errors.As(err, &netErr):
		return "connection failed"
	default:
		return "refused"
	}
}

// Sign gives the X-Line-Signature of a body: base64 HMAC-SHA256 keyed by the channel secret.
func Sign(channelSecret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(channelSecret))
	_, _ = mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func text(value string) json.RawMessage {
	encoded, _ := json.Marshal(map[string]string{"type": "text", "text": value})
	return encoded
}

func chooser(shops []Shop) json.RawMessage {
	type action struct {
		Type        string `json:"type"`
		Label       string `json:"label"`
		Data        string `json:"data"`
		DisplayText string `json:"displayText"`
	}
	type item struct {
		Type   string `json:"type"`
		Action action `json:"action"`
	}
	items := make([]item, 0, len(shops))
	for _, shop := range shops {
		if len(items) == maxChoices {
			break
		}
		items = append(items, item{Type: "action", Action: action{
			Type: "postback", Label: truncate(shop.Name, maxLabelRunes), Data: selectPrefix + shop.TenantID.String(), DisplayText: truncate("ร้าน "+shop.Name, maxShopRunes),
		}})
	}
	encoded, _ := json.Marshal(map[string]any{"type": "text", "text": MessageChoose, "quickReply": map[string]any{"items": items}})
	return encoded
}

func truncate(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

// withShopPrefix puts the shop the person chose in front of their words, so the shop's assistant can say which shop it is
// answering for. Anything it cannot rewrite is passed on as it came.
func withShopPrefix(raw json.RawMessage, shopName string) json.RawMessage {
	var event map[string]json.RawMessage
	if json.Unmarshal(raw, &event) != nil {
		return raw
	}
	var message map[string]json.RawMessage
	if json.Unmarshal(event["message"], &message) != nil {
		return raw
	}
	var current string
	if json.Unmarshal(message["text"], &current) != nil {
		return raw
	}
	prefixed, _ := json.Marshal("[ถามร้าน " + shopName + "] " + current)
	message["text"] = prefixed
	event["message"], _ = json.Marshal(message)
	rebuilt, err := json.Marshal(event)
	if err != nil {
		return raw
	}
	return rebuilt
}

// HTTPForwarder posts to the assistant named by a single internal host name.
type HTTPForwarder struct {
	client *http.Client
	// Template holds {host}, for example http://{host}:8646/line/webhook.
	template string
}

func NewHTTPForwarder(template string) (*HTTPForwarder, error) {
	if strings.Count(template, "{host}") != 1 {
		return nil, errors.New("assistant URL template must contain {host} once")
	}
	probe, err := url.Parse(strings.Replace(template, "{host}", "assistant", 1))
	if err != nil || probe.Scheme != "http" || probe.Host == "" || probe.User != nil {
		return nil, errors.New("assistant URL template must be an http URL")
	}
	return &HTTPForwarder{client: &http.Client{Timeout: forwardTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects are not followed") }}, template: template}, nil
}

func (forwarder *HTTPForwarder) Forward(ctx context.Context, host string, body []byte, signature string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.Replace(forwarder.template, "{host}", host, 1), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Line-Signature", signature)
	response, err := forwarder.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		return fmt.Errorf("assistant answered %d", response.StatusCode)
	}
	return nil
}
