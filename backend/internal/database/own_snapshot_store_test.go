package database

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A viewer's own finished run of a report is found again for the same period, a
// colleague's run is not, and a summary run is left to the tenant-wide lookup.
func TestOwnDetailSnapshotIsFoundForTheViewerWhoRanItAndNoOneElse(t *testing.T) {
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
	tenantID, mine, theirs := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Own snapshot', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "own-"+tenantID.String(), now.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		insert into tenant_sml_connections (
		  tenant_id, endpoint_url, database_name, username_ciphertext, username_nonce,
		  password_ciphertext, password_nonce, encryption_key_id, version, readiness_status
		) values ($1, 'https://sml.example.test', 'DATA', '\x01', '\x02', '\x03', '\x04', 'test', 1, 'READY')`, tenantID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{mine, theirs} {
		if _, err := pool.Exec(ctx, `
			insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at)
			values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, id, []byte(id.String()), now); err != nil {
			t.Fatal(err)
		}
	}
	period := report.Period{Preset: report.Custom, DateFrom: "2026-09-01", DateTo: "2026-09-30"}
	dashboard := report.Dashboard{
		ReportKey: report.CustomerRFM, Version: "1.0.0", Period: period, Timezone: "Asia/Bangkok",
		KPIs: []report.DashboardMetric{}, Visualizations: []report.DashboardVisualization{},
		Quality: report.DashboardQuality{Status: "OK", Warnings: []string{}},
	}
	dashboardJSON, _ := json.Marshal(dashboard)
	store := NewReportStore(pool)
	finish := func(recipient uuid.UUID, kind report.ResultKind, key string) uuid.UUID {
		who := recipient
		run, err := store.Enqueue(ctx, report.EnqueueInput{
			TenantID: tenantID, ReportKey: report.CustomerRFM, Source: report.SourceDashboard, ResultKind: kind,
			IdempotencyKey: key, Period: period, RequestedByRecipient: &who,
		}, now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			update report_runs set status = 'SUCCEEDED', dashboard_json = $2, dashboard_version = '1.0.0',
			  started_at = $3, source_started_at = $3, finished_at = $4, source_finished_at = $4
			where id = $1`, run.ID, dashboardJSON, now.Add(-time.Minute), now); err != nil {
			t.Fatal(err)
		}
		return run.ID
	}
	mineRun := finish(mine, report.ResultDetail, "own-detail-mine-001")
	finish(theirs, report.ResultDetail, "own-detail-theirs-001")

	got, err := store.GetOwnDetailSnapshotForPeriod(ctx, mine, tenantID, report.CustomerRFM, period, now.Add(time.Minute))
	if err != nil || got.RunID != mineRun || !got.DetailsAvailable {
		t.Fatalf("own snapshot = %+v, %v; want my run %s with its rows", got, err, mineRun)
	}
	stranger := uuid.New()
	if _, err := store.GetOwnDetailSnapshotForPeriod(ctx, stranger, tenantID, report.CustomerRFM, period, now.Add(time.Minute)); !errors.Is(err, report.ErrRunNotFound) {
		t.Fatalf("a viewer who ran nothing must not get someone else's run, err=%v", err)
	}
	other := report.Period{Preset: report.Custom, DateFrom: "2026-08-01", DateTo: "2026-08-31"}
	if _, err := store.GetOwnDetailSnapshotForPeriod(ctx, mine, tenantID, report.CustomerRFM, other, now.Add(time.Minute)); !errors.Is(err, report.ErrRunNotFound) {
		t.Fatalf("a different period must not match, err=%v", err)
	}
	// The tenant-wide lookup still ignores a viewer's DETAIL run: it is not a summary.
	if _, err := store.GetExactSnapshotForPeriod(ctx, tenantID, report.CustomerRFM, period, now.Add(time.Minute)); !errors.Is(err, report.ErrRunNotFound) {
		t.Fatalf("tenant-wide lookup must not return a detail run, err=%v", err)
	}
}
