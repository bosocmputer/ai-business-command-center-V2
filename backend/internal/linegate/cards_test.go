package linegate

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewevent"
	"github.com/google/uuid"
)

type fakeCards struct {
	calls    []agent.QuickKind
	messages []json.RawMessage
	err      error
}

func (cards *fakeCards) Card(_ context.Context, _ uuid.UUID, _ Shop, kind agent.QuickKind) ([]json.RawMessage, error) {
	cards.calls = append(cards.calls, kind)
	return cards.messages, cards.err
}

func gateWithCards(shops []Shop, cards Cards) (*Gate, *fakeReplier, *fakeForwarder, *fakeStore) {
	store := &fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}
	forwarder := &fakeForwarder{}
	gate, replier := newGate(store, forwarder)
	return gate.WithCards(cards), replier, forwarder, store
}

func TestAReportButtonIsAnsweredWithACardAndNeverReachesTheAssistant(t *testing.T) {
	cards := &fakeCards{messages: []json.RawMessage{json.RawMessage(`{"type":"flex","altText":"card","contents":{}}`)}}
	gate, replier, forwarder, _ := gateWithCards(shopsOf("A"), cards)
	loader := &fakeLoader{}
	gate.WithLoader(loader)
	for text, want := range map[string]agent.QuickKind{TextYesterday: agent.QuickYesterday, TextToday: agent.QuickToday, TextReorder: agent.QuickReorder} {
		cards.calls = nil
		gate.Process(context.Background(), []byte(textEvent("Uone", text)))
		if len(cards.calls) != 1 || cards.calls[0] != want {
			t.Fatalf("%q: calls = %v", text, cards.calls)
		}
	}
	if len(forwarder.sent) != 0 || len(replier.replies) != 3 {
		t.Fatalf("forwarded=%d replies=%d", len(forwarder.sent), len(replier.replies))
	}
	if len(loader.users) != 3 {
		t.Fatalf("the loading animation should show while a card is made, saw %d", len(loader.users))
	}
}

func TestACardThatCannotBeMadeSaysSoInsteadOfStayingSilent(t *testing.T) {
	for _, cards := range []*fakeCards{{err: errors.New("boom")}, {}} {
		gate, replier, forwarder, _ := gateWithCards(shopsOf("A"), cards)
		gate.Process(context.Background(), []byte(textEvent("Uone", TextToday)))
		if len(forwarder.sent) != 0 || len(replier.replies) != 1 || replyText(t, replier.replies[0]) != MessageCardFailed {
			t.Fatalf("forwarded=%d replies=%d", len(forwarder.sent), len(replier.replies))
		}
	}
}

func TestTheChatButtonGivesExampleQuestionsAndCostsNoModelCall(t *testing.T) {
	cards := &fakeCards{}
	store := &fakeStore{recipient: uuid.New(), shops: shopsOf("A", "B"), selected: map[uuid.UUID]uuid.UUID{}}
	store.selected[store.recipient] = store.shops[0].TenantID
	forwarder := &fakeForwarder{}
	gate, replier := newGate(store, forwarder)
	gate.WithCards(cards)
	gate.Process(context.Background(), []byte(textEvent("Umany", TextChat)))
	if len(cards.calls) != 0 || len(forwarder.sent) != 0 || len(replier.replies) != 1 {
		t.Fatalf("cards=%d forwarded=%d replies=%d", len(cards.calls), len(forwarder.sent), len(replier.replies))
	}
	var message struct {
		Text       string `json:"text"`
		QuickReply struct {
			Items []struct {
				Action struct {
					Label, Text string
				} `json:"action"`
			} `json:"items"`
		} `json:"quickReply"`
	}
	if err := json.Unmarshal(replier.replies[0], &message); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message.Text, "ร้าน A") || len(message.QuickReply.Items) != 5 || message.QuickReply.Items[4].Action.Text != "เปลี่ยนร้าน" {
		t.Fatalf("message = %+v", message)
	}
	for _, item := range message.QuickReply.Items {
		if len([]rune(item.Action.Label)) > 20 {
			t.Fatalf("label %q is longer than LINE allows", item.Action.Label)
		}
	}
}

