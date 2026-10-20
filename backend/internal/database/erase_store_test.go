package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type eraseFixture struct {
	ctx           context.Context
	pool          *pgxpool.Pool
	now           time.Time
	tenantID      uuid.UUID
	otherTenantID uuid.UUID
	onlyHere      uuid.UUID // belongs to the shop only
	inBoth        uuid.UUID // also belongs to another shop
	exec          func(query string, args ...any)
	count         func(query string, args ...any) int
}

func newEraseFixture(t *testing.T) *eraseFixture {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	f := &eraseFixture{ctx: ctx, pool: pool, now: time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC), tenantID: uuid.New(), otherTenantID: uuid.New(), onlyHere: uuid.New(), inBoth: uuid.New()}
	f.exec = func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	f.count = func(query string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `delete from tenants where id = any($1)`, []uuid.UUID{f.tenantID, f.otherTenantID})
		_, _ = pool.Exec(context.Background(), `delete from line_recipients where id = any($1)`, []uuid.UUID{f.onlyHere, f.inBoth})
		_, _ = pool.Exec(context.Background(), `delete from audit_logs where action in ('TENANT_ERASED', 'RECIPIENT_ERASED') and request_id = 'erase-tool' and resource_id = any($1)`, []string{f.tenantID.String(), f.onlyHere.String(), f.inBoth.String()})
	})
	for _, id := range []uuid.UUID{f.tenantID, f.otherTenantID} {
		status := "DISABLED"
		f.exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านลบ', 'Asia/Bangkok', $3, $4)`, id, "erase-"+id.String(), status, f.now.AddDate(1, 0, 0))
	}
	for _, id := range []uuid.UUID{f.onlyHere, f.inBoth} {
		f.exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, display_name_ciphertext, display_name_nonce, encryption_key_id, status, verified_at)
		        values ($1, $2, '\x01', '\x02', '\x03', '\x04', 'test', 'ACTIVE', $3)`, id, []byte("erase-"+id.String()), f.now)
		f.exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, 'ACTIVE', true)`, f.tenantID, id)
		f.exec(`insert into report_view_events (tenant_id, recipient_id, kind, occurred_at, expires_at) values ($1, $2, 'CARD_OPEN', $3, $4)`, f.tenantID, id, f.now, f.now.AddDate(1, 0, 0))
		f.exec(`insert into owner_weekly_summaries (tenant_id, recipient_id, sent_on) values ($1, $2, '2026-10-19')`, f.tenantID, id)
	}
	f.exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, 'ACTIVE', false)`, f.otherTenantID, f.inBoth)
	f.exec(`insert into audit_logs (tenant_id, actor_type, actor_id_hash, action, resource_type, resource_id, request_id, result, created_at, expires_at) values ($1, 'ADMIN', '\x01', 'TENANT_UPDATED', 'TENANT', $2, 'erase-test', 'SUCCESS', $3, $4)`, f.tenantID, f.tenantID.String(), f.now, f.now.AddDate(1, 0, 0))
	return f
}

func tableRows(counts []TableCount, table string) int64 {
	for _, item := range counts {
		if item.Table == table {
			return item.Rows
		}
	}
	return 0
}

func TestErasingAShopPlansFirstRefusesAnActiveShopAndLeavesNothingBehindExceptWhatItSaysItKeeps(t *testing.T) {
	f := newEraseFixture(t)
	store := NewEraseStore(f.pool)
	plan, err := store.PlanShop(f.ctx, f.tenantID)
	if err != nil || tableRows(plan.Tables, "report_view_events") != 2 || tableRows(plan.Tables, "tenant_memberships") != 2 || len(plan.People) != 1 || plan.People[0] != f.onlyHere {
		t.Fatalf("plan = %+v %v (one person belongs to this shop only)", plan, err)
	}
	if f.count(`select count(*) from tenants where id = $1`, f.tenantID) != 1 {
		t.Fatal("a plan must delete nothing")
	}
	f.exec(`update tenants set status = 'ACTIVE' where id = $1`, f.tenantID)
	if _, err := store.EraseShop(f.ctx, f.tenantID, true, f.now); !errors.Is(err, ErrEraseShopActive) {
		t.Fatalf("an active shop must be refused: %v", err)
	}
	f.exec(`update tenants set status = 'DISABLED' where id = $1`, f.tenantID)
	if _, err := store.EraseShop(f.ctx, uuid.New(), false, f.now); !errors.Is(err, ErrEraseNotFound) {
		t.Fatalf("an unknown id: %v", err)
	}

	result, err := store.EraseShop(f.ctx, f.tenantID, true, f.now)
	if err != nil || result.PeopleDone != 1 {
		t.Fatalf("erase = %+v %v", result, err)
	}
	if f.count(`select count(*) from tenants where id = $1`, f.tenantID) != 0 || f.count(`select count(*) from tenant_memberships where tenant_id = $1`, f.tenantID) != 0 {
		t.Fatal("the shop and its memberships must be gone")
	}
	if f.count(`select count(*) from tenants where id = $1`, f.otherTenantID) != 1 || f.count(`select count(*) from tenant_memberships where tenant_id = $1`, f.otherTenantID) != 1 {
		t.Fatal("another shop must be untouched")
	}
	// The person of this shop only has no LINE ID left; the person who also belongs to another shop keeps theirs.
	if f.count(`select count(*) from line_recipients where id = $1 and line_user_id_ciphertext is null and line_user_id_hash is null and display_name_ciphertext is null and status = 'REVOKED'`, f.onlyHere) != 1 {
		t.Fatal("the person who belonged to this shop only must be wiped")
	}
	if f.count(`select count(*) from line_recipients where id = $1 and line_user_id_ciphertext is not null and status = 'ACTIVE'`, f.inBoth) != 1 {
		t.Fatal("a person who still belongs to another shop must keep their LINE ID")
	}
	if f.count(`select count(*) from report_view_events where recipient_id = $1`, f.onlyHere) != 0 || f.count(`select count(*) from owner_weekly_summaries where recipient_id = $1`, f.onlyHere) != 0 {
		t.Fatal("what the wiped person did must be gone")
	}
	// What is kept on purpose: the audit trail of the shop (its tenant set to null) and a record of the erase with counts only.
	if f.count(`select count(*) from audit_logs where request_id = 'erase-test' and tenant_id is null and resource_id = $1`, f.tenantID.String()) != 1 {
		t.Fatal("the audit trail of the shop stays, without its tenant")
	}
	if f.count(`select count(*) from audit_logs where action = 'TENANT_ERASED' and resource_id = $1 and tenant_id is null`, f.tenantID.String()) != 1 {
		t.Fatal("the erase must be recorded")
	}
}

func TestErasingAPersonWipesTheLineIDAndWhatTheyDidButKeepsTheRowAndRefusesAnAssistantHolder(t *testing.T) {
	f := newEraseFixture(t)
	store := NewEraseStore(f.pool)
	f.exec(`insert into assistant_instances (tenant_id, instance, desired, status, recipient_id) values ($1, $2, 'RUNNING', 'RUNNING', $3)`, f.otherTenantID, "erase"+f.otherTenantID.String()[:8], f.inBoth)
	plan, err := store.PlanPerson(f.ctx, f.inBoth)
	if err != nil || plan.HoldsAssistants != 1 || tableRows(plan.Tables, "report_view_events") != 1 {
		t.Fatalf("plan = %+v %v", plan, err)
	}
	if _, err := store.ErasePerson(f.ctx, f.inBoth, false, f.now); !errors.Is(err, ErrEraseHolder) {
		t.Fatalf("the holder of an assistant must be refused without the flag: %v", err)
	}
	if f.count(`select count(*) from line_recipients where id = $1 and line_user_id_ciphertext is not null`, f.inBoth) != 1 {
		t.Fatal("a refused erase must change nothing")
	}
	if _, err := store.ErasePerson(f.ctx, f.inBoth, true, f.now); err != nil {
		t.Fatal(err)
	}
	if f.count(`select count(*) from line_recipients where id = $1 and line_user_id_ciphertext is null and display_name_ciphertext is null and status = 'REVOKED'`, f.inBoth) != 1 {
		t.Fatal("the row stays, empty")
	}
	if f.count(`select count(*) from tenant_memberships where recipient_id = $1 and status <> 'REVOKED'`, f.inBoth) != 0 || f.count(`select count(*) from report_view_events where recipient_id = $1`, f.inBoth) != 0 {
		t.Fatal("memberships must end and activity must go")
	}
	if f.count(`select count(*) from assistant_instances where recipient_id = $1`, f.inBoth) != 0 {
		t.Fatal("the assistant must have no holder left")
	}
	if f.count(`select count(*) from line_recipients where id = $1 and line_user_id_ciphertext is not null`, f.onlyHere) != 1 {
		t.Fatal("somebody else must be untouched")
	}
	if _, err := store.ErasePerson(f.ctx, uuid.New(), false, f.now); !errors.Is(err, ErrEraseNotFound) {
		t.Fatalf("an unknown id: %v", err)
	}
}
