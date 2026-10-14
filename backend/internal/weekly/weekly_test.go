package weekly

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type storeFake struct {
	due      []Due
	claimed  map[string]bool
	released int
}

func key(due Due) string { return due.RecipientID.String() + due.LocalDay }

func (fake *storeFake) Due(context.Context, time.Time) ([]Due, error) { return fake.due, nil }
func (fake *storeFake) Claim(_ context.Context, due Due, _ time.Time) (bool, error) {
	if fake.claimed[key(due)] {
		return false, nil
	}
	fake.claimed[key(due)] = true
	return true, nil
}
func (fake *storeFake) Release(_ context.Context, due Due) error {
	delete(fake.claimed, key(due))
	fake.released++
	return nil
}

type senderFake struct {
	texts []string
	keys  []uuid.UUID
	err   error
}

func (fake *senderFake) SendText(_ context.Context, _ uuid.UUID, retryKey uuid.UUID, text string) error {
	if fake.err != nil {
		return fake.err
	}
	fake.texts, fake.keys = append(fake.texts, text), append(fake.keys, retryKey)
	return nil
}

func notifier(store *storeFake, sender *senderFake) *Notifier {
	return NewNotifier(store, sender, time.Now, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func due(fetched, opened, alerts int) Due {
	return Due{TenantID: uuid.New(), RecipientID: uuid.New(), Shop: "ร้านทดสอบ", LocalDay: "2026-10-19", Fetched: fetched, Opened: opened, Alerts: alerts}
}

func TestAWeekWithNothingToCountSendsNothing(t *testing.T) {
	store, sender := &storeFake{due: []Due{due(0, 0, 0)}, claimed: map[string]bool{}}, &senderFake{}
	summary := notifier(store, sender).RunOnce(context.Background())
	if summary.Skipped != 1 || summary.Sent != 0 || len(sender.texts) != 0 || len(store.claimed) != 0 {
		t.Fatalf("summary=%+v texts=%d claimed=%d", summary, len(sender.texts), len(store.claimed))
	}
}

func TestASummaryIsSentOncePerDayEvenIfThePassRunsAgain(t *testing.T) {
	one := due(5, 3, 1)
	store, sender := &storeFake{due: []Due{one}, claimed: map[string]bool{}}, &senderFake{}
	n := notifier(store, sender)
	if summary := n.RunOnce(context.Background()); summary.Sent != 1 {
		t.Fatalf("first pass = %+v", summary)
	}
	if summary := n.RunOnce(context.Background()); summary.Sent != 0 || len(sender.texts) != 1 {
		t.Fatalf("second pass must send nothing: %+v texts=%d", summary, len(sender.texts))
	}
	if sender.keys[0] != RetryKey(one) {
		t.Fatal("the retry key must come from owner, shop and day")
	}
}

func TestAFailedSendIsGivenBackSoTheNextPassTriesAgain(t *testing.T) {
	store, sender := &storeFake{due: []Due{due(2, 0, 0)}, claimed: map[string]bool{}}, &senderFake{err: errors.New("quota")}
	n := notifier(store, sender)
	if summary := n.RunOnce(context.Background()); summary.Failed != 1 || store.released != 1 || len(store.claimed) != 0 {
		t.Fatalf("summary=%+v released=%d claimed=%d", summary, store.released, len(store.claimed))
	}
	sender.err = nil
	if summary := n.RunOnce(context.Background()); summary.Sent != 1 {
		t.Fatalf("retry = %+v", summary)
	}
}

func TestMessageStatesOnlyCountsAndLeavesOutZeros(t *testing.T) {
	text := Message(due(5, 0, 2))
	for _, want := range []string{"ร้านทดสอบ", "ดึงตัวเลขให้คุณ 5 ครั้ง", "แจ้งเตือนที่ส่งให้ 2 เรื่อง"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "เปิดการ์ด") {
		t.Fatalf("a zero count must be left out: %q", text)
	}
}

func TestRetryKeyDiffersByDay(t *testing.T) {
	a := due(1, 1, 1)
	b := a
	b.LocalDay = "2026-10-26"
	if RetryKey(a) == RetryKey(b) || RetryKey(a) != RetryKey(a) {
		t.Fatal("same owner/day must repeat the key, another day must change it")
	}
}
