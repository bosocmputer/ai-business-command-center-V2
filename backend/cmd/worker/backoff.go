package main

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const (
	errorBackoffInitial = time.Second
	errorBackoffMax     = 30 * time.Second
	idleDelay           = 500 * time.Millisecond
)

// errorBackoff spaces out retries while a lane keeps failing: 1 s, 2 s, 4 s ... up to 30 s, and back to 1 s after any
// success or idle pass. A lane that retried at once on every error wrote its error line thousands of times a second
// when the database was unreachable, which fills a disk through the container log.
type errorBackoff struct {
	initial, max, current time.Duration
}

func newErrorBackoff(initial, max time.Duration) *errorBackoff {
	return &errorBackoff{initial: initial, max: max}
}

func (backoff *errorBackoff) next() time.Duration {
	if backoff.current == 0 {
		backoff.current = backoff.initial
	} else if backoff.current *= 2; backoff.current > backoff.max {
		backoff.current = backoff.max
	}
	return backoff.current
}

func (backoff *errorBackoff) reset() { backoff.current = 0 }

// sleepContext waits for delay and reports false when the context ended first.
func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// runLane calls step until the context ends. A nil result continues at once, an idle result (nothing to do) waits idle,
// a cancelled context stops the lane, and any other error is logged and retried after a growing delay.
func runLane(ctx context.Context, logger *slog.Logger, message string, isIdle func(error) bool, step func(context.Context) error, backoff *errorBackoff, idle time.Duration, attrs ...any) {
	for ctx.Err() == nil {
		err := step(ctx)
		switch {
		case err == nil:
			backoff.reset()
		case isIdle(err):
			backoff.reset()
			if !sleepContext(ctx, idle) {
				return
			}
		case errors.Is(err, context.Canceled):
			return
		default:
			delay := backoff.next()
			logger.Error(message, append(attrs, "error", err, "retryInSeconds", delay.Seconds())...)
			if !sleepContext(ctx, delay) {
				return
			}
		}
	}
}
