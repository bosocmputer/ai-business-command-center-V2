package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReadsShowShopIsSlowOnlyWhenTheNewestReadIsMuchSlowerThanTheShopsOwnUsual(t *testing.T) {
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	recent := func(count int) []time.Time {
		at := make([]time.Time, count)
		for index := range at {
			at[index] = now.Add(-time.Duration(index+1) * 2 * time.Minute)
		}
		return at
	}
	cases := []struct {
		name    string
		seconds []float64
		at      []time.Time
		want    bool
	}{
		{"a fast report that suddenly takes much longer", []float64{20, 1.2, 1.0, 1.5, 0.9, 1.1}, recent(6), true},
		{"a heavy report that is always slow", []float64{110, 100, 105, 98, 103}, recent(5), false},
		{"slower but still short", []float64{6, 1, 1, 1, 1}, recent(5), false},
		{"three times the usual but under the floor", []float64{7.5, 2, 2, 2.5, 2}, recent(5), false},
		{"not enough history to tell", []float64{30, 1, 1}, recent(3), false},
		{"the slow read is old news", []float64{30, 1, 1, 1, 1}, append([]time.Time{now.Add(-30 * time.Minute)}, recent(4)...), false},
		{"mismatched input is ignored", []float64{30, 1, 1, 1, 1}, recent(2), false},
	}
	for _, test := range cases {
		if got := readsShowShopIsSlow(test.seconds, test.at, now); got != test.want {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestShopIsSlowNowReadsTheShopsOwnRecentRuns(t *testing.T) {
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
	now := time.Now().UTC()
	tenantID := uuid.New()
	defer func() { _, _ = pool.Exec(context.Background(), `delete from tenants where id = $1`, tenantID) }()
	if _, err := pool.Exec(ctx, `
		insert into tenants (id, slug, name, timezone, status, access_ends_at)
		values ($1, $2, 'Slow shop', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "slow-shop-"+tenantID.String(), now.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	insert := func(minutesAgo int, seconds float64) {
		finished := now.Add(-time.Duration(minutesAgo) * time.Minute)
		started := finished.Add(-time.Duration(seconds * float64(time.Second)))
		id := uuid.New()
		if _, err := pool.Exec(ctx, `
			insert into report_runs (id, tenant_id, report_key, source, idempotency_key, status, period_preset, period_from, period_to,
			                         queued_at, started_at, finished_at, expires_at, result_kind, priority)
			values ($1, $2, 'sales_goods_services', 'BACKGROUND', $3, 'SUCCEEDED', 'CUSTOM', '2026-10-12', '2026-10-12',
			        $4, $4, $5, $6, 'SUMMARY', 30)`, id, tenantID, "slow-"+id.String(), started, finished, now.Add(24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	store := NewReportStore(pool)
	for index, seconds := range []float64{1.0, 1.2, 0.9, 1.1, 1.0} {
		insert(30+index*5, seconds)
	}
	if slow, _ := store.shopIsSlowNow(ctx, tenantID, report.SalesGoodsServices, now); slow {
		t.Fatal("a shop answering as usual must not be held back")
	}
	insert(1, 25)
	slow, wait := store.shopIsSlowNow(ctx, tenantID, report.SalesGoodsServices, now)
	if !slow || wait != slowReadWait {
		t.Fatalf("a read 25 times slower than usual should hold refreshes back: slow=%v wait=%v", slow, wait)
	}
	if other, _ := store.shopIsSlowNow(ctx, tenantID, report.StockBalance, now); other {
		t.Fatal("another report's reads must not be judged by this one's")
	}
}
