package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/retention"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAlertRulesLifecycleTargetsEventsAndRetention(t *testing.T) {
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
	now := time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)
	tenantID, recipientID, otherRecipient := uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "alert-"+tenantID.String(), now.AddDate(1, 0, 0))
	for _, id := range []uuid.UUID{recipientID, otherRecipient} {
		exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, id, []byte(id.String()), now)
		exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, 'ACTIVE', true)`, tenantID, id)
	}
	store := NewAgentStore(pool)
	expires := now.Add(90 * 24 * time.Hour)
	if _, err := store.IssueToken(ctx, []byte("admin"), "r1", tenantID, recipientID, false, hashOf("owner"), expires, now); err != nil {
		t.Fatal(err)
	}
	principal, err := store.Authenticate(ctx, hashOf("owner"), now)
	if err != nil {
		t.Fatal(err)
	}

	// Setting a rule stores it trimmed, and a second set changes it in place.
	saved, err := store.UpsertAlert(ctx, principal, agent.AlertAROverdue, "500000", true, "req-a", now)
	if err != nil || saved.Threshold != "500000" || !saved.Enabled {
		t.Fatalf("first set = %+v %v", saved, err)
	}
	if _, err := store.UpsertAlert(ctx, principal, agent.AlertSalesDrop, "40", true, "req-b", now); err != nil {
		t.Fatal(err)
	}
	rules, err := store.Alerts(ctx, principal)
	if err != nil || len(rules) != 2 {
		t.Fatalf("rules = %+v %v", rules, err)
	}
	if got := map[agent.AlertRuleKey]string{rules[0].Rule: rules[0].Threshold, rules[1].Rule: rules[1].Threshold}; got[agent.AlertSalesDrop] != "40" || got[agent.AlertAROverdue] != "500000" {
		t.Fatalf("thresholds must keep their zeros (40 is not 4): %+v", got)
	}

	// Another recipient never sees or changes these.
	other, _ := store.IssueToken(ctx, []byte("admin"), "r2", tenantID, otherRecipient, false, hashOf("other"), expires, now)
	_ = other
	otherPrincipal, _ := store.Authenticate(ctx, hashOf("other"), now)
	if rules, _ := store.Alerts(ctx, otherPrincipal); len(rules) != 0 {
		t.Fatalf("a recipient sees someone else's alerts: %+v", rules)
	}

	// Only rules of people who can still use the assistant are checked.
	targets, err := store.AlertTargets(ctx, now)
	if err != nil || len(targets) != 2 {
		t.Fatalf("targets = %d %v", len(targets), err)
	}
	var overdue agent.AlertTarget
	for _, target := range targets {
		if target.Rule == agent.AlertAROverdue {
			overdue = target
		}
	}
	if overdue.Principal.RecipientID != recipientID || overdue.Principal.ShopName != "ร้านทดสอบ" || overdue.Threshold.FloatString(0) != "500000" || overdue.LastCheckedOn != "" {
		t.Fatalf("target = %+v", overdue)
	}

	// A check marks the day only when the data was usable.
	if err := store.MarkAlertChecked(ctx, overdue.RuleID, "2026-10-08", agent.AlertCheckNotReady); err != nil {
		t.Fatal(err)
	}
	if targets, _ = store.AlertTargets(ctx, now); targetFor(targets, agent.AlertAROverdue).LastCheckedOn != "" {
		t.Fatal("a not-ready check must not count as the day's check")
	}
	if err := store.MarkAlertChecked(ctx, overdue.RuleID, "2026-10-08", agent.AlertCheckOK); err != nil {
		t.Fatal(err)
	}
	if targets, _ = store.AlertTargets(ctx, now); targetFor(targets, agent.AlertAROverdue).LastCheckedOn != "2026-10-08" {
		t.Fatal("an OK check marks the day")
	}

	// Firing: recorded once a day, delivery tracked, retries bounded.
	event, created, err := store.RecordAlertFire(ctx, overdue, "2026-10-08", "641200.50", "500000", "ข้อความทดสอบ", agent.AlertEventPending, now)
	if err != nil || !created || event.ID == uuid.Nil {
		t.Fatalf("fire = %+v %v %v", event, created, err)
	}
	if _, created, err := store.RecordAlertFire(ctx, overdue, "2026-10-08", "700000", "500000", "อีกข้อความ", agent.AlertEventPending, now); err != nil || created {
		t.Fatalf("a second record the same day must be refused: created=%v err=%v", created, err)
	}
	if targets, _ = store.AlertTargets(ctx, now); targetFor(targets, agent.AlertAROverdue).LastFiredAt == nil || targetFor(targets, agent.AlertAROverdue).LastFiredValue.FloatString(2) != "641200.50" {
		t.Fatal("the rule must remember when and at what figure it last spoke")
	}
	unsent, err := store.UnsentAlerts(ctx, now)
	if err != nil || len(unsent) != 1 || unsent[0].Message != "ข้อความทดสอบ" || unsent[0].FiredOn != "2026-10-08" {
		t.Fatalf("unsent = %+v %v", unsent, err)
	}
	if err := store.MarkAlertDelivery(ctx, event.ID, false, "UNREACHABLE", now); err != nil {
		t.Fatal(err)
	}
	if unsent, _ = store.UnsentAlerts(ctx, now); len(unsent) != 1 || unsent[0].Attempts != 1 || unsent[0].Status != agent.AlertEventFailed {
		t.Fatalf("a failed try stays in the list: %+v", unsent)
	}
	if unsent, _ = store.UnsentAlerts(ctx, now.Add(7*time.Hour)); len(unsent) != 0 {
		t.Fatalf("after six hours it is dropped: %+v", unsent)
	}
	for i := 0; i < 4; i++ {
		_ = store.MarkAlertDelivery(ctx, event.ID, false, "UNREACHABLE", now)
	}
	if unsent, _ = store.UnsentAlerts(ctx, now); len(unsent) != 0 {
		t.Fatalf("after five tries it is dropped: %+v", unsent)
	}
	var lastError string
	if err := pool.QueryRow(ctx, `select last_error from agent_alert_events where id = $1`, event.ID).Scan(&lastError); err != nil || lastError != "UNREACHABLE" {
		t.Fatalf("only the short code is kept: %q %v", lastError, err)
	}

	// Dry-run events are never offered for delivery.
	if _, created, _ := store.RecordAlertFire(ctx, targetFor(targets, agent.AlertSalesDrop), "2026-10-08", "45", "40", "ซ้อม", agent.AlertEventDryRun, now); !created {
		t.Fatal("dry run record")
	}
	if unsent, _ = store.UnsentAlerts(ctx, now); len(unsent) != 0 {
		t.Fatalf("a dry run must never be delivered: %+v", unsent)
	}

	// Changing the threshold starts the rule afresh; switching off removes it from the daily check but keeps the figure.
	if _, err := store.UpsertAlert(ctx, principal, agent.AlertAROverdue, "800000", true, "req-c", now); err != nil {
		t.Fatal(err)
	}
	if targets, _ = store.AlertTargets(ctx, now); targetFor(targets, agent.AlertAROverdue).LastCheckedOn != "" || targetFor(targets, agent.AlertAROverdue).LastFiredAt != nil {
		t.Fatal("a new threshold must be checked again at once and forget the old firing")
	}
	if _, err := store.UpsertAlert(ctx, principal, agent.AlertAROverdue, "800000", false, "req-d", now); err != nil {
		t.Fatal(err)
	}
	if targets, _ = store.AlertTargets(ctx, now); len(targets) != 1 {
		t.Fatalf("a switched-off rule is not checked: %d targets", len(targets))
	}

	// Anyone who can no longer use the assistant has their rules left alone.
	exec(`update tenant_memberships set ai_chat_enabled = false where tenant_id = $1 and recipient_id = $2`, tenantID, recipientID)
	if targets, _ = store.AlertTargets(ctx, now); len(targets) != 0 {
		t.Fatalf("a recipient without the assistant must not be checked: %d", len(targets))
	}

	// Setting and changing a rule leaves audit rows that name the assistant and no raw token.
	var audit int
	if err := pool.QueryRow(ctx, `select count(*) from audit_logs where tenant_id = $1 and action = 'AGENT_ALERT_SET' and actor_type = 'VIEWER' and after_json->>'via' = 'assistant'`, tenantID).Scan(&audit); err != nil || audit != 4 {
		t.Fatalf("audit rows = %d %v, want 4", audit, err)
	}

	// Retention removes events past their date.
	exec(`update agent_alert_events set expires_at = $1`, now.Add(-time.Hour))
	counts, err := NewRetentionStore(pool).Run(ctx, retention.ProductionPolicy(), now)
	if err != nil || counts.AgentAlertEvents != 2 {
		t.Fatalf("retention = %+v %v", counts, err)
	}
}

func targetFor(targets []agent.AlertTarget, rule agent.AlertRuleKey) agent.AlertTarget {
	for _, target := range targets {
		if target.Rule == rule {
			return target
		}
	}
	return agent.AlertTarget{}
}
