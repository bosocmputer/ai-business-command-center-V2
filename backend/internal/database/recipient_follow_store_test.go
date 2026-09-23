package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecordFollowStateIsOrderedAndAudited(t *testing.T) {
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

	now := time.Date(2026, 9, 23, 3, 0, 0, 0, time.UTC)
	tenantID, recipientID := uuid.New(), uuid.New()
	lineHash := []byte("follow-hash-" + recipientID.String())
	if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Follow', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "follow-"+tenantID.String(), now.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at)
		values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, recipientID, lineHash, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into tenant_memberships (tenant_id, recipient_id, status) values ($1, $2, 'ACTIVE')`, tenantID, recipientID); err != nil {
		t.Fatal(err)
	}

	store := NewRecipientStore(pool)
	followState := func() (string, time.Time) {
		var state string
		var changedAt time.Time
		if err := pool.QueryRow(ctx, `select line_follow_status, line_follow_changed_at from line_recipients where id = $1`, recipientID).Scan(&state, &changedAt); err != nil {
			t.Fatal(err)
		}
		return state, changedAt
	}

	blockedAt := now.Add(-time.Minute)
	if changed, err := store.RecordFollowState(ctx, lineHash, recipient.FollowStateBlocked, blockedAt, now); err != nil || !changed {
		t.Fatalf("first block = %v, %v", changed, err)
	}
	if state, at := followState(); state != "BLOCKED" || !at.Equal(blockedAt) {
		t.Fatalf("state after block = %s at %s", state, at)
	}

	// An older follow delivered late must not undo the newer block.
	if changed, err := store.RecordFollowState(ctx, lineHash, recipient.FollowStateFollowing, blockedAt.Add(-time.Hour), now); err != nil || changed {
		t.Fatalf("stale follow = %v, %v", changed, err)
	}
	// A redelivery of the same event is a no-op.
	if changed, err := store.RecordFollowState(ctx, lineHash, recipient.FollowStateBlocked, blockedAt, now); err != nil || changed {
		t.Fatalf("redelivered block = %v, %v", changed, err)
	}
	if changed, err := store.RecordFollowState(ctx, lineHash, recipient.FollowStateFollowing, now, now); err != nil || !changed {
		t.Fatalf("newer follow = %v, %v", changed, err)
	}
	if state, _ := followState(); state != "FOLLOWING" {
		t.Fatalf("state after follow = %s", state)
	}

	if changed, err := store.RecordFollowState(ctx, []byte("unknown-user"), recipient.FollowStateBlocked, now, now); err != nil || changed {
		t.Fatalf("unknown user = %v, %v", changed, err)
	}

	var blockedAudits, followedAudits int
	if err := pool.QueryRow(ctx, `
		select count(*) filter (where action = 'LINE_RECIPIENT_BLOCKED'),
		       count(*) filter (where action = 'LINE_RECIPIENT_FOLLOWED')
		from audit_logs
		where tenant_id = $1 and resource_id = $2 and actor_type = 'SYSTEM'`, tenantID, recipientID.String()).Scan(&blockedAudits, &followedAudits); err != nil {
		t.Fatal(err)
	}
	if blockedAudits != 1 || followedAudits != 1 {
		t.Fatalf("audit rows: blocked %d followed %d, want one each for applied changes only", blockedAudits, followedAudits)
	}
}
