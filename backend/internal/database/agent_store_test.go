package database

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func hashOf(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }

func TestAgentTokenLifecycleAndEveryReasonToRefuse(t *testing.T) {
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
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	tenantID, recipientID := uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "agent-"+tenantID.String(), now.AddDate(1, 0, 0))
	exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, recipientID, []byte(recipientID.String()), now)
	exec(`insert into tenant_memberships (tenant_id, recipient_id, status) values ($1, $2, 'ACTIVE')`, tenantID, recipientID)
	exec(`insert into recipient_report_permissions (tenant_id, recipient_id, report_key) values ($1, $2, 'sales_goods_services'), ($1, $2, 'ar_aging')`, tenantID, recipientID)
	store := NewAgentStore(pool)
	expires := now.Add(90 * 24 * time.Hour)

	// A recipient who has not been given the assistant cannot be issued a token.
	if _, err := store.IssueToken(ctx, []byte("admin"), "req-1", tenantID, recipientID, false, hashOf("first"), expires, now); !errors.Is(err, agent.ErrAIChatDisabled) {
		t.Fatalf("issue without the assistant switch: %v", err)
	}
	if _, err := store.IssueToken(ctx, []byte("admin"), "req-1", tenantID, uuid.New(), false, hashOf("first"), expires, now); !errors.Is(err, agent.ErrRecipientNotFound) {
		t.Fatalf("issue for someone who is not in the shop: %v", err)
	}
	exec(`update tenant_memberships set ai_chat_enabled = true where tenant_id = $1 and recipient_id = $2`, tenantID, recipientID)

	info, err := store.IssueToken(ctx, []byte("admin"), "req-2", tenantID, recipientID, true, hashOf("first"), expires, now)
	if err != nil || info.Status != "ACTIVE" || !info.NamesVisible || info.ExpiresAt == nil {
		t.Fatalf("issue = %+v, %v", info, err)
	}
	principal, err := store.Authenticate(ctx, hashOf("first"), now)
	if err != nil || principal.TenantID != tenantID || principal.RecipientID != recipientID || !principal.NamesVisible || principal.ShopName != "ร้านทดสอบ" || principal.Timezone != "Asia/Bangkok" {
		t.Fatalf("authenticate = %+v, %v", principal, err)
	}

	// Issuing again revokes the first token at once.
	if _, err := store.IssueToken(ctx, []byte("admin"), "req-3", tenantID, recipientID, false, hashOf("second"), expires, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Authenticate(ctx, hashOf("first"), now); !errors.Is(err, agent.ErrUnauthorized) {
		t.Fatalf("the replaced token must stop working: %v", err)
	}
	if _, err := store.Authenticate(ctx, hashOf("second"), now); err != nil {
		t.Fatal(err)
	}

	refusals := map[string]func(){
		"switched off": func() {
			exec(`update tenant_memberships set ai_chat_enabled = false where tenant_id = $1 and recipient_id = $2`, tenantID, recipientID)
		},
		"membership ended": func() {
			exec(`update tenant_memberships set status = 'REVOKED' where tenant_id = $1 and recipient_id = $2`, tenantID, recipientID)
		},
		"recipient revoked": func() {
			exec(`update line_recipients set status = 'REVOKED' where id = $1`, recipientID)
		},
		"shop disabled": func() { exec(`update tenants set status = 'DISABLED' where id = $1`, tenantID) },
		"shop access over": func() {
			exec(`update tenants set access_ends_at = $2 where id = $1`, tenantID, now.Add(-time.Hour))
		},
	}
	restore := func() {
		exec(`update tenant_memberships set ai_chat_enabled = true, status = 'ACTIVE' where tenant_id = $1 and recipient_id = $2`, tenantID, recipientID)
		exec(`update line_recipients set status = 'ACTIVE' where id = $1`, recipientID)
		exec(`update tenants set status = 'ACTIVE', access_ends_at = $2 where id = $1`, tenantID, now.AddDate(1, 0, 0))
	}
	for name, change := range refusals {
		change()
		if _, err := store.Authenticate(ctx, hashOf("second"), now); !errors.Is(err, agent.ErrUnauthorized) {
			t.Errorf("%s: err = %v, want ErrUnauthorized", name, err)
		}
		restore()
	}
	if _, err := store.Authenticate(ctx, hashOf("second"), expires.Add(time.Second)); !errors.Is(err, agent.ErrUnauthorized) {
		t.Fatalf("an expired token: %v", err)
	}
	if _, err := store.Authenticate(ctx, hashOf("nothing"), now); !errors.Is(err, agent.ErrUnauthorized) {
		t.Fatalf("an unknown token: %v", err)
	}

	// The call log counts answered calls for the limit and fetches for the budget, and records no values.
	token := principal.TokenID
	_ = token
	second, _ := store.Authenticate(ctx, hashOf("second"), now)
	record := func(outcome agent.Outcome, at time.Time) {
		if err := store.RecordCall(ctx, agent.Call{TokenID: second.TokenID, TenantID: tenantID, RecipientID: recipientID, Tool: agent.ToolGetReport, ReportKey: "sales_goods_services", PeriodFrom: "2026-09-01", PeriodTo: "2026-09-30", Outcome: outcome, Duration: 12 * time.Millisecond}, at); err != nil {
			t.Fatal(err)
		}
	}
	record(agent.OutcomeOK, now)
	record(agent.OutcomePreparing, now)
	record(agent.OutcomePreparing, now) // asking again for the same report and period is not another fetch
	record(agent.OutcomeRateLimited, now)
	record(agent.OutcomeOK, now.Add(-2*time.Hour))
	if used, err := store.CallsSince(ctx, second.TokenID, now.Add(-time.Hour)); err != nil || used != 3 {
		t.Fatalf("answered calls in the last hour = %d, %v (want 3); a rate-limited refusal must not count", used, err)
	}
	if preparing, err := store.PreparingSince(ctx, tenantID, now.Add(-time.Hour)); err != nil || preparing != 1 {
		t.Fatalf("fetches started = %d, %v", preparing, err)
	}
	if err := store.TouchToken(ctx, second.TokenID, now); err != nil {
		t.Fatal(err)
	}
	status, err := store.TokenInfo(ctx, tenantID, recipientID, now)
	if err != nil || status.Status != "ACTIVE" || status.LastUsedAt == nil || status.Calls24h != 5 {
		t.Fatalf("token info = %+v, %v", status, err)
	}

	permitted, err := store.PermittedReports(ctx, second, now)
	if err != nil || len(permitted) != 2 || permitted[0] != report.ARAging || permitted[1] != report.SalesGoodsServices {
		t.Fatalf("permitted = %v, %v", permitted, err)
	}

	// Revoking ends the token, is quiet when there is none, and issue and revoke leave audit rows.
	if err := store.RevokeToken(ctx, []byte("admin"), "req-4", tenantID, recipientID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeToken(ctx, []byte("admin"), "req-5", tenantID, recipientID, now); err != nil {
		t.Fatalf("revoking twice: %v", err)
	}
	if _, err := store.Authenticate(ctx, hashOf("second"), now); !errors.Is(err, agent.ErrUnauthorized) {
		t.Fatalf("a revoked token: %v", err)
	}
	if info, _ := store.TokenInfo(ctx, tenantID, recipientID, now); info.Status != "NONE" {
		t.Fatalf("after revoke: %+v", info)
	}
	var issued, revoked int
	if err := pool.QueryRow(ctx, `
		select count(*) filter (where action = 'AGENT_TOKEN_ISSUED'), count(*) filter (where action = 'AGENT_TOKEN_REVOKED')
		from audit_logs where tenant_id = $1`, tenantID).Scan(&issued, &revoked); err != nil {
		t.Fatal(err)
	}
	if issued != 2 || revoked != 1 {
		t.Fatalf("audit rows: issued=%d revoked=%d, want 2 and 1 (the second revoke changed nothing)", issued, revoked)
	}
	// Only the hash is stored.
	var stored int
	if err := pool.QueryRow(ctx, `select count(*) from agent_tokens where token_hash = $1`, []byte("second")).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("a raw token must never be stored: %d, %v", stored, err)
	}
}

