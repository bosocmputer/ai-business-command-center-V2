package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

var errTest = errors.New("database unreachable")

func quietLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestErrorBackoffDoublesUpToTheCapAndResets(t *testing.T) {
	backoff := newErrorBackoff(time.Second, 30*time.Second)
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30}
	for index, seconds := range want {
		if got := backoff.next(); got != seconds*time.Second {
			t.Fatalf("failure %d waits %v, want %v", index+1, got, seconds*time.Second)
		}
	}
	backoff.reset()
	if got := backoff.next(); got != time.Second {
		t.Fatalf("after a success the wait is %v, want 1s", got)
	}
}

// A lane that fails every time must not retry in a tight loop: that is what filled a disk with error lines.
func TestRunLaneDoesNotRetryInATightLoopWhileFailing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var calls atomic.Int64
	runLane(ctx, quietLogger(), "test lane error", func(error) bool { return false },
		func(context.Context) error { calls.Add(1); return errTest },
		newErrorBackoff(5*time.Millisecond, 40*time.Millisecond), time.Millisecond)
	// Delays 5, 10, 20, 40, 40 ... ms: about nine calls in 200 ms. A retry with no delay would make thousands.
	if got := calls.Load(); got < 3 || got > 15 {
		t.Fatalf("lane called step %d times in 200 ms", got)
	}
}

func TestRunLaneStopsAtOnceWhenTheContextEndsDuringABackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runLane(ctx, quietLogger(), "test lane error", func(error) bool { return false },
			func(context.Context) error { return errTest },
			newErrorBackoff(time.Hour, time.Hour), time.Hour)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lane kept waiting out a one hour backoff after the context was cancelled")
	}
}

func TestRunLaneStopsOnACancelledStep(t *testing.T) {
	var calls atomic.Int64
	runLane(context.Background(), quietLogger(), "test lane error", func(error) bool { return false },
		func(context.Context) error { calls.Add(1); return context.Canceled },
		newErrorBackoff(time.Hour, time.Hour), time.Hour)
	if calls.Load() != 1 {
		t.Fatalf("step called %d times, want 1", calls.Load())
	}
}

func TestRunLaneResetsTheBackoffAfterASuccessAndAfterAnIdlePass(t *testing.T) {
	errIdle := errors.New("nothing to do")
	ctx, cancel := context.WithCancel(context.Background())
	backoff := newErrorBackoff(time.Millisecond, 4*time.Millisecond)
	results := []error{errTest, errTest, nil, errTest, errIdle}
	var index int
	step := func(context.Context) error {
		result := results[index]
		index++
		if index == len(results) {
			cancel() // the last pass is the idle one; stop after it
		}
		return result
	}
	runLane(ctx, quietLogger(), "test lane error", func(err error) bool { return errors.Is(err, errIdle) }, step, backoff, time.Millisecond)
	if index != len(results) {
		t.Fatalf("ran %d of %d passes", index, len(results))
	}
	if backoff.current != 0 {
		t.Fatalf("backoff is %v after an idle pass, want it reset", backoff.current)
	}
}
