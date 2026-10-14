package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/inactivity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestInactivityStoreFindsOnlyInvitedOwnersWhoWentQuietAndRemembersWhomItToldAbout(t *testing.T) {
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
	now := time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	tenantID := uuid.New()
	defer func() { _, _ = pool.Exec(context.Background(), `delete from tenants where id = $1`, tenantID) }()
	if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "inactive-"+tenantID.String(), now.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	addRecipient := func(status string, aiChat bool, joinedDaysAgo int) uuid.UUID {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `
			insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at)
			values ($1, $2, '\x01', '\x02', 'test', $3, $4)`, id, []byte("inactive-"+id.String()), status, now); err != nil {
			t.Fatal(err)
		}
		joined := now.Add(-time.Duration(joinedDaysAgo) * day)
		if _, err := pool.Exec(ctx, `insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled, created_at) values ($1, $2, 'ACTIVE', $3, $4)`, tenantID, id, aiChat, joined); err != nil {
			t.Fatal(err)
		}
		return id
	}
	use := func(id uuid.UUID, daysAgo int) {
		at := now.Add(-time.Duration(daysAgo) * day)
		if _, err := pool.Exec(ctx, `insert into report_view_events (tenant_id, recipient_id, kind, occurred_at, expires_at) values ($1, $2, 'LINE_MESSAGE', $3, $4)`, tenantID, id, at, at.Add(365*day)); err != nil {
			t.Fatal(err)
		}
	}
	quietOwner := addRecipient("ACTIVE", true, 60)
	use(quietOwner, 20)
	use(quietOwner, 6) // the newest use counts: six days ago
	activeOwner := addRecipient("ACTIVE", true, 60)
	use(activeOwner, 2)
	goneForGood := addRecipient("ACTIVE", true, 300)
	use(goneForGood, 120)
	staffMember := addRecipient("ACTIVE", false, 60)
	use(staffMember, 30)
	revoked := addRecipient("REVOKED", true, 60)
	use(revoked, 30)
	neverStarted := addRecipient("ACTIVE", true, 8)
	justJoined := addRecipient("ACTIVE", true, 3)
	_, _ = revoked, justJoined

	store := NewInactivityStore(pool)
	found, err := store.Candidates(ctx, now, 5*day, 7*day)
	if err != nil {
		t.Fatal(err)
	}
	byRecipient := map[uuid.UUID]inactivity.Candidate{}
	for _, candidate := range found {
		byRecipient[candidate.RecipientID] = candidate
	}
	if len(found) != 2 {
		t.Fatalf("found %d candidates: %+v", len(found), found)
	}
	if c := byRecipient[quietOwner]; c.Kind != inactivity.Inactive || c.LastActivity == nil || c.Shop != "ร้านทดสอบ" || c.TenantID != tenantID || now.Sub(*c.LastActivity) != 6*day {
		t.Fatalf("the quiet owner = %+v", c)
	}
	if c := byRecipient[neverStarted]; c.Kind != inactivity.NeverUsed || c.LastActivity != nil {
		t.Fatalf("the owner who never started = %+v", c)
	}

	// Once told, an owner is not told again for the same quiet spell, but is when they come back and go quiet again.
	for _, candidate := range found {
		if err := store.MarkNotified(ctx, candidate, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MarkNotified(ctx, byRecipient[quietOwner], now); err != nil {
		t.Fatalf("telling twice must not fail: %v", err)
	}
	if again, _ := store.Candidates(ctx, now.Add(day), 5*day, 7*day); len(again) != 0 {
		t.Fatalf("already told: %+v", again)
	}
	back := now.Add(2 * day)
	if _, err := pool.Exec(ctx, `insert into report_view_events (tenant_id, recipient_id, kind, occurred_at, expires_at) values ($1, $2, 'LINE_MENU', $3, $4)`, tenantID, quietOwner, back, back.Add(365*day)); err != nil {
		t.Fatal(err)
	}
	if again, _ := store.Candidates(ctx, back.Add(3*day), 5*day, 7*day); len(again) != 0 {
		t.Fatalf("only three days quiet: %+v", again)
	}
	later, _ := store.Candidates(ctx, back.Add(6*day), 5*day, 7*day)
	if len(later) != 1 || later[0].RecipientID != quietOwner || later[0].Kind != inactivity.Inactive {
		t.Fatalf("quiet again after coming back: %+v", later)
	}
}
