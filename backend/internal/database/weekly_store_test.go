package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/weekly"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWeeklyStoreListsOnlyDueOwnersOfShopsThatSwitchedItOnAndCountsTheirWeek(t *testing.T) {
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
		t.Fatal(err)
	}
	// Monday 19 Oct 2026, 09:30 in Bangkok (02:30 UTC).
	now := time.Date(2026, 10, 19, 2, 30, 0, 0, time.UTC)
	day := 24 * time.Hour
	tenantID, owner, staff := uuid.New(), uuid.New(), uuid.New()
	defer func() { _, _ = pool.Exec(context.Background(), `delete from tenants where id = $1`, tenantID) }()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "weekly-"+tenantID.String(), now.AddDate(1, 0, 0))
	for id, aiChat := range map[uuid.UUID]bool{owner: true, staff: false} {
		exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, id, []byte("weekly-"+id.String()), now)
		exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled, created_at) values ($1, $2, 'ACTIVE', $3, $4)`, tenantID, id, aiChat, now.Add(-60*day))
	}
	event := func(kind string, at time.Time) {
		exec(`insert into report_view_events (tenant_id, recipient_id, kind, occurred_at, expires_at) values ($1, $2, $3, $4, $5)`, tenantID, owner, kind, at, at.Add(365*day))
	}
	event("CARD_OPEN", now.Add(-1*day))
	event("LINE_MENU", now.Add(-2*day))
	event("LINE_MESSAGE", now.Add(-2*day)) // a message and a menu button both count as asking
	event("CARD_OPEN", now.Add(-9*day))    // older than a week: not counted

	store := NewWeeklyStore(pool)
	dueOf := func(at time.Time) []weekly.Due {
		t.Helper()
		list, err := store.Due(ctx, at)
		if err != nil {
			t.Fatal(err)
		}
		mine := make([]weekly.Due, 0)
		for _, due := range list {
			if due.TenantID == tenantID {
				mine = append(mine, due)
			}
		}
		return mine
	}
	// Not switched on: nobody. Switched on for Monday 08:00: the owner only (the staff member has no AI chat).
	if len(dueOf(now)) != 0 {
		t.Fatal("a shop that never chose the summary must list nobody")
	}
	engagementStore := NewEngagementStore(pool)
	chosen := engagement.Defaults()
	chosen.WeeklySummaryEnabled, chosen.WeeklySummaryWeekday, chosen.WeeklySummaryHour = true, 1, 10
	if _, err := engagementStore.Put(ctx, []byte("admin"), "w1", tenantID, chosen, 0, now); err != nil {
		t.Fatal(err)
	}
	if len(dueOf(now)) != 0 {
		t.Fatal("09:30 is before the chosen 10:00")
	}
	chosen.WeeklySummaryHour = 8
	if _, err := engagementStore.Put(ctx, []byte("admin"), "w2", tenantID, chosen, 1, now); err != nil {
		t.Fatal(err)
	}
	list := dueOf(now)
	if len(list) != 1 || list[0].RecipientID != owner || list[0].LocalDay != "2026-10-19" || list[0].Opened != 1 || list[0].Asked != 2 || list[0].Alerts != 0 {
		t.Fatalf("due = %+v", list)
	}
	if len(dueOf(now.Add(day))) != 0 {
		t.Fatal("Tuesday is not the chosen weekday")
	}
	claimed, err := store.Claim(ctx, list[0], now)
	if err != nil || !claimed {
		t.Fatalf("claim = %v %v", claimed, err)
	}
	if again, _ := store.Claim(ctx, list[0], now); again {
		t.Fatal("a second claim for the same day must be refused")
	}
	if len(dueOf(now)) != 0 {
		t.Fatal("a claimed summary must not be listed again")
	}
	if err := store.Release(ctx, list[0]); err != nil || len(dueOf(now)) != 1 {
		t.Fatalf("a released summary must be listed again: %v", err)
	}
}
