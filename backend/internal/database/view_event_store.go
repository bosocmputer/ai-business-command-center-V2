package database

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewevent"
	"github.com/jackc/pgx/v5/pgxpool"
)

const viewEventWriteTimeout = 2 * time.Second

// ViewEventStore records who opened what. Recording is best effort: a failure
// is logged by category and never reaches the viewer's request.
type ViewEventStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewViewEventStore(pool *pgxpool.Pool, logger *slog.Logger) *ViewEventStore {
	if logger == nil {
		logger = slog.Default()
	}
	return &ViewEventStore{pool: pool, logger: logger}
}

// Record stores the event in the background so the response is not delayed.
func (s *ViewEventStore) Record(event viewevent.Event) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), viewEventWriteTimeout)
		defer cancel()
		if err := s.Insert(ctx, event); err != nil {
			s.logger.Warn("view event not recorded", "error_category", "store")
		}
	}()
}

// Insert writes one event unless the same viewer did the same thing in the same
// place within DuplicateWindow.
func (s *ViewEventStore) Insert(ctx context.Context, event viewevent.Event) error {
	if !event.Kind.Valid() {
		return errors.New("view event kind is invalid")
	}
	_, err := s.pool.Exec(ctx, `
		insert into report_view_events (tenant_id, recipient_id, kind, report_key, delivery_id, occurred_at, expires_at)
		select $1, $2, $3, nullif($4, ''), $5, $6, $7
		where not exists (
		  select 1 from report_view_events
		  where tenant_id = $1 and recipient_id = $2 and kind = $3
		    and coalesce(report_key, '') = $4
		    and occurred_at > $6::timestamptz - make_interval(secs => $8)
		)`,
		event.TenantID, event.RecipientID, string(event.Kind), event.ReportKey, event.DeliveryID,
		event.At, event.At.Add(viewevent.Retention), viewevent.DuplicateWindow.Seconds())
	return err
}
