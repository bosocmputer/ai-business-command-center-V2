package alert

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/google/uuid"
)

// RoutedSender sends an alert the way its shop's admin chose: through the assistant's webhook (Telegram) or as a LINE message.
type RoutedSender struct {
	Telegram Sender
	Line     Sender
}

func (sender RoutedSender) Send(ctx context.Context, event agent.AlertEvent) error {
	switch event.Channel {
	case engagement.ChannelLine:
		if sender.Line == nil {
			return &SendError{Code: "LINE_NOT_CONFIGURED"}
		}
		return sender.Line.Send(ctx, event)
	case engagement.ChannelOff:
		return &SendError{Code: "CHANNEL_OFF"}
	default:
		if sender.Telegram == nil {
			return &SendError{Code: "TELEGRAM_NOT_CONFIGURED"}
		}
		return sender.Telegram.Send(ctx, event)
	}
}

// LineReserve is the part of the month's message quota alerts never use: the report cards the shops are scheduled to receive come first.
const LineReserve = 40

// maxLineText is below LINE's 5000-character limit for one text message.
const maxLineText = 4500

type LinePusher interface {
	Push(ctx context.Context, lineUserID string, retryKey uuid.UUID, message json.RawMessage) line.PushResult
}

type LineRecipients interface {
	OutboundLineUserID(ctx context.Context, recipientID uuid.UUID) (string, error)
}

// LineSender pushes an alert to the person it was made for, as one text message. It costs one message of the shared channel's monthly quota,
// so it refuses when the quota is not known or would drop below LineReserve. The alert's id is the push's retry key: a second try of the same
// alert is accepted by LINE without a second message.
type LineSender struct {
	Pusher     LinePusher
	Recipients LineRecipients
	Left       func(ctx context.Context, now time.Time) (int, bool)
	Now        func() time.Time
}

func (sender *LineSender) Send(ctx context.Context, event agent.AlertEvent) error {
	return sender.push(ctx, event.Recipient, event.ID, event.Message, alertQuickReplies(event.Rule))
}

// SendText pushes one text message to a recipient under the same quota guard; retryKey makes a repeat of the same message harmless.
func (sender *LineSender) SendText(ctx context.Context, recipientID, retryKey uuid.UUID, message string) error {
	return sender.push(ctx, recipientID, retryKey, message, nil)
}

// quickReply is a button under the last message of a chat: a tap sends its text as if the owner had typed it. It costs nothing extra.
type quickReply struct {
	Type   string      `json:"type"`
	Action quickAction `json:"action"`
}

type quickAction struct {
	Type  string `json:"type"`
	Label string `json:"label"` // LINE allows at most 20 characters
	Text  string `json:"text"`
}

// alertFollowUps is the question an owner most likely asks next about an alert (the same questions the alert text suggests).
var alertFollowUps = map[agent.AlertRuleKey]string{
	agent.AlertAROverdue:      "ลูกหนี้รายไหนเลยกำหนดเยอะสุด",
	agent.AlertAROverYear:     "ลูกหนี้รายไหนค้างนานที่สุด",
	agent.AlertStockReorder:   "สินค้าอะไรใกล้หมดสต็อกบ้าง",
	agent.AlertCustomersQuiet: "ลูกค้ารายไหนเงียบเกินรอบเยอะสุด",
	agent.AlertSalesDrop:      "ยอดขายเมื่อวานเป็นอย่างไรบ้าง",
	agent.AlertReceiptsDrop:   "เงินเข้าเมื่อวานเป็นอย่างไรบ้าง",
	agent.AlertMarginDrop:     "กำไรขั้นต้นเมื่อวานเป็นอย่างไรบ้าง",
}

// alertQuickReplies gives an alert its two buttons, "ask more about this" and "switch this alert off", so an owner who does not type much can
// answer with a tap. The morning digest only gets the switch-off button.
func alertQuickReplies(rule agent.AlertRuleKey) []quickReply {
	button := func(label, text string) quickReply {
		return quickReply{Type: "action", Action: quickAction{Type: "message", Label: label, Text: text}}
	}
	if rule == agent.AlertMorningDigest {
		return []quickReply{button("ปิดสรุปเช้า", "ปิดสรุปเช้า")}
	}
	definition, known := agent.AlertRuleFor(string(rule))
	if !known {
		return nil
	}
	var buttons []quickReply
	if question := alertFollowUps[rule]; question != "" {
		buttons = append(buttons, button("ถามต่อเรื่องนี้", question))
	}
	return append(buttons, button("ปิดเตือนนี้", "ปิดเตือน"+definition.Label))
}

func (sender *LineSender) push(ctx context.Context, recipientID, retryKey uuid.UUID, message string, buttons []quickReply) error {
	left, known := sender.Left(ctx, sender.Now())
	if !known {
		return &SendError{Code: "QUOTA_UNKNOWN"}
	}
	if left <= LineReserve {
		return &SendError{Code: "QUOTA_LOW"}
	}
	lineUserID, err := sender.Recipients.OutboundLineUserID(ctx, recipientID)
	if err != nil {
		return &SendError{Code: "RECIPIENT_UNAVAILABLE"}
	}
	text := []rune(message)
	if len(text) > maxLineText {
		text = text[:maxLineText]
	}
	body := map[string]any{"type": "text", "text": string(text)}
	if len(buttons) > 0 {
		body["quickReply"] = map[string]any{"items": buttons}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return &SendError{Code: "ENCODE_FAILED"}
	}
	result := sender.Pusher.Push(ctx, lineUserID, retryKey, payload)
	if result.Outcome == line.PushAccepted {
		return nil
	}
	if result.SafeCode == "" {
		return errors.New("LINE push failed")
	}
	return &SendError{Code: result.SafeCode}
}
