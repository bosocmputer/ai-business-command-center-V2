package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
)

func reorderDashboard(count string, items ...[]string) report.Dashboard { // item: code, name, unit, short, balance, point, onOrder
	period := report.Period{Preset: report.AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	dashboard := report.Dashboard{ReportKey: report.StockReorder, Period: period, Quality: report.DashboardQuality{Status: "OK"},
		KPIs: []report.DashboardMetric{{Key: "reorder_item_count", Value: count, Unit: report.UnitCount}}}
	if len(items) == 0 {
		return dashboard
	}
	visualization := report.DashboardVisualization{Key: "agent_reorder_items", Intent: report.IntentRanking, Unit: report.UnitQuantity,
		Series: []report.VisualizationSeries{{Key: "shortage_qty"}, {Key: "balance_qty"}, {Key: "purchase_point"}, {Key: "on_order_qty"}}}
	for _, item := range items {
		visualization.Categories = append(visualization.Categories, item[0])
		visualization.Series[0].Values = append(visualization.Series[0].Values, item[3])
		visualization.Series[0].PointLabels = append(visualization.Series[0].PointLabels, item[1])
		visualization.Series[1].Values = append(visualization.Series[1].Values, item[4])
		visualization.Series[1].PointLabels = append(visualization.Series[1].PointLabels, item[2])
		visualization.Series[2].Values = append(visualization.Series[2].Values, item[5])
		visualization.Series[3].Values = append(visualization.Series[3].Values, item[6])
	}
	dashboard.Visualizations = []report.DashboardVisualization{visualization}
	return dashboard
}

func reorderService(dashboard report.Dashboard, permitted ...report.Key) (*Service, *fakeStore) {
	store := &fakeStore{permitted: permitted}
	period := report.Period{Preset: report.AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{snapshotKey(report.StockReorder, period): fresh(dashboard)}}
	return newService(store, snapshots), store
}

func TestAPurchaseDraftListsTheItemsBelowTheirReorderPoint(t *testing.T) {
	dashboard := reorderDashboard("3",
		[]string{"A01", "ปูนซีเมนต์ถุง", "ถุง", "1250.0000", "-50.0000", "1200.0000", "300.0000"},
		[]string{"B02", "เหล็กเส้น 12 มม.", "เส้น", "12.5000", "2.5000", "15.0000", "0.0000"})
	service, store := reorderService(dashboard, report.StockReorder)
	got, err := service.DraftPurchaseOrder(context.Background(), principal)
	if err != nil || got.Status != "READY" || got.ItemsListed != 2 || got.ItemsBelowPoint != 3 || len(got.Items) != 2 {
		t.Fatalf("draft = %+v %v", got, err)
	}
	for _, want := range []string{"ขอสั่งซื้อสินค้า จาก ร้านทดสอบ", "1 ต.ค. 2569", "เรียน ผู้จำหน่าย", "1. ปูนซีเมนต์ถุง (รหัส A01) จำนวน 1,250 ถุง (มียอดค้างรับจากใบสั่งซื้อเดิม 300 ถุง)", "2. เหล็กเส้น 12 มม. (รหัส B02) จำนวน 12.5 เส้น\n", "ร้านทดสอบ"} {
		if !strings.Contains(got.Draft, want) {
			t.Errorf("draft lacks %q:\n%s", want, got.Draft)
		}
	}
	notes := strings.Join(got.Notes, " | ")
	for _, want := range []string{"ยังไม่ได้ส่งให้ใคร", "ไม่มีข้อมูลผู้จำหน่าย", "ไม่ใช่จำนวนที่แนะนำให้สั่ง", "ค้างรับ", "มีสินค้าถึงจุดสั่งซื้อ 3 รายการ ร่างนี้แสดง 2"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q: %s", want, notes)
		}
	}
	if last := store.calls[len(store.calls)-1]; last.Tool != ToolDraft || last.ReportKey != "stock_reorder" || last.Outcome != OutcomeOK {
		t.Errorf("call log = %+v", last)
	}
}

func TestAPurchaseDraftSaysWhenNothingIsBelowItsPoint(t *testing.T) {
	service, _ := reorderService(reorderDashboard("0"), report.StockReorder)
	got, err := service.DraftPurchaseOrder(context.Background(), principal)
	if err != nil || got.Status != "NOTHING_TO_ORDER" || got.Draft != "" || got.Message != MessageNothingToOrder {
		t.Fatalf("got = %+v %v", got, err)
	}
	// The report counts items but this snapshot has no list (made before the list existed): never invent one.
	older, _ := reorderService(reorderDashboard("4"), report.StockReorder)
	got, err = older.DraftPurchaseOrder(context.Background(), principal)
	if err != nil || got.Status != "UNAVAILABLE" || got.Draft != "" {
		t.Fatalf("older snapshot = %+v %v", got, err)
	}
}

func TestAPurchaseDraftNeedsThePermissionAndWaitsForTheReport(t *testing.T) {
	service, store := reorderService(reorderDashboard("1", []string{"A01", "ปูน", "ถุง", "5", "1", "6", "0"}), report.SalesGoodsServices)
	if _, err := service.DraftPurchaseOrder(context.Background(), principal); !errors.Is(err, ErrNoData) {
		t.Fatalf("without the reorder report it answers like a missing one: %v", err)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeNoData {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
	waiting := newService(&fakeStore{permitted: []report.Key{report.StockReorder}}, &fakeSnapshots{revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationMissingRefreshing, RetryAfter: 60}})
	got, err := waiting.DraftPurchaseOrder(context.Background(), principal)
	if err != nil || got.Status != "PREPARING" || got.Draft != "" || got.RetryAfterSeconds < 60 {
		t.Fatalf("preparing = %+v %v", got, err)
	}
}

func TestTheReorderListIsNotReturnedByTheReportTool(t *testing.T) {
	service, _ := reorderService(reorderDashboard("1", []string{"A01", "ปูน", "ถุง", "5", "1", "6", "0"}), report.StockReorder)
	got, err := service.Report(context.Background(), principal, "stock_reorder", "", "")
	if err != nil || got.Status != "READY" {
		t.Fatalf("report = %+v %v", got, err)
	}
	for _, visualization := range got.Visualizations {
		if strings.HasPrefix(visualization.Key, "agent_") {
			t.Errorf("%s is for the draft tool only", visualization.Key)
		}
	}
}
