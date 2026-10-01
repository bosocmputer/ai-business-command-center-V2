package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/retention"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewevent"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestViewEventStoreCollapsesRepeatsAndIsRetained(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	tenantID, recipientID, otherRecipientID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Events', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "events-"+tenantID.String(), now.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{recipientID, otherRecipientID} {
		if _, err := pool.Exec(ctx, `
			insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, display_name_ciphertext, display_name_nonce, encryption_key_id, status, verified_at)
			values ($1, $2, '\x01', '\x02', '\x03', '\x04', 'test', 'ACTIVE', $3)`, id, []byte("events-"+id.String()), now); err != nil {
			t.Fatal(err)
		}
	}
	store := NewViewEventStore(pool, nil)
	count := func(where string, args ...any) (n int) {
		if err := pool.QueryRow(ctx, `select count(*) from report_view_events where tenant_id = $1 `+where, append([]any{tenantID}, args...)...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	event := func(recipient uuid.UUID, kind viewevent.Kind, key string, at time.Time) viewevent.Event {
		return viewevent.Event{TenantID: tenantID, RecipientID: recipient, Kind: kind, ReportKey: key, At: at}
	}

	if err := store.Insert(ctx, event(recipientID, viewevent.ReportView, "stock_balance", now)); err != nil {
		t.Fatal(err)
	}
	// A reload seconds later is the same open.
	if err := store.Insert(ctx, event(recipientID, viewevent.ReportView, "stock_balance", now.Add(20*time.Second))); err != nil {
		t.Fatal(err)
	}
	if got := count(``); got != 1 {
		t.Fatalf("a repeat within the window stored %d rows, want 1", got)
	}
	// Anything that differs is a separate event.
	for _, other := range []viewevent.Event{
		event(recipientID, viewevent.ReportView, "sales_goods_services", now.Add(30*time.Second)),
		event(otherRecipientID, viewevent.ReportView, "stock_balance", now.Add(30*time.Second)),
		event(recipientID, viewevent.CardOpen, "", now.Add(30*time.Second)),
		event(recipientID, viewevent.ReportView, "stock_balance", now.Add(2*time.Minute)),
	} {
		if err := store.Insert(ctx, other); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(``); got != 5 {
		t.Fatalf("stored %d rows, want 5 (first + 4 distinct)", got)
	}
	if got := count(`and report_key is null`); got != 1 {
		t.Fatalf("an empty report key must be stored as null, got %d", got)
	}
	if err := store.Insert(ctx, viewevent.Event{TenantID: tenantID, RecipientID: recipientID, Kind: "NOPE", At: now}); err == nil {
		t.Fatal("an unknown kind must be rejected")
	}

	// The retention worker removes expired events and nothing else.
	counts, err := NewRetentionStore(pool).Run(ctx, retention.ProductionPolicy(), now.Add(viewevent.Retention).Add(time.Hour))
	if err != nil || counts.ViewEvents != 5 {
		t.Fatalf("retention counts = %+v, %v; want 5 view events removed", counts, err)
	}
	if got := count(``); got != 0 {
		t.Fatalf("%d events survived retention", got)
	}

	// Erasing a recipient erases their events.
	if err := store.Insert(ctx, event(recipientID, viewevent.OverviewView, "", now)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `delete from line_recipients where id = $1`, recipientID); err != nil {
		t.Fatal(err)
	}
	if got := count(``); got != 0 {
		t.Fatalf("deleting the recipient left %d events", got)
	}
}
