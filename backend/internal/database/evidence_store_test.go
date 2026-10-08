package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/retention"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEvidenceCaseKeepsTheProofAfterNormalRetentionDeletesIt(t *testing.T) {
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
	now := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	tenantID, otherTenant, runID, otherRun := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{tenantID, otherTenant} {
		exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "evidence-"+id.String(), now.AddDate(1, 0, 0))
	}
	for id, tenant := range map[uuid.UUID]uuid.UUID{runID: tenantID, otherRun: otherTenant} {
		exec(`
			insert into report_runs (id, tenant_id, report_key, source, result_kind, idempotency_key, status, period_preset, period_from, period_to,
			  queued_at, started_at, finished_at, source_finished_at, expires_at, created_at, updated_at, dashboard_json)
			values ($1, $2, 'sales_goods_services', 'SCHEDULE', 'SUMMARY', $3, 'SUCCEEDED', 'YESTERDAY', '2026-10-07', '2026-10-07', $4, $4, $4, $4, $5, $6, $6, '{"kpis":[{"key":"total_amount","value":"123456.00"}]}'::jsonb)`,
			id, tenant, "evidence-"+id.String(), now.Add(-time.Hour), now.Add(-time.Minute), now.AddDate(-1, -1, 0)) // older than a year: retention will delete it
	}
	recipient := uuid.New()
	exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, recipient, []byte(recipient.String()), now)
	exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, 'ACTIVE', true)`, tenantID, recipient)
	if _, err := NewAgentStore(pool).IssueToken(ctx, []byte("admin"), "evidence", tenantID, recipient, false, hashOf("evidence-"+tenantID.String()), now.AddDate(0, 0, 90), now); err != nil {
		t.Fatal(err)
	}
	var token uuid.UUID
	if err := pool.QueryRow(ctx, `select id from agent_tokens where tenant_id = $1`, tenantID).Scan(&token); err != nil {
		t.Fatal(err)
	}
	exec(`insert into agent_calls (token_id, tenant_id, recipient_id, tool, report_key, period_from, period_to, outcome, duration_ms, snapshot_run_id, created_at, expires_at)
	      values ($1, $2, $3, 'get_report', 'sales_goods_services', '2026-10-07', '2026-10-07', 'OK', 120, $4, $5, $6)`, token, tenantID, recipient, runID, now.Add(-30*time.Minute), now.Add(-time.Second))
	exec(`insert into agent_calls (token_id, tenant_id, recipient_id, tool, report_key, outcome, duration_ms, created_at, expires_at)
	      values ($1, $2, $3, 'context', null, 'OK', 5, $4, $5)`, token, tenantID, recipient, now.Add(-30*time.Hour), now.Add(-time.Second)) // outside the window

	if spent, err := NewAgentStore(pool).TenantCallsSince(ctx, tenantID, now.Add(-40*24*time.Hour)); err != nil || spent != 2 {
		t.Fatalf("a shop's calls this month: %d %v", spent, err)
	}
	if spent, err := NewAgentStore(pool).TenantCallsSince(ctx, otherTenant, now.Add(-40*24*time.Hour)); err != nil || spent != 0 {
		t.Fatalf("another shop's calls never count: %d %v", spent, err)
	}
	store := NewEvidenceStore(pool)
	summary, err := store.Capture(ctx, tenantID, now.Add(-2*time.Hour), now, "ยอดขายเมื่อวานไม่ตรง", "ทดสอบ", now)
	if err != nil || summary.Calls != 1 || summary.Snapshots != 1 || summary.Cards != 0 || summary.Bytes == 0 {
		t.Fatalf("summary = %+v %v", summary, err)
	}
	if summary.ExpiresAt.Sub(now) < 1000*24*time.Hour {
		t.Errorf("a case is kept about three years: %v", summary.ExpiresAt)
	}

	// Normal retention removes the call log and the stored report; the case still has both.
	if _, err := NewRetentionStore(pool).Run(ctx, retention.ProductionPolicy(), now); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx, `select (select count(*) from agent_calls where tenant_id = $1) + (select count(*) from report_runs where id = $2)`, tenantID, runID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("retention was expected to remove the originals, %d remain", remaining)
	}
	payload, err := store.Payload(ctx, tenantID, summary.ID)
	if err != nil || !strings.Contains(payload, "123456.00") || !strings.Contains(payload, runID.String()) || !strings.Contains(payload, "get_report") {
		t.Fatalf("the case must still hold the figure and the call: %v %.200s", err, payload)
	}
	if _, err := store.Payload(ctx, otherTenant, summary.ID); err == nil {
		t.Fatal("another shop must not read the case")
	}
	if list, err := store.List(ctx, otherTenant); err != nil || len(list) != 0 {
		t.Fatalf("another shop's list = %+v %v", list, err)
	}
	if list, err := store.List(ctx, tenantID); err != nil || len(list) != 1 || list[0].Reason != "ยอดขายเมื่อวานไม่ตรง" {
		t.Fatalf("list = %+v %v", list, err)
	}

	// An expired case is removed by retention; deleting is audited.
	exec(`update evidence_cases set expires_at = $2 where id = $1`, summary.ID, now.Add(-time.Hour))
	if _, err := NewRetentionStore(pool).Run(ctx, retention.ProductionPolicy(), now); err != nil {
		t.Fatal(err)
	}
	if list, _ := store.List(ctx, tenantID); len(list) != 0 {
		t.Fatalf("an expired case must go: %+v", list)
	}
	second, err := store.Capture(ctx, tenantID, now.Add(-2*time.Hour), now, "ซ้ำ", "ทดสอบ", now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted, err := store.Delete(ctx, otherTenant, second.ID, "ทดสอบ", now); err != nil || deleted {
		t.Fatalf("another shop cannot delete it: %v %v", deleted, err)
	}
	if deleted, err := store.Delete(ctx, tenantID, second.ID, "ทดสอบ", now); err != nil || !deleted {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	var audits int
	if err := pool.QueryRow(ctx, `select count(*) from audit_logs where tenant_id = $1 and action in ('EVIDENCE_CASE_CREATED', 'EVIDENCE_CASE_DELETED')`, tenantID).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("every capture and delete is audited: %d %v", audits, err)
	}
}
