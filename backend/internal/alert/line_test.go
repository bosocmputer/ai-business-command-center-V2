package alert

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/line"
	"github.com/google/uuid"
)

type pusherFake struct {
	calls   int
	to      string
	key     uuid.UUID
	message json.RawMessage
	result  line.PushResult
}

func (fake *pusherFake) Push(_ context.Context, to string, key uuid.UUID, message json.RawMessage) line.PushResult {
	fake.calls++
	fake.to, fake.key, fake.message = to, key, message
	return fake.result
}

type recipientsFake struct {
	id  string
	err error
}

func (fake recipientsFake) OutboundLineUserID(context.Context, uuid.UUID) (string, error) {
	return fake.id, fake.err
}

func lineSender(pusher *pusherFake, recipients recipientsFake, left int, known bool) *LineSender {
	return &LineSender{Pusher: pusher, Recipients: recipients, Now: time.Now,
		Left: func(context.Context, time.Time) (int, bool) { return left, known }}
}

func sendCode(err error) string {
	var sendErr *SendError
	if errors.As(err, &sendErr) {
		return sendErr.Code
	}
	if err != nil {
		return "other"
	}
	return ""
}

func TestLineSenderPushesOneTextWithTheAlertIDAsRetryKey(t *testing.T) {
	pusher := &pusherFake{result: line.PushResult{Outcome: line.PushAccepted}}
	event := agent.AlertEvent{ID: uuid.New(), Recipient: uuid.New(), Message: "⚠️ แจ้งเตือน: ทดสอบ"}
	if err := lineSender(pusher, recipientsFake{id: "U123456"}, 250, true).Send(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	if err := json.Unmarshal(pusher.message, &body); err != nil || body["type"] != "text" || body["text"] != event.Message {
		t.Fatalf("message = %s %v", pusher.message, err)
	}
	if pusher.to != "U123456" || pusher.key != event.ID || pusher.calls != 1 {
		t.Fatalf("to=%q key=%s calls=%d", pusher.to, pusher.key, pusher.calls)
	}
}

func TestLineSenderKeepsTheReserveAndNeverSpendsAnUnknownQuota(t *testing.T) {
	for name, tc := range map[string]struct {
		left  int
		known bool
		code  string
	}{
		"unknown":           {250, false, "QUOTA_UNKNOWN"},
		"at the reserve":    {LineReserve, true, "QUOTA_LOW"},
		"below the reserve": {3, true, "QUOTA_LOW"},
	} {
		pusher := &pusherFake{result: line.PushResult{Outcome: line.PushAccepted}}
		err := lineSender(pusher, recipientsFake{id: "U123456"}, tc.left, tc.known).Send(context.Background(), agent.AlertEvent{ID: uuid.New(), Message: "x"})
		if sendCode(err) != tc.code || pusher.calls != 0 {
			t.Fatalf("%s: err=%v calls=%d", name, err, pusher.calls)
		}
	}
	pusher := &pusherFake{result: line.PushResult{Outcome: line.PushAccepted}}
	if err := lineSender(pusher, recipientsFake{id: "U123456"}, LineReserve+1, true).Send(context.Background(), agent.AlertEvent{ID: uuid.New(), Message: "x"}); err != nil {
		t.Fatalf("one above the reserve may send: %v", err)
	}
}

func TestLineSenderReportsAShortCodeForEveryFailure(t *testing.T) {
	event := agent.AlertEvent{ID: uuid.New(), Message: "x"}
	if code := sendCode(lineSender(&pusherFake{}, recipientsFake{err: errors.New("gone")}, 250, true).Send(context.Background(), event)); code != "RECIPIENT_UNAVAILABLE" {
		t.Fatalf("code = %s", code)
	}
	rejected := &pusherFake{result: line.PushResult{Outcome: line.PushPermanent, SafeCode: "LINE_PUSH_REJECTED"}}
	if code := sendCode(lineSender(rejected, recipientsFake{id: "U123456"}, 250, true).Send(context.Background(), event)); code != "LINE_PUSH_REJECTED" {
		t.Fatalf("code = %s", code)
	}
	retryable := &pusherFake{result: line.PushResult{Outcome: line.PushRetryable, SafeCode: "LINE_PUSH_UNAVAILABLE"}}
	if code := sendCode(lineSender(retryable, recipientsFake{id: "U123456"}, 250, true).Send(context.Background(), event)); code != "LINE_PUSH_UNAVAILABLE" {
		t.Fatalf("code = %s", code)
	}
}

func TestLineSenderCutsATooLongMessageBelowLINEsLimit(t *testing.T) {
	pusher := &pusherFake{result: line.PushResult{Outcome: line.PushAccepted}}
	event := agent.AlertEvent{ID: uuid.New(), Message: strings.Repeat("ก", 6000)}
	if err := lineSender(pusher, recipientsFake{id: "U123456"}, 250, true).Send(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	var body map[string]string
	_ = json.Unmarshal(pusher.message, &body)
	if got := len([]rune(body["text"])); got != maxLineText {
		t.Fatalf("text has %d characters", got)
	}
}

type senderFake struct{ calls int }

func (fake *senderFake) Send(context.Context, agent.AlertEvent) error { fake.calls++; return nil }

func TestRoutedSenderUsesTheChannelTheShopChose(t *testing.T) {
	telegram, lineFake := &senderFake{}, &senderFake{}
	routed := RoutedSender{Telegram: telegram, Line: lineFake}
	for channel, want := range map[string][2]int{engagement.ChannelTelegram: {1, 0}, "": {1, 0}, engagement.ChannelLine: {0, 1}} {
		telegram.calls, lineFake.calls = 0, 0
		if err := routed.Send(context.Background(), agent.AlertEvent{Channel: channel}); err != nil || telegram.calls != want[0] || lineFake.calls != want[1] {
			t.Fatalf("channel %q: err=%v telegram=%d line=%d", channel, err, telegram.calls, lineFake.calls)
		}
	}
	telegram.calls, lineFake.calls = 0, 0
	if code := sendCode(routed.Send(context.Background(), agent.AlertEvent{Channel: engagement.ChannelOff})); code != "CHANNEL_OFF" || telegram.calls+lineFake.calls != 0 {
		t.Fatalf("OFF must send nothing: code=%s", code)
	}
	if code := sendCode((RoutedSender{Telegram: telegram}).Send(context.Background(), agent.AlertEvent{Channel: engagement.ChannelLine})); code != "LINE_NOT_CONFIGURED" {
		t.Fatalf("code = %s", code)
	}
}
