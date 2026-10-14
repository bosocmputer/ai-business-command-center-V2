package inactivity

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

type fakeStore struct {
	candidates []Candidate
	marked     []Candidate
	err        error
}

func (store *fakeStore) Candidates(context.Context, time.Time, time.Duration, time.Duration) ([]Candidate, error) {
	return store.candidates, store.err
}
func (store *fakeStore) MarkNotified(_ context.Context, candidate Candidate, _ time.Time) error {
	store.marked = append(store.marked, candidate)
	return nil
}

type fakeNames map[uuid.UUID]string

func (names fakeNames) DisplayName(_ context.Context, id uuid.UUID) string { return names[id] }

type fakeSender struct {
	sent []string
	err  error
}

func (sender *fakeSender) Send(_ context.Context, text string) error {
	if sender.err != nil {
		return sender.err
	}
	sender.sent = append(sender.sent, text)
	return nil
}

var quiet = slog.New(slog.NewJSONHandler(io.Discard, nil))

// 10:00 in Bangkok on 14 Oct 2026.
var morning = time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC)

func candidates() (*Candidate, *Candidate) {
	last := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	inactive := &Candidate{TenantID: uuid.New(), RecipientID: uuid.New(), Shop: "ร้านทดสอบ", Kind: Inactive, LastActivity: &last}
	never := &Candidate{TenantID: uuid.New(), RecipientID: uuid.New(), Shop: "ร้านอีกแห่ง", Kind: NeverUsed, Joined: time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)}
	return inactive, never
}

func notifier(store Store, names Names, sender Sender, mode Mode, at time.Time) *Notifier {
	return NewNotifier(store, names, sender, mode, func() time.Time { return at }, quiet)
}

func TestAnOwnerWhoWentQuietIsToldToTheOperatorOnceWithTheNameAndTheDays(t *testing.T) {
	inactive, never := candidates()
	store := &fakeStore{candidates: []Candidate{*inactive, *never}}
	sender := &fakeSender{}
	summary := notifier(store, fakeNames{inactive.RecipientID: "คุณสมชาย"}, sender, ModeSend, morning).RunOnce(context.Background())
	if summary.Candidates != 2 || summary.Told != 2 || len(store.marked) != 2 || len(sender.sent) != 2 {
		t.Fatalf("summary = %+v, sent %d, marked %d", summary, len(sender.sent), len(store.marked))
	}
	for _, want := range []string{"คุณสมชาย", "ร้านทดสอบ", "6 วัน", "8 ต.ค. 2569"} {
		if !strings.Contains(sender.sent[0], want) {
			t.Errorf("the quiet owner's notice lacks %q:\n%s", want, sender.sent[0])
		}
	}
	for _, want := range []string{"ผู้รับรายหนึ่ง", "ร้านอีกแห่ง", "9 วัน", "ยังไม่เคยใช้"} {
		if !strings.Contains(sender.sent[1], want) {
			t.Errorf("the never-used notice lacks %q:\n%s", want, sender.sent[1])
		}
	}
	if strings.Contains(sender.sent[0]+sender.sent[1], "2026") {
		t.Error("years are written in the Buddhist Era")
	}
}

func TestObserveModeLooksButTellsNobodyAndRemembersNothing(t *testing.T) {
	inactive, _ := candidates()
	store := &fakeStore{candidates: []Candidate{*inactive}}
	sender := &fakeSender{}
	summary := notifier(store, fakeNames{}, sender, ModeObserve, morning).RunOnce(context.Background())
	if summary.Candidates != 1 || summary.Told != 0 || len(sender.sent) != 0 || len(store.marked) != 0 {
		t.Fatalf("summary = %+v, sent %d, marked %d", summary, len(sender.sent), len(store.marked))
	}
}

func TestOffModeDoesNotEvenLook(t *testing.T) {
	store := &fakeStore{err: errors.New("must not be called")}
	if summary := notifier(store, fakeNames{}, &fakeSender{}, ModeOff, morning).RunOnce(context.Background()); summary != (Summary{}) {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestNothingIsSaidBeforeTheMorningOfTheShopDay(t *testing.T) {
	inactive, _ := candidates()
	earlyBangkok := time.Date(2026, 10, 14, 0, 30, 0, 0, time.UTC) // 07:30 in Bangkok
	sender := &fakeSender{}
	if summary := notifier(&fakeStore{candidates: []Candidate{*inactive}}, fakeNames{}, sender, ModeSend, earlyBangkok).RunOnce(context.Background()); summary.Candidates != 0 || len(sender.sent) != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestANoticeThatCouldNotBeSentIsNotRememberedSoItIsTriedAgain(t *testing.T) {
	inactive, _ := candidates()
	store := &fakeStore{candidates: []Candidate{*inactive}}
	summary := notifier(store, fakeNames{}, &fakeSender{err: errors.New("down")}, ModeSend, morning).RunOnce(context.Background())
	if summary.Failed != 1 || summary.Told != 0 || len(store.marked) != 0 {
		t.Fatalf("summary = %+v, marked %d", summary, len(store.marked))
	}
}

func TestAStoreFailureIsSurvivedAndNobodyIsTold(t *testing.T) {
	sender := &fakeSender{}
	summary := notifier(&fakeStore{err: errors.New("down")}, fakeNames{}, sender, ModeSend, morning).RunOnce(context.Background())
	if summary != (Summary{}) || len(sender.sent) != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}
