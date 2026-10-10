package database

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
)

func TestAClosedPeriodReadBeforeItEndedIsNotTreatedAsFinal(t *testing.T) {
	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	definition, ok := report.DefinitionFor(report.SalesGoodsServices)
	if !ok {
		t.Fatal("no sales report")
	}
	policy := report.DefaultRefreshPolicy(uuid.New())
	period := report.Period{DateFrom: "2026-10-09", DateTo: "2026-10-09"}
	now := time.Date(2026, 10, 10, 8, 0, 0, 0, bangkok)
	check := func(name string, finished time.Time, want viewer.FreshnessStatus) {
		t.Helper()
		snapshot := viewer.DashboardSnapshot{SourceFinishedAt: &finished}
		if err := finalizeSnapshot(&snapshot, report.ResultSummary, now.Add(24*time.Hour), definition, period, policy, now); err != nil {
			t.Fatal(err)
		}
		if snapshot.FreshnessStatus != want {
			t.Errorf("%s: freshness = %s, want %s", name, snapshot.FreshnessStatus, want)
		}
	}
	check("read at 17:43 on the day itself, asked next morning", time.Date(2026, 10, 9, 17, 43, 0, 0, bangkok), viewer.FreshnessStale)
	check("read at 23:59 on the day itself", time.Date(2026, 10, 9, 23, 59, 0, 0, bangkok), viewer.FreshnessStale)
	check("read after midnight, so the whole day is in it", time.Date(2026, 10, 10, 0, 5, 0, 0, bangkok), viewer.FreshnessFresh)
}

func TestAnEarlierClosedPeriodReadLaterStaysFinal(t *testing.T) {
	bangkok := time.FixedZone("Asia/Bangkok", 7*60*60)
	definition, _ := report.DefinitionFor(report.SalesGoodsServices)
	policy := report.DefaultRefreshPolicy(uuid.New())
	period := report.Period{DateFrom: "2026-09-01", DateTo: "2026-09-30"}
	now := time.Date(2026, 10, 9, 8, 0, 0, 0, bangkok)
	finished := time.Date(2026, 10, 2, 9, 0, 0, 0, bangkok)
	snapshot := viewer.DashboardSnapshot{SourceFinishedAt: &finished}
	if err := finalizeSnapshot(&snapshot, report.ResultSummary, now.Add(24*time.Hour), definition, period, policy, now); err != nil {
		t.Fatal(err)
	}
	if snapshot.FreshnessStatus != viewer.FreshnessStale {
		// historicalSnapshotTTL is a day: a month read a week ago is past it, but it must be served as stale, never expired.
		t.Errorf("freshness = %s", snapshot.FreshnessStatus)
	}
	if snapshot.StaleUntil == nil || !snapshot.StaleUntil.After(now) {
		t.Errorf("a finished month must stay readable for a long time: %v", snapshot.StaleUntil)
	}
}
