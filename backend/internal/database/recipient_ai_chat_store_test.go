package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/recipient"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRecipientAIChatPermissionLifecycle(t *testing.T) {
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

	now := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	tenantID, otherTenantID, recipientID := uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenantID, otherTenantID} {
		if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'AI chat', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "aichat-"+id.String(), now.AddDate(1, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, display_name_ciphertext, display_name_nonce, encryption_key_id, status, verified_at)
		values ($1, $2, '\x01', '\x02', '\x03', '\x04', 'test', 'ACTIVE', $3)`, recipientID, []byte("aichat-"+recipientID.String()), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into tenant_memberships (tenant_id, recipient_id, status) values ($1, $2, 'ACTIVE')`, tenantID, recipientID); err != nil {
		t.Fatal(err)
	}
	store := NewRecipientStore(pool)
	actor := []byte("admin")

	audits := func() (updates int, latestBefore, latestAfter string) {
		if err := pool.QueryRow(ctx, `
			select count(*) from audit_logs
			where tenant_id = $1 and resource_id = $2 and action = 'RECIPIENT_AI_CHAT_UPDATED'`, tenantID, recipientID.String()).Scan(&updates); err != nil {
			t.Fatal(err)
		}
		_ = pool.QueryRow(ctx, `
			select coalesce(before_json::text, ''), coalesce(after_json::text, '') from audit_logs
			where tenant_id = $1 and resource_id = $2 and action = 'RECIPIENT_AI_CHAT_UPDATED'
			order by created_at desc, id desc limit 1`, tenantID, recipientID.String()).Scan(&latestBefore, &latestAfter)
		return updates, latestBefore, latestAfter
	}

	// A recipient starts without assistant access, in the single and list views.
	got, err := store.GetForTenant(ctx, tenantID, recipientID)
	if err != nil || got.AIChatEnabled {
		t.Fatalf("new recipient = %+v, %v; want AI chat off", got, err)
	}
	page, err := store.List(ctx, tenantID, 10, "")
	if err != nil || len(page.Stored) != 1 || page.Stored[0].AIChatEnabled {
		t.Fatalf("list = %+v, %v; want one recipient with AI chat off", page, err)
	}

	enabled, err := store.SetAIChat(ctx, actor, "req-1", tenantID, recipientID, true, now)
	if err != nil || !enabled.AIChatEnabled {
		t.Fatalf("enable = %+v, %v", enabled, err)
	}
	if count, before, after := audits(); count != 1 || before != `{"aiChatEnabled": false}` || after != `{"aiChatEnabled": true}` {
		t.Fatalf("audit after enable: count=%d before=%s after=%s", count, before, after)
	}
	if page, err = store.List(ctx, tenantID, 10, ""); err != nil || !page.Stored[0].AIChatEnabled {
		t.Fatalf("list after enable = %+v, %v", page, err)
	}

	// Setting the same value again must not create audit noise or bump anything.
	if _, err := store.SetAIChat(ctx, actor, "req-2", tenantID, recipientID, true, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if count, _, _ := audits(); count != 1 {
		t.Fatalf("repeat enable wrote %d audit rows, want 1", count)
	}

	// The switch is independent of report permissions in both directions.
	replaced, err := store.ReplacePermissions(ctx, actor, "req-3", tenantID, recipientID, []report.Key{report.SalesGoodsServices}, got.PermissionsVersion, now.Add(2*time.Minute))
	if err != nil || !replaced.AIChatEnabled || replaced.PermissionsVersion != got.PermissionsVersion+1 {
		t.Fatalf("replace permissions = %+v, %v; AI chat must survive and version advance by one", replaced, err)
	}
	disabled, err := store.SetAIChat(ctx, actor, "req-4", tenantID, recipientID, false, now.Add(3*time.Minute))
	if err != nil || disabled.AIChatEnabled || disabled.PermissionsVersion != replaced.PermissionsVersion || len(disabled.ReportKeys) != 1 {
		t.Fatalf("disable = %+v, %v; report permissions and version must be untouched", disabled, err)
	}
	if count, before, after := audits(); count != 2 || before != `{"aiChatEnabled": true}` || after != `{"aiChatEnabled": false}` {
		t.Fatalf("audit after disable: count=%d before=%s after=%s", count, before, after)
	}

	// Scoped to its own tenant, and gone once the membership is revoked.
	if _, err := store.SetAIChat(ctx, actor, "req-5", otherTenantID, recipientID, true, now); !errors.Is(err, recipient.ErrRecipientNotFound) {
		t.Fatalf("other tenant error = %v", err)
	}
	if _, err := store.SetAIChat(ctx, actor, "req-6", tenantID, uuid.New(), true, now); !errors.Is(err, recipient.ErrRecipientNotFound) {
		t.Fatalf("unknown recipient error = %v", err)
	}
	if _, err := pool.Exec(ctx, `update tenant_memberships set status = 'REVOKED' where tenant_id = $1 and recipient_id = $2`, tenantID, recipientID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetAIChat(ctx, actor, "req-7", tenantID, recipientID, true, now); !errors.Is(err, recipient.ErrRecipientNotFound) {
		t.Fatalf("revoked membership error = %v", err)
	}
}