func TestLatestDeliveryReturnsWhatTheRecipientWasLastSent(t *testing.T) {
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
	now := time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC)
	tenantID, mine, other := uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Delivery', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "agent-delivery-"+tenantID.String(), now.AddDate(1, 0, 0))
	for _, id := range []uuid.UUID{mine, other} {
		exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, id, []byte(id.String()), now)
		exec(`insert into tenant_memberships (tenant_id, recipient_id, status) values ($1, $2, 'ACTIVE')`, tenantID, id)
	}
	scheduleID := uuid.New()
	exec(`insert into notification_schedules (id, tenant_id, name, status, local_time, timezone, period_preset) values ($1, $2, 'เช้า', 'ACTIVE', '09:00', 'Asia/Bangkok', 'YESTERDAY')`, scheduleID, tenantID)
	deliver := func(label string, total string, acceptedAt time.Time, recipient uuid.UUID, status string) {
		runID, notificationID := uuid.New(), uuid.New()
		dashboard := `{"reportKey":"sales_goods_services","version":"1.0.0","period":{"preset":"YESTERDAY","dateFrom":"2026-09-30","dateTo":"2026-09-30"},"kpis":[{"key":"total_amount","label":"ยอดขาย","value":"` + total + `","unit":"THB","comparison":{"availability":"UNAVAILABLE"}}],"visualizations":[],"quality":{"status":"OK","warnings":[]}}`
		exec(`
			insert into report_runs (id, tenant_id, report_key, source, result_kind, idempotency_key, status, period_preset, period_from, period_to,
			  queued_at, started_at, finished_at, source_finished_at, expires_at, created_at, updated_at, dashboard_json)
			values ($1, $2, 'sales_goods_services', 'SCHEDULE', 'SUMMARY', $3, 'SUCCEEDED', 'YESTERDAY', '2026-09-30', '2026-09-30', $4, $4, $4, $4, $5, $4, $4, $6::jsonb)`,
			runID, tenantID, "delivery-"+label+"-run", acceptedAt.Add(-time.Minute), acceptedAt.Add(24*time.Hour), dashboard)
		exec(`insert into notification_runs (id, tenant_id, schedule_id, scheduled_for, status) values ($1, $2, $3, $4, 'COMPLETED')`, notificationID, tenantID, scheduleID, acceptedAt)
		exec(`insert into notification_run_reports (notification_run_id, report_key, report_run_id) values ($1, 'sales_goods_services', $2)`, notificationID, runID)
		exec(`insert into line_deliveries (tenant_id, notification_run_id, recipient_id, status, accepted_at) values ($1, $2, $3, $4, $5)`, tenantID, notificationID, recipient, status, acceptedAt)
	}
	deliver("older", "100.00", now.Add(-48*time.Hour), mine, "ACCEPTED")
	deliver("newer", "200.00", now.Add(-24*time.Hour), mine, "ACCEPTED")
	deliver("failed", "300.00", now.Add(-1*time.Hour), mine, "FAILED_PERMANENT")
	deliver("someone", "400.00", now.Add(-2*time.Hour), other, "ACCEPTED")

	store := NewAgentStore(pool)
	got, err := store.LatestDelivery(ctx, agent.Principal{TenantID: tenantID, RecipientID: mine}, report.SalesGoodsServices)
	if err != nil || got.Dashboard.KPIs[0].Value != "200.00" || got.DeliveredAt.IsZero() {
		t.Fatalf("latest delivery = %+v, %v; want the newest accepted one sent to this recipient", got, err)
	}
	if _, err := store.LatestDelivery(ctx, agent.Principal{TenantID: tenantID, RecipientID: uuid.New()}, report.SalesGoodsServices); !errors.Is(err, agent.ErrNoData) {
		t.Fatalf("a recipient who was sent nothing: %v", err)
	}
	if _, err := store.LatestDelivery(ctx, agent.Principal{TenantID: tenantID, RecipientID: mine}, report.ARAging); !errors.Is(err, agent.ErrNoData) {
		t.Fatalf("a report that was never sent: %v", err)
	}
}
