package quota

import (
	"context"
	"time"
)

// Reader gives the figures the worker keeps in step with LINE.
type Reader interface {
	Get(ctx context.Context, now time.Time) (Status, error)
}

// MessagesLeft says how many messages the shared channel may still send this month. ok is false when the figure is not known (never synced,
// or the sync is failing): callers that spend the quota treat that as "do not send".
func MessagesLeft(reader Reader) func(ctx context.Context, now time.Time) (left int, ok bool) {
	return func(ctx context.Context, now time.Time) (int, bool) {
		status, err := reader.Get(ctx, now)
		if err != nil {
			return 0, false
		}
		switch status.State {
		case StateUnlimited:
			return 1 << 30, true
		case StateReady:
			if status.ProviderLimit != nil && status.ProviderConsumed != nil {
				return *status.ProviderLimit - *status.ProviderConsumed, true
			}
		}
		return 0, false
	}
}
