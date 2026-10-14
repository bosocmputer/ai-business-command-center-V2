package quota

import (
	"context"
	"errors"
	"testing"
	"time"
)

type readerFake struct {
	status Status
	err    error
}

func (fake readerFake) Get(context.Context, time.Time) (Status, error) { return fake.status, fake.err }

func TestMessagesLeftOnlyAnswersFromKnownFigures(t *testing.T) {
	limit, used := 300, 50
	cases := map[string]struct {
		reader readerFake
		left   int
		ok     bool
	}{
		"ready":     {readerFake{status: Status{State: StateReady, ProviderLimit: &limit, ProviderConsumed: &used}}, 250, true},
		"unlimited": {readerFake{status: Status{State: StateUnlimited}}, 1 << 30, true},
		"no figure": {readerFake{status: Status{State: StateReady}}, 0, false},
		"failing":   {readerFake{status: Status{State: "ERROR"}}, 0, false},
		"error":     {readerFake{err: errors.New("down")}, 0, false},
	}
	for name, tc := range cases {
		left, ok := MessagesLeft(tc.reader)(context.Background(), time.Now())
		if left != tc.left || ok != tc.ok {
			t.Fatalf("%s: left=%d ok=%v", name, left, ok)
		}
	}
}
