package engagement

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDefaultsAreValidAndKeepTodaysBehaviour(t *testing.T) {
	defaults := Defaults()
	if err := defaults.Validate(); err != nil {
		t.Fatal(err)
	}
	if defaults.AlertChannel != ChannelTelegram || !defaults.DigestEnabled || defaults.WeeklySummaryEnabled || !defaults.InactivityNoticeEnabled || defaults.InactivityQuietDays != 5 || defaults.Version != 0 {
		t.Fatalf("defaults changed behaviour: %+v", defaults)
	}
}

func TestValidateRefusesEveryOutOfRangeChoice(t *testing.T) {
	cases := map[string]func(*Settings){
		"unknown channel": func(s *Settings) { s.AlertChannel = "SMS" },
		"empty channel":   func(s *Settings) { s.AlertChannel = "" },
		"weekday 0":       func(s *Settings) { s.WeeklySummaryWeekday = 0 },
		"weekday 8":       func(s *Settings) { s.WeeklySummaryWeekday = 8 },
		"hour -1":         func(s *Settings) { s.WeeklySummaryHour = -1 },
		"hour 24":         func(s *Settings) { s.WeeklySummaryHour = 24 },
		"quiet 1 day":     func(s *Settings) { s.InactivityQuietDays = 1 },
		"quiet 61 days":   func(s *Settings) { s.InactivityQuietDays = 61 },
	}
	for name, change := range cases {
		settings := Defaults()
		change(&settings)
		if err := settings.Validate(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s must be refused, got %v", name, err)
		}
	}
	for _, channel := range []string{ChannelTelegram, ChannelLine, ChannelOff} {
		settings := Defaults()
		settings.AlertChannel = channel
		if err := settings.Validate(); err != nil {
			t.Fatalf("%s must be accepted: %v", channel, err)
		}
	}
}

type storeFake struct {
	calls    int
	expected int
	err      error
}

func (fake *storeFake) Get(context.Context, uuid.UUID) (Settings, error) { return Defaults(), nil }
func (fake *storeFake) Put(_ context.Context, _ []byte, _ string, _ uuid.UUID, settings Settings, expected int, _ time.Time) (Settings, error) {
	fake.calls++
	fake.expected = expected
	return settings, fake.err
}

func TestPutValidatesBeforeTouchingTheStoreAndPassesTheSeenVersion(t *testing.T) {
	fake := &storeFake{}
	service := NewService(fake, time.Now)
	bad := Defaults()
	bad.AlertChannel = "SMS"
	if _, err := service.Put(context.Background(), nil, "r", uuid.New(), bad); !errors.Is(err, ErrInvalid) || fake.calls != 0 {
		t.Fatalf("an invalid choice must stop before the store: err=%v calls=%d", err, fake.calls)
	}
	good := Defaults()
	good.Version = 4
	if _, err := service.Put(context.Background(), nil, "r", uuid.New(), good); err != nil || fake.calls != 1 || fake.expected != 4 {
		t.Fatalf("err=%v calls=%d expected=%d", err, fake.calls, fake.expected)
	}
}
