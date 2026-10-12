package linegate

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/google/uuid"
)

// The owner's menu buttons send these words. Report buttons are answered here with a card made from the shop's own figures, without the
// assistant (and so without a model call); "chat" answers with a few example questions. Anything else the person types still goes to the
// assistant. The words are matched exactly, so a longer question that merely contains them is an ordinary question.
const (
	TextYesterday = "ยอดขายเมื่อวาน"
	TextToday     = "ยอดขายล่าสุด"
	TextReorder   = "สินค้าใกล้หมด"
	TextChat      = "คุยกับเลขา AI"

	MessageCardFailed = "สร้างการ์ดนี้ไม่สำเร็จ ลองกดอีกครั้ง หรือพิมพ์ถามเลขา AI ได้เลย"
	MessageCardStale  = "หมายเหตุ: ดึงข้อมูลใหม่ไม่สำเร็จ จึงแสดงตัวเลขที่ดึงไว้ล่าสุด ดูเวลาอัปเดตในการ์ด"
)

type quickAction struct {
	kind     agent.QuickKind
	chat     bool
	followUp line.FooterButton
}

var quickActions = map[string]quickAction{
	TextYesterday: {kind: agent.QuickYesterday, followUp: line.FooterButton{Label: "ถามเลขา AI ต่อ", Text: "ยอดขายเมื่อวานต่างจากวันก่อนหน้าเพราะอะไร"}},
	TextToday:     {kind: agent.QuickToday, followUp: line.FooterButton{Label: "ถามเลขา AI ต่อ", Text: "ยอดขายวันนี้เทียบกับวันเดียวกันของสัปดาห์ก่อนเป็นยังไง"}},
	TextReorder:   {kind: agent.QuickReorder, followUp: line.FooterButton{Label: "ร่างใบสั่งซื้อ", Text: "ร่างใบสั่งซื้อสินค้าใกล้หมดให้หน่อย"}},
	TextChat:      {chat: true},
}

// Cards makes the answer to a report button: the messages to reply with.
type Cards interface {
	Card(ctx context.Context, recipientID uuid.UUID, shop Shop, severalShops bool, kind agent.QuickKind) ([]json.RawMessage, error)
}

// ReportSource is the part of the agent service the cards need.
type ReportSource interface {
	QuickReport(ctx context.Context, tenantID, recipientID uuid.UUID, kind agent.QuickKind) (agent.QuickResult, error)
}

// CardMaker turns a shop's figures into a LINE card.
type CardMaker struct {
	source  ReportSource
	baseURL *url.URL
}

func NewCardMaker(source ReportSource, publicBaseURL *url.URL) *CardMaker {
	return &CardMaker{source: source, baseURL: publicBaseURL}
}

func (maker *CardMaker) Card(ctx context.Context, recipientID uuid.UUID, shop Shop, severalShops bool, kind agent.QuickKind) ([]json.RawMessage, error) {
	result, err := maker.source.QuickReport(ctx, shop.TenantID, recipientID, kind)
	if err != nil {
		return nil, err
	}
	if result.Status != agent.QuickReady {
		message := result.Message
		if message == "" {
			message = MessageCardFailed
		}
		return []json.RawMessage{text(message)}, nil
	}
	if maker.baseURL == nil || maker.baseURL.Host == "" {
		return nil, errors.New("the public address of the viewer is not set")
	}
	overview := *maker.baseURL
	overview.Path = "/app/tenant/" + shop.TenantID.String()
	card, err := line.RenderFlex(line.FlexInput{
		TenantName: strings.TrimSpace(shop.Name), Timezone: "Asia/Bangkok", Period: result.Period, GeneratedAt: result.FinishedAt, ActionURL: overview.String(),
		Reports: []line.FlexReport{{Key: result.Key, Dashboard: &result.Dashboard, Period: result.Period, FinishedAt: result.FinishedAt}},
	})
	if err != nil {
		return nil, err
	}
	buttons := []line.FooterButton{}
	for _, action := range quickActions {
		if action.kind == kind {
			buttons = append(buttons, action.followUp)
		}
	}
	if severalShops {
		buttons = append(buttons, line.FooterButton{Label: "เปลี่ยนร้าน", Text: "เปลี่ยนร้าน"})
	}
	card, err = line.WithFooterButtons(card, buttons)
	if err != nil {
		return nil, err
	}
	messages := []json.RawMessage{card}
	if result.Stale {
		messages = append(messages, text(MessageCardStale))
	}
	return messages, nil
}

// chatInvitation answers the "chat with the assistant" button: where to type, and a few questions to tap.
func chatInvitation(shop Shop, severalShops bool) json.RawMessage {
	type action struct {
		Type  string `json:"type"`
		Label string `json:"label"`
		Text  string `json:"text"`
	}
	type item struct {
		Type   string `json:"type"`
		Action action `json:"action"`
	}
	examples := []action{
		{Type: "message", Label: "เทียบเดือนก่อน", Text: "ยอดขายเดือนนี้เทียบกับเดือนก่อน"},
		{Type: "message", Label: "ลูกหนี้ค้างสูงสุด", Text: "ลูกหนี้ใครค้างเยอะสุด"},
		{Type: "message", Label: "สินค้าขายดีเดือนนี้", Text: "สินค้าขายดี 5 อันดับเดือนนี้"},
		{Type: "message", Label: "ไฟล์ Excel ยอดขาย", Text: "ทำไฟล์ Excel ยอดขายเดือนนี้ให้หน่อย"},
	}
	items := make([]item, 0, len(examples)+1)
	for _, example := range examples {
		items = append(items, item{Type: "action", Action: example})
	}
	if severalShops {
		items = append(items, item{Type: "action", Action: action{Type: "message", Label: "เปลี่ยนร้าน", Text: "เปลี่ยนร้าน"}})
	}
	body := "พิมพ์ถามเลขา AI ของ " + shopLabel(shop.Name) + " ได้เลยครับ ภาษาไทยธรรมดา หรือกดตัวอย่างด้านล่าง"
	encoded, _ := json.Marshal(map[string]any{"type": "text", "text": body, "quickReply": map[string]any{"items": items}})
	return encoded
}

const cardTimeout = 28 * time.Second
