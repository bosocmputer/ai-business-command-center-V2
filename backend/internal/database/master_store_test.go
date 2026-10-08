package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/master"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMasterCopyTargetsReplaceFailureAndSearch(t *testing.T) {
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
	morning := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC) // 07:00 in Bangkok
	tenantID, otherTenant, recipientID := uuid.New(), uuid.New(), uuid.New()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []uuid.UUID{tenantID, otherTenant} {
		exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านทดสอบ', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "master-"+id.String(), morning.AddDate(1, 0, 0))
	}
	exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, recipientID, []byte(recipientID.String()), morning)
	exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, 'ACTIVE', true)`, tenantID, recipientID)
	agents := NewAgentStore(pool)
	if _, err := agents.IssueToken(ctx, []byte("admin"), "r1", tenantID, recipientID, true, hashOf("owner"), morning.Add(90*24*time.Hour), morning); err != nil {
		t.Fatal(err)
	}
	store := NewMasterStore(pool)

	// A shop is due only when its SML works and its assistant is on.
	if targets, err := store.Targets(ctx, morning); err != nil || len(targets) != 0 {
		t.Fatalf("no SML connection yet: %+v %v", targets, err)
	}
	for _, id := range []uuid.UUID{tenantID, otherTenant} {
		exec(`insert into tenant_sml_connections (tenant_id, endpoint_url, database_name, username_ciphertext, username_nonce, password_ciphertext, password_nonce, encryption_key_id, readiness_status) values ($1, 'http://sml.test', 'db', '\x01', '\x02', '\x03', '\x04', 'test', 'READY')`, id)
	}
	targets, err := store.Targets(ctx, morning)
	if err != nil || len(targets) != 1 || targets[0].TenantID != tenantID || len(targets[0].Due) != 3 {
		t.Fatalf("only the shop with the assistant on is due, for all three kinds: %+v %v", targets, err)
	}

	customers := []master.Record{
		{Code: "C001", Name: "บริษัท ก่อสร้างดี จำกัด", Phone: "081-234-5678", Active: true},
		{Code: "C002", Name: "ก่อสร้างเยี่ยม", Phone: "02-555-0000", Active: true},
		{Code: "C003", Name: "ก่อสร้างเก่า", Active: false},
	}
	if err := store.Replace(ctx, tenantID, master.KindCustomer, customers, morning); err != nil {
		t.Fatal(err)
	}
	if err := store.Replace(ctx, otherTenant, master.KindCustomer, []master.Record{{Code: "X1", Name: "ก่อสร้างของร้านอื่น", Active: true}}, morning); err != nil {
		t.Fatal(err)
	}
	// Customers are copied for today; the other kinds are still due, and nothing is due again for the kind just copied.
	targets, _ = store.Targets(ctx, morning.Add(time.Hour))
	if len(targets) != 1 || len(targets[0].Due) != 2 {
		t.Fatalf("after copying customers: %+v", targets)
	}

	search := func(tenant uuid.UUID, kind agent.MasterKind, words ...string) agent.MasterResult {
		t.Helper()
		result, err := store.SearchMaster(ctx, tenant, kind, words, 10)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := search(tenantID, agent.MasterCustomer, "ก่อสร้าง"); got.Total != 2 || len(got.Matches) != 2 || got.SyncedAt == nil {
		t.Fatalf("a word in the name finds the active records of this shop only: %+v", got)
	}
	if got := search(tenantID, agent.MasterCustomer, "ก่อสร้าง", "เยี่ยม"); got.Total != 1 || got.Matches[0].Code != "C002" {
		t.Fatalf("every word must match: %+v", got)
	}
	if got := search(tenantID, agent.MasterCustomer, "0812345678"); got.Total != 1 || got.Matches[0].Code != "C001" {
		t.Fatalf("a phone number matches whatever way it is punctuated: %+v", got)
	}
	if got := search(tenantID, agent.MasterCustomer, "c002"); got.Total != 1 || got.Matches[0].Name != "ก่อสร้างเยี่ยม" {
		t.Fatalf("a code matches without regard to case: %+v", got)
	}
	if got := search(tenantID, agent.MasterSupplier, "ก่อสร้าง"); got.SyncedAt != nil || got.Total != 0 {
		t.Fatalf("a kind that was never copied reports no copy: %+v", got)
	}

	// A later copy removes what is gone from SML, keeps what stays, and an empty answer never wipes the copy.
	later := morning.Add(24 * time.Hour)
	if err := store.Replace(ctx, tenantID, master.KindCustomer, customers[:1], later); err != nil {
		t.Fatal(err)
	}
	if got := search(tenantID, agent.MasterCustomer, "ก่อสร้าง"); got.Total != 1 || got.Matches[0].Code != "C001" {
		t.Fatalf("rows no longer in SML must go: %+v", got)
	}
	if err := store.Replace(ctx, tenantID, master.KindCustomer, nil, later.Add(time.Hour)); !errors.Is(err, master.ErrEmpty) {
		t.Fatalf("an empty result must not replace an existing copy: %v", err)
	}
	if got := search(tenantID, agent.MasterCustomer, "ก่อสร้าง"); got.Total != 1 {
		t.Fatalf("the copy must survive an empty answer: %+v", got)
	}
	// A failed try is recorded, the good copy and its time stay, and the next try waits half an hour.
	if err := store.MarkFailed(ctx, tenantID, master.KindCustomer, "SML_TIMEOUT", later.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var status, code string
	var syncedAt *time.Time
	if err := pool.QueryRow(ctx, `select status, coalesce(error_code, ''), synced_at from master_sync where tenant_id = $1 and kind = 'CUSTOMER'`, tenantID).Scan(&status, &code, &syncedAt); err != nil {
		t.Fatal(err)
	}
	if status != "ERROR" || code != "SML_TIMEOUT" || syncedAt == nil || !syncedAt.Equal(later) {
		t.Fatalf("state = %s %s %v", status, code, syncedAt)
	}
	targets, _ = store.Targets(ctx, later.Add(2*time.Hour+10*time.Minute))
	for _, target := range targets {
		for _, kind := range target.Due {
			if kind == master.KindCustomer && target.TenantID == tenantID {
				// copied on the later day already, so only a new day makes it due
				t.Fatalf("a kind copied today and failed after must not be due the same day: %+v", target)
			}
		}
	}
	// Items keep their unit and supplier code and are searched without any names permission at this layer.
	if err := store.Replace(ctx, tenantID, master.KindItem, []master.Record{{Code: "A01", Name: "ปูนซีเมนต์ถุง", Unit: "ถุง", SupplierCode: "S1", Active: true}}, later); err != nil {
		t.Fatal(err)
	}
	if got := search(tenantID, agent.MasterItem, strings.ToLower("ปูน")); got.Total != 1 || got.Matches[0].Unit != "ถุง" || got.Matches[0].SupplierCode != "S1" {
		t.Fatalf("item = %+v", got)
	}
}
