package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/executionmode"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReportModeStoreLifecycle(t *testing.T) {
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
	tenantID, otherTenantID := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenantID, otherTenantID} {
		if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Modes', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "modes-"+id.String(), now.AddDate(1, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	store := NewReportModeStore(pool)
	admin := []byte("admin")
	audits := func(action string) (count int) {
		if err := pool.QueryRow(ctx, `select count(*) from audit_logs where tenant_id = $1 and resource_id = $2 and action = $3`, tenantID, string(report.StockBalance), action).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}

	// No row reads as DIRECT/DEFAULT.
	got, err := store.Get(ctx, tenantID, report.StockBalance)
	if err != nil || got.Mode != report.ModeDirect || got.Source != report.ModeSourceDefault {
		t.Fatalf("default = %+v, %v", got, err)
	}

	// The legacy allowlist seeds CHUNKED for known tenants only and never overwrites.
	if err := store.SeedFromTargets(ctx, []string{otherTenantID.String() + "/stock_balance", uuid.New().String() + "/stock_balance", "garbage", "/x"}, now); err != nil {
		t.Fatal(err)
	}
	if seeded, _ := store.Get(ctx, otherTenantID, report.StockBalance); seeded.Mode != report.ModeChunked || seeded.Source != report.ModeSourceEnvSeed {
		t.Fatalf("seeded = %+v", seeded)
	}
	if err := store.SeedFromTargets(ctx, []string{otherTenantID.String() + "/stock_balance"}, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// A DIRECT timeout streak counts up and a success clears it.
	for want := 1; want <= 2; want++ {
		if streak, err := store.RecordDirectTimeout(ctx, tenantID, report.StockBalance, now); err != nil || streak != want {
			t.Fatalf("streak = %d, %v; want %d", streak, err, want)
		}
	}
	if err := store.RecordDirectSuccess(ctx, tenantID, report.StockBalance, 1200, 41*time.Second, now); err != nil {
		t.Fatal(err)
	}
	if streak, _ := store.RecordDirectTimeout(ctx, tenantID, report.StockBalance, now); streak != 1 {
		t.Fatalf("streak after a success = %d, want 1", streak)
	}
	got, _ = store.Get(ctx, tenantID, report.StockBalance)
	if got.LastRows == nil || *got.LastRows != 1200 || got.LastDurationMS == nil || *got.LastDurationMS != 41000 || got.Mode != report.ModeDirect {
		t.Fatalf("measurement must be stored without changing the mode: %+v", got)
	}

	// The automatic switch happens once, is audited once, and later switches are no-ops.
	switched, err := store.SwitchToChunked(ctx, tenantID, report.StockBalance, "SML_RESULT_INVALID/XML_MALFORMED", now)
	if err != nil || !switched {
		t.Fatalf("switch = %v, %v", switched, err)
	}
	if again, err := store.SwitchToChunked(ctx, tenantID, report.StockBalance, "x", now); err != nil || again {
		t.Fatalf("second switch = %v, %v; want false", again, err)
	}
	got, _ = store.Get(ctx, tenantID, report.StockBalance)
	if got.Mode != report.ModeChunked || got.Source != report.ModeSourceAutoSwitched || got.Reason != "SML_RESULT_INVALID/XML_MALFORMED" {
		t.Fatalf("after switch = %+v", got)
	}
	if audits("REPORT_MODE_AUTO_SWITCHED") != 1 {
		t.Fatalf("auto-switch audit rows = %d, want 1", audits("REPORT_MODE_AUTO_SWITCHED"))
	}
	var actor string
	if err := pool.QueryRow(ctx, `select actor_type from audit_logs where tenant_id = $1 and action = 'REPORT_MODE_AUTO_SWITCHED'`, tenantID).Scan(&actor); err != nil || actor != "WORKER" {
		t.Fatalf("actor = %q, %v", actor, err)
	}

	// An admin can put it back; a same-mode set writes no second audit row.
	set, err := store.Set(ctx, admin, "req-1", tenantID, report.StockBalance, report.ModeDirect, report.ModeSourceManual, "ร้านเล็กลง", now)
	if err != nil || set.Mode != report.ModeDirect || set.Source != report.ModeSourceManual || set.Reason != "ร้านเล็กลง" {
		t.Fatalf("set = %+v, %v", set, err)
	}
	if _, err := store.Set(ctx, admin, "req-2", tenantID, report.StockBalance, report.ModeDirect, report.ModeSourceManual, "again", now); err != nil {
		t.Fatal(err)
	}
	if audits("REPORT_MODE_SET") != 1 {
		t.Fatalf("set audit rows = %d, want 1", audits("REPORT_MODE_SET"))
	}

	// A first-time measurement is audited under its own action.
	if _, err := store.Set(ctx, admin, "req-3", tenantID, report.ARCustomerMovement, report.ModeChunked, report.ModeSourceMeasured, "measured 5000 units", now); err != nil {
		t.Fatal(err)
	}
	var measuredAudits int
	if err := pool.QueryRow(ctx, `select count(*) from audit_logs where tenant_id = $1 and action = 'REPORT_MODE_MEASURED'`, tenantID).Scan(&measuredAudits); err != nil || measuredAudits != 1 {
		t.Fatalf("measured audits = %d, %v", measuredAudits, err)
	}

	// Refusals and boundaries.
	if _, err := store.Set(ctx, admin, "r", tenantID, report.SalesGoodsServices, report.ModeChunked, report.ModeSourceManual, "", now); !errors.Is(err, ErrExecutionModeUnsupported) {
		t.Fatalf("chunked for an unchunkable report: %v", err)
	}
	if _, err := store.Set(ctx, admin, "r", uuid.New(), report.StockBalance, report.ModeChunked, report.ModeSourceManual, "", now); !errors.Is(err, executionmode.ErrTenantNotFound) {
		t.Fatalf("unknown tenant: %v", err)
	}
	if list, err := store.List(ctx, tenantID); err != nil || len(list) != 2 {
		t.Fatalf("list = %d rows, %v; want stock and AR", len(list), err)
	}
	if busy, err := store.TenantBusy(ctx, tenantID); err != nil || busy {
		t.Fatalf("busy = %v, %v", busy, err)
	}
}
