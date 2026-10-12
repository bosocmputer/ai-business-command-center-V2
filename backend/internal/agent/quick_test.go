package agent

import (
	"context"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

func TestQuickYesterdayReadsYesterdaysSalesForTheirTenant(t *testing.T) {
	period := report.Period{DateFrom: "2026-09-30", DateTo: "2026-09-30"}
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{snapshotKey(report.SalesGoodsServices, period): fresh(salesDashboard(period, "101310.00"))}}
	result, err := newService(store, snapshots).QuickReport(context.Background(), tenantID, uuid.New(), QuickYesterday)
	if err != nil || result.Status != QuickReady || result.Key != report.SalesGoodsServices || result.Period.DateFrom != "2026-09-30" || result.Dashboard.KPIs[0].Value != "101310.00" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if snapshots.refreshed != 0 {
		t.Fatal("yesterday is read from the stored figures, not read again")
	}
	if len(store.calls) != 0 {
		t.Fatal("a menu card is not a token call and must not be logged as one")
	}
}

func TestQuickIsRefusedForAReportThePersonMayNotSee(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.StockBalance}}
	result, err := newService(store, &fakeSnapshots{}).QuickReport(context.Background(), tenantID, uuid.New(), QuickYesterday)
	if err != nil || result.Status != QuickNotAllowed || result.Message != MessageQuickNotAllowed {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestQuickTodayAsksForALiveReadAndHasNoComparisonWithAWholeDay(t *testing.T) {
	period := report.Period{DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	old := fresh(salesDashboard(period, "500.00")) // read 5 minutes ago: older than the minute that counts as the latest
	snapshots := &fakeSnapshots{
		exact:        map[string]viewer.DashboardSnapshot{snapshotKey(report.SalesGoodsServices, period): old},
		revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationFreshCache, Snapshot: &old},
	}
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	result, err := newService(store, snapshots).QuickReport(context.Background(), tenantID, uuid.New(), QuickToday)
	if err != nil || result.Status != QuickReady {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if snapshots.refreshed != 1 {
		t.Fatalf("the button for the latest figures must read again, refreshed = %d", snapshots.refreshed)
	}
	for _, metric := range result.Dashboard.KPIs {
		if metric.Comparison.Availability != report.ComparisonUnavailable {
			t.Fatalf("today must not be compared with a whole day: %+v", metric)
		}
	}
	if result.Dashboard.ComparisonPeriod.DateFrom != "" {
		t.Fatalf("comparison period should be cleared, got %+v", result.Dashboard.ComparisonPeriod)
	}
}

func TestQuickReorderReadsTheCurrentStockReport(t *testing.T) {
	period := report.Period{DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	dashboard := report.Dashboard{ReportKey: report.StockReorder, Period: period, KPIs: []report.DashboardMetric{{Key: "reorder_item_count", Value: "2", Unit: report.UnitCount}}}
	store := &fakeStore{permitted: []report.Key{report.StockReorder}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{snapshotKey(report.StockReorder, period): fresh(dashboard)}}
	result, err := newService(store, snapshots).QuickReport(context.Background(), tenantID, uuid.New(), QuickReorder)
	if err != nil || result.Status != QuickReady || result.Key != report.StockReorder {
		t.Fatalf("result = %+v, %v", result, err)
	}
}

func TestQuickAnswersPlainlyWhenFiguresAreNotThereYet(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	snapshots := &fakeSnapshots{revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationMissingRefreshing}}
	result, err := newServiceWith(store, snapshots, Config{StaleWait: -1}).QuickReport(context.Background(), tenantID, uuid.New(), QuickYesterday)
	if err != nil || result.Status != QuickPreparing || result.Message != MessageQuickPreparing {
		t.Fatalf("result = %+v, %v", result, err)
	}
}