func TestTwoShopsWithNoChoiceStillAskWhichShopBeforeAnyCard(t *testing.T) {
	cards := &fakeCards{}
	gate, replier, _, _ := gateWithCards(shopsOf("A", "B"), cards)
	gate.Process(context.Background(), []byte(textEvent("Umany", TextToday)))
	if len(cards.calls) != 0 || len(replier.replies) != 1 || replyText(t, replier.replies[0]) != MessageChoose {
		t.Fatalf("cards=%d replies=%d", len(cards.calls), len(replier.replies))
	}
}

func TestAQuestionThatMerelyContainsAButtonWordGoesToTheAssistant(t *testing.T) {
	cards := &fakeCards{}
	gate, replier, forwarder, _ := gateWithCards(shopsOf("A"), cards)
	gate.Process(context.Background(), []byte(textEvent("Uone", "ยอดขายเมื่อวานต่างจากวันก่อนหน้าเพราะอะไร")))
	if len(cards.calls) != 0 || len(forwarder.sent) != 1 || len(replier.replies) != 0 {
		t.Fatalf("cards=%d forwarded=%d replies=%d", len(cards.calls), len(forwarder.sent), len(replier.replies))
	}
}

type fakeSource struct {
	result agent.QuickResult
	err    error
	asked  []agent.QuickKind
}

func (source *fakeSource) QuickReport(_ context.Context, _, _ uuid.UUID, kind agent.QuickKind) (agent.QuickResult, error) {
	source.asked = append(source.asked, kind)
	return source.result, source.err
}

func salesDashboard() report.Dashboard {
	period := report.Period{Preset: report.Custom, DateFrom: "2026-10-12", DateTo: "2026-10-12"}
	off := report.MetricComparison{Availability: report.ComparisonUnavailable}
	return report.Dashboard{
		ReportKey: report.SalesGoodsServices, Version: "1.0.0", Period: period, Timezone: "Asia/Bangkok",
		KPIs: []report.DashboardMetric{
			{Key: "total_amount", Label: "ยอดขายสุทธิ", Value: "1154929.00", Unit: report.UnitTHB, Comparison: off},
			{Key: "document_count", Label: "จำนวนเอกสาร", Value: "275", Unit: report.UnitCount, Comparison: off},
			{Key: "average_per_document", Label: "เฉลี่ยต่อเอกสาร", Value: "4200.00", Unit: report.UnitTHB, Comparison: off},
		},
		Quality: report.DashboardQuality{Status: "OK", Warnings: []string{}},
	}
}

func TestCardMakerBuildsTheMorningCardWithOnlyTheDetailsButton(t *testing.T) {
	base, _ := url.Parse("https://dashboard.example.com")
	dashboard := salesDashboard()
	run := uuid.New()
	source := &fakeSource{result: agent.QuickResult{Status: agent.QuickReady, Key: report.SalesGoodsServices, Period: dashboard.Period, Dashboard: dashboard, RunID: run, FinishedAt: time.Date(2026, 10, 12, 7, 39, 0, 0, time.UTC)}}
	shop := Shop{TenantID: uuid.New(), Name: "ร้านทดสอบ"}

	messages, err := NewCardMaker(source, base).Card(context.Background(), uuid.New(), shop, agent.QuickToday)
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages=%d err=%v", len(messages), err)
	}
	card := string(messages[0])
	link := "https://dashboard.example.com/app/tenant/" + shop.TenantID.String() + "/report/sales_goods_services?snapshotRunId=" + run.String()
	for _, want := range []string{`"type":"flex"`, "1,154,929", "เปิดรายละเอียด", link} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q: %s", want, card)
		}
	}
	// One button only: a person with one shop must never see a switch, and nothing else is added under the card.
	for _, unwanted := range []string{"เปลี่ยนร้าน", "ถามเลขา AI", "ร่างใบสั่งซื้อ", `"type":"message"`} {
		if strings.Contains(card, unwanted) {
			t.Fatalf("the card must not carry %q: %s", unwanted, card)
		}
	}
}

func TestCardMakerAnswersWithTextWhenTheFiguresAreNotReadyOrNotAllowed(t *testing.T) {
	base, _ := url.Parse("https://dashboard.example.com")
	for status, message := range map[string]string{agent.QuickPreparing: agent.MessageQuickPreparing, agent.QuickNotAllowed: agent.MessageQuickNotAllowed, agent.QuickUnavailable: ""} {
		maker := NewCardMaker(&fakeSource{result: agent.QuickResult{Status: status, Message: message}}, base)
		messages, err := maker.Card(context.Background(), uuid.New(), Shop{TenantID: uuid.New(), Name: "A"}, agent.QuickYesterday)
		if err != nil || len(messages) != 1 || strings.Contains(string(messages[0]), `"flex"`) {
			t.Fatalf("%s: %v %s", status, err, messages)
		}
	}
}

