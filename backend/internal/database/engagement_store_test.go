package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/engagement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEngagementStoreDefaultsConflictAuditAndWhatItSwitchesOff(t *testing.T) {
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
	tenantID, recipientID := uuid.New(), uuid.New()
	defer func() { _, _ = pool.Exec(context.Background(), `delete from tenants where id = $1`, tenantID) }()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "engage-"+tenantID.String(), now.AddDate(1, 0, 0))
	exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, recipientID, []byte("engage-"+recipientID.String()), now)
	exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled, created_at) values ($1, $2, 'ACTIVE', true, $3)`, tenantID, recipientID, now.Add(-60*day))

	store := NewEngagementStore(pool)
	// Nothing chosen yet: the defaults, version 0; a shop that does not exist is refused.
	got, err := store.Get(ctx, tenantID)
	if err != nil || got != engagement.Defaults() {
		t.Fatalf("defaults = %+v %v", got, err)
	}
	if _, err := store.Get(ctx, uuid.New()); !errors.Is(err, engagement.ErrNotFound) {
		t.Fatalf("unknown shop: %v", err)
	}

	// The first save needs version 0; a later one needs the version it saw.
	chosen := engagement.Defaults()
	chosen.AlertChannel, chosen.DigestEnabled, chosen.InactivityQuietDays = engagement.ChannelLine, false, 9
	if _, err := store.Put(ctx, []byte("admin"), "req-1", tenantID, chosen, 3, now); !errors.Is(err, engagement.ErrConflict) {
		t.Fatalf("a first save claiming version 3 must conflict: %v", err)
	}
	saved, err := store.Put(ctx, []byte("admin"), "req-1", tenantID, chosen, 0, now)
	if err != nil || saved.Version != 1 || saved.AlertChannel != engagement.ChannelLine || saved.DigestEnabled || saved.InactivityQuietDays != 9 {
		t.Fatalf("first save = %+v %v", saved, err)
	}
	if _, err := store.Put(ctx, []byte("admin"), "req-2", tenantID, chosen, 0, now); !errors.Is(err, engagement.ErrConflict) {
		t.Fatalf("a stale version must conflict: %v", err)
	}
	chosen.WeeklySummaryEnabled = true
	if saved, err = store.Put(ctx, []byte("admin"), "req-3", tenantID, chosen, 1, now); err != nil || saved.Version != 2 || !saved.WeeklySummaryEnabled {
		t.Fatalf("second save = %+v %v", saved, err)
	}
	var audits int
	if err := pool.QueryRow(ctx, `select count(*) from audit_logs where tenant_id = $1 and action = 'TENANT_ENGAGEMENT_UPDATED'`, tenantID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audit records = %d %v (only the two saves count, the conflicts write nothing)", audits, err)
	}
	// A value outside the allowed range is refused by the table itself, not only by the service.
	if _, err := pool.Exec(ctx, `update tenant_engagement_settings set inactivity_quiet_days = 1 where tenant_id = $1`, tenantID); err == nil {
		t.Fatal("the table must refuse one quiet day")
	}

	// What the settings switch off. Alerts: the assistant's token and two rules (a threshold and the digest).
	agentStore := NewAgentStore(pool)
	expires := now.Add(90 * day)
	if _, err := agentStore.IssueToken(ctx, []byte("admin"), "r1", tenantID, recipientID, false, hashOf("engage"), expires, now); err != nil {
		t.Fatal(err)
	}
	principal, err := agentStore.Authenticate(ctx, hashOf("engage"), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentStore.UpsertAlert(ctx, principal, agent.AlertAROverdue, "500000", true, "req-a", now); err != nil {
		t.Fatal(err)
	}
	if _, err := agentStore.UpsertAlert(ctx, principal, agent.AlertMorningDigest, "1", true, "req-b", now); err != nil {
		t.Fatal(err)
	}
	targetKeys := func() map[agent.AlertRuleKey]bool {
		t.Helper()
		targets, err := agentStore.AlertTargets(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		keys := map[agent.AlertRuleKey]bool{}
		for _, target := range targets {
			if target.Principal.TenantID == tenantID {
				keys[target.Rule] = true
			}
		}
		return keys
	}
	// Channel LINE with the digest off: the threshold rule is still checked, the digest is not.
	if keys := targetKeys(); !keys[agent.AlertAROverdue] || keys[agent.AlertMorningDigest] {
		t.Fatalf("digest off: %+v", keys)
	}
	chosen.DigestEnabled = true
	if _, err = store.Put(ctx, []byte("admin"), "req-4", tenantID, chosen, 2, now); err != nil {
		t.Fatal(err)
	}
	if keys := targetKeys(); !keys[agent.AlertAROverdue] || !keys[agent.AlertMorningDigest] {
		t.Fatalf("digest on: %+v", keys)
	}
	// An alert made now is listed with the channel the shop chose (LINE here), and not listed at all once the shop has chosen no alerts.
	targets, err := agentStore.AlertTargets(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	var overdueTarget agent.AlertTarget
	for _, target := range targets {
		if target.Principal.TenantID == tenantID && target.Rule == agent.AlertAROverdue {
			overdueTarget = target
		}
	}
	if _, created, err := agentStore.RecordAlertFire(ctx, overdueTarget, "2026-10-14", "600000", "500000", "ข้อความทดสอบ", agent.AlertEventPending, now); err != nil || !created {
		t.Fatalf("fire: created=%v %v", created, err)
	}
	unsent := func() []agent.AlertEvent {
		t.Helper()
		events, err := agentStore.UnsentAlerts(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		mine := make([]agent.AlertEvent, 0)
		for _, event := range events {
			if event.TenantID == tenantID {
				mine = append(mine, event)
			}
		}
		return mine
	}
	if events := unsent(); len(events) != 1 || events[0].Channel != engagement.ChannelLine {
		t.Fatalf("unsent with channel LINE = %+v", events)
	}
	chosen.AlertChannel = engagement.ChannelOff
	if _, err = store.Put(ctx, []byte("admin"), "req-5", tenantID, chosen, 3, now); err != nil {
		t.Fatal(err)
	}
	if keys := targetKeys(); len(keys) != 0 {
		t.Fatalf("channel OFF must check nothing for the shop: %+v", keys)
	}
	if events := unsent(); len(events) != 0 {
		t.Fatalf("channel OFF must leave the shop's events undelivered: %+v", events)
	}

	// Inactivity: the owner last used the assistant six days ago. Nine quiet days chosen: not quiet yet. Five: quiet. Notice off: never listed.
	use := now.Add(-6 * day)
	exec(`insert into report_view_events (tenant_id, recipient_id, kind, occurred_at, expires_at) values ($1, $2, 'LINE_MESSAGE', $3, $4)`, tenantID, recipientID, use, use.Add(365*day))
	inactivityStore := NewInactivityStore(pool)
	listed := func() bool {
		t.Helper()
		found, err := inactivityStore.Candidates(ctx, now, 5*day, 7*day)
		if err != nil {
			t.Fatal(err)
		}
		for _, candidate := range found {
			if candidate.TenantID == tenantID {
				return true
			}
		}
		return false
	}
	if listed() {
		t.Fatal("nine quiet days chosen: six days of silence is not quiet yet")
	}
	chosen.InactivityQuietDays = 5
	if _, err = store.Put(ctx, []byte("admin"), "req-6", tenantID, chosen, 4, now); err != nil {
		t.Fatal(err)
	}
	if !listed() {
		t.Fatal("five quiet days chosen: six days of silence is quiet")
	}
	chosen.InactivityNoticeEnabled = false
	if _, err = store.Put(ctx, []byte("admin"), "req-7", tenantID, chosen, 5, now); err != nil {
		t.Fatal(err)
	}
	if listed() {
		t.Fatal("the notice is switched off for the shop")
	}
}