func TestCardMakerTellsWhenTheFiguresAreOlderThanAsked(t *testing.T) {
	base, _ := url.Parse("https://dashboard.example.com")
	dashboard := salesDashboard()
	source := &fakeSource{result: agent.QuickResult{Status: agent.QuickReady, Stale: true, Key: report.SalesGoodsServices, Period: dashboard.Period, Dashboard: dashboard, FinishedAt: time.Now()}}
	messages, err := NewCardMaker(source, base).Card(context.Background(), uuid.New(), Shop{TenantID: uuid.New(), Name: "A"}, agent.QuickToday)
	if err != nil || len(messages) != 2 || replyText(t, messages[1]) != MessageCardStale {
		t.Fatalf("messages=%d err=%v", len(messages), err)
	}
}

// The menu is built in one package and answered in another; this keeps the two lists of words equal.
func TestEveryMenuButtonTextIsAnsweredByTheGate(t *testing.T) {
	buttons := line.DefaultMenuButtons()
	if len(buttons) != len(quickActions) {
		t.Fatalf("%d buttons, %d gate actions", len(buttons), len(quickActions))
	}
	for _, button := range buttons {
		if _, ok := quickActions[button.Text]; !ok {
			t.Fatalf("the gate does not answer the button text %q", button.Text)
		}
	}
}

func TestTheChatButtonOffersASwitchOnlyToAPersonWithSeveralShops(t *testing.T) {
	one, many := string(chatInvitation(Shop{Name: "A"}, false)), string(chatInvitation(Shop{Name: "A"}, true))
	if strings.Contains(one, "เปลี่ยนร้าน") || !strings.Contains(many, "เปลี่ยนร้าน") {
		t.Fatalf("one shop: %s\nseveral: %s", one, many)
	}
}

type fakeActivity struct{ events []viewevent.Event }

func (recorder *fakeActivity) Record(event viewevent.Event) {
	recorder.events = append(recorder.events, event)
}

func TestWhatAPersonSendsToAnAssistantIsRecordedAsUseWithNoWords(t *testing.T) {
	shops := shopsOf("A")
	store := &fakeStore{recipient: uuid.New(), shops: shops, selected: map[uuid.UUID]uuid.UUID{}}
	activity := &fakeActivity{}
	gate, _ := newGate(store, &fakeForwarder{})
	gate.WithCards(&fakeCards{messages: []json.RawMessage{json.RawMessage(`{"type":"text","text":"x"}`)}}).WithActivity(activity)
	gate.Process(context.Background(), []byte(textEvent("Uone", "ยอดขายเมื่อวานเท่าไหร่")))
	gate.Process(context.Background(), []byte(textEvent("Uone", TextToday)))
	if len(activity.events) != 2 {
		t.Fatalf("events = %+v", activity.events)
	}
	for index, want := range []viewevent.Kind{viewevent.LineMessage, viewevent.LineMenu} {
		event := activity.events[index]
		if event.Kind != want || event.TenantID != shops[0].TenantID || event.RecipientID != store.recipient || event.ReportKey != "" || event.At.IsZero() {
			t.Fatalf("event %d = %+v, want kind %s", index, event, want)
		}
	}
}

func TestNothingIsRecordedForAStrangerOrBeforeAShopIsChosen(t *testing.T) {
	activity := &fakeActivity{}
	stranger, _ := newGate(&fakeStore{selected: map[uuid.UUID]uuid.UUID{}}, &fakeForwarder{})
	stranger.WithActivity(activity).Process(context.Background(), []byte(textEvent("Ustranger", "hello")))
	undecided, _ := newGate(&fakeStore{recipient: uuid.New(), shops: shopsOf("A", "B"), selected: map[uuid.UUID]uuid.UUID{}}, &fakeForwarder{})
	undecided.WithActivity(activity).Process(context.Background(), []byte(textEvent("Umany", "ยอดขาย")))
	if len(activity.events) != 0 {
		t.Fatalf("events = %+v", activity.events)
	}
}
