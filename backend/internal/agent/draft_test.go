package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
)

var namedPrincipal = func() Principal { named := principal; named.NamesVisible = true; return named }()

func agingDashboard() report.Dashboard {
	period := report.Period{Preset: report.AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	visualization := func(key string, unit report.MetricUnit, values ...string) report.DashboardVisualization {
		return report.DashboardVisualization{Key: key, Intent: report.IntentRanking, Unit: unit,
			Categories: []string{"บริษัท ก่อสร้างดี จำกัด", "ห้างหุ้นส่วน ปูนทอง", "บริษัท ก่อสร้างเยี่ยม จำกัด"}, Series: []report.VisualizationSeries{{Key: "value", Values: values}}}
	}
	return report.Dashboard{ReportKey: report.ARAging, Period: period, Quality: report.DashboardQuality{Status: "OK"},
		Visualizations: []report.DashboardVisualization{
			visualization("overdue_debtors", report.UnitTHB, "641200.50", "52000.00", "1000.00"),
			visualization("overdue_debtor_days", report.UnitCount, "95", "12", "3"),
			{Key: "stale_overdue_debtors", Intent: report.IntentRanking, Unit: report.UnitTHB, Categories: []string{"บริษัท หนี้เก่า จำกัด"}, Series: []report.VisualizationSeries{{Key: "value", Values: []string{"108650.00"}}}},
			{Key: "stale_overdue_debtor_days", Intent: report.IntentRanking, Unit: report.UnitCount, Categories: []string{"บริษัท หนี้เก่า จำกัด"}, Series: []report.VisualizationSeries{{Key: "value", Values: []string{"2498"}}}},
			{Key: "agent_overdue_documents", Intent: report.IntentRanking, Unit: report.UnitTHB, Categories: []string{"INV-7", "INV-9", "INV-3"}, Series: []report.VisualizationSeries{
				{Key: "value", Values: []string{"600000.00", "30000.50", "30000.00"}, PointLabels: []string{"บริษัท ก่อสร้างดี จำกัด", "บริษัท ก่อสร้างดี จำกัด", "ห้างหุ้นส่วน ปูนทอง"}},
				{Key: "days_past_due", Values: []string{"95", "20", "12"}, PointLabels: []string{"2026-07-05", "2026-09-11", "2026-09-19"}},
			}},
		}}
}

func draftService(dashboard report.Dashboard, permitted ...report.Key) (*Service, *fakeStore) {
	store := &fakeStore{permitted: permitted}
	period := report.Period{Preset: report.AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{snapshotKey(report.ARAging, period): fresh(dashboard)}}
	return newService(store, snapshots), store
}

func TestACollectionDraftIsWrittenFromTheReportsOwnFigures(t *testing.T) {
	service, store := draftService(agingDashboard(), report.ARAging)
	got, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "  ปูนทอง ", Tone: "friendly"})
	if err != nil || got.Status != "READY" || got.Customer != "ห้างหุ้นส่วน ปูนทอง" || got.OverdueAmount != "52000.00" || got.MaxDaysPastDue != 12 {
		t.Fatalf("draft = %+v %v", got, err)
	}
	for _, want := range []string{"เรียน ห้างหุ้นส่วน ปูนทอง", "ร้านทดสอบ", "1 ต.ค. 2569", "52,000.00 บาท", "เลยมาแล้ว 12 วัน", "ขอบคุณ"} {
		if !strings.Contains(got.Draft, want) {
			t.Errorf("draft lacks %q:\n%s", want, got.Draft)
		}
	}
	for _, forbidden := range []string{"ค่าปรับ", "ดำเนินคดี", "ภายใน", "ฟ้อง"} {
		if strings.Contains(got.Draft, forbidden) {
			t.Errorf("a reminder sets no deadline and threatens nothing, but has %q:\n%s", forbidden, got.Draft)
		}
	}
	if len(got.Notes) == 0 || !strings.Contains(got.Notes[0], "ยังไม่ได้ส่งให้ใคร") || got.CollectedAt == "" || got.Freshness != "FRESH" {
		t.Errorf("the draft must say it is unsent and when the data was read: %+v", got)
	}
	last := store.calls[len(store.calls)-1]
	if last.Tool != ToolDraft || last.Outcome != OutcomeOK || last.ReportKey != "ar_aging" {
		t.Errorf("the call log must show a draft that worked: %+v", last)
	}
}

func TestTheFormalToneAndTheDefaultToneDiffer(t *testing.T) {
	service, _ := draftService(agingDashboard(), report.ARAging)
	friendly, _ := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง"})
	formal, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง", Tone: "formal"})
	if err != nil || friendly.Tone != "friendly" || formal.Tone != "formal" || !strings.Contains(formal.Draft, "ขอแสดงความนับถือ") || strings.Contains(friendly.Draft, "ขอแสดงความนับถือ") {
		t.Fatalf("friendly = %q formal = %q %v", friendly.Draft, formal.Draft, err)
	}
}

func TestADraftNeedsOneCustomerAndListsTheChoicesWhenItCannotDecide(t *testing.T) {
	service, store := draftService(agingDashboard(), report.ARAging)
	ambiguous, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ก่อสร้าง", Tone: "friendly"})
	if err != nil || ambiguous.Status != "AMBIGUOUS" || ambiguous.Draft != "" || len(ambiguous.Candidates) != 2 {
		t.Fatalf("two customers match, so no draft: %+v %v", ambiguous, err)
	}
	missing, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ไม่มีชื่อนี้", Tone: "friendly"})
	if err != nil || missing.Status != "NOT_FOUND" || missing.Draft != "" || len(missing.Candidates) != 3 || !strings.Contains(missing.Message, "10 รายแรก") {
		t.Fatalf("not in the top list: %+v %v", missing, err)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeNoData {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
}

func TestADraftRefusesBadInputWithoutTouchingTheReport(t *testing.T) {
	service, store := draftService(agingDashboard(), report.ARAging)
	for _, request := range []DraftRequest{{Customer: ""}, {Customer: "ก"}, {Customer: strings.Repeat("ก", 101)}, {Customer: "ปูนทอง", Tone: "angry"}} {
		_, err := service.DraftCollection(context.Background(), namedPrincipal, request)
		var invalid *InvalidDraftError
		if !errors.As(err, &invalid) || invalid.Message == "" {
			t.Errorf("%+v: err = %v", request, err)
		}
	}
	if last := store.calls[len(store.calls)-1]; last.Outcome != OutcomeInvalidDraft {
		t.Errorf("outcome = %s", last.Outcome)
	}
}

func TestADraftNeedsNamesAndThePermissionToReadReceivables(t *testing.T) {
	service, store := draftService(agingDashboard(), report.ARAging)
	masked, err := service.DraftCollection(context.Background(), principal, DraftRequest{Customer: "ปูนทอง", Tone: "friendly"}) // names hidden
	if err != nil || masked.Status != "UNAVAILABLE" || masked.Draft != "" || masked.Message != MessageDraftNeedsNames {
		t.Fatalf("masked = %+v %v", masked, err)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeUnavailable {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
	other, otherStore := draftService(agingDashboard(), report.SalesGoodsServices)
	if _, err := other.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง", Tone: "friendly"}); !errors.Is(err, ErrNoData) {
		t.Fatalf("without the receivable report it answers like a missing one: %v", err)
	}
	if otherStore.calls[len(otherStore.calls)-1].Outcome != OutcomeNoData {
		t.Errorf("outcome = %s", otherStore.calls[len(otherStore.calls)-1].Outcome)
	}
}

func TestADraftWaitsForAReportThatIsStillBeingFetched(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.ARAging}}
	snapshots := &fakeSnapshots{revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationMissingRefreshing, RetryAfter: 60}}
	service := newService(store, snapshots)
	got, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง", Tone: "friendly"})
	if err != nil || got.Status != "PREPARING" || got.Draft != "" || got.RetryAfterSeconds < 60 {
		t.Fatalf("preparing = %+v %v", got, err)
	}
}

func TestADraftFromAReportWithoutTheOverdueChartsSaysNotFoundRatherThanInventing(t *testing.T) {
	old := agingDashboard()
	old.Visualizations = nil // a snapshot made before the charts existed
	service, _ := draftService(old, report.ARAging)
	got, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง", Tone: "friendly"})
	if err != nil || got.Status != "NOT_FOUND" || got.Draft != "" {
		t.Fatalf("got = %+v %v", got, err)
	}
}

func TestTheDraftListsTheDocumentsAndStatesWhatIsLeftAsAnAmount(t *testing.T) {
	service, _ := draftService(agingDashboard(), report.ARAging)
	got, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ก่อสร้างดี", Tone: "friendly"})
	if err != nil || got.Status != "READY" {
		t.Fatalf("draft = %+v %v", got, err)
	}
	for _, want := range []string{"เอกสารเลขที่ INV-7 ครบกำหนด 5 ก.ค. 2569 ยอดค้าง 600,000.00 บาท", "เอกสารเลขที่ INV-9 ครบกำหนด 11 ก.ย. 2569 ยอดค้าง 30,000.50 บาท", "และรายการอื่นอีกรวม 11,200.00 บาท"} {
		if !strings.Contains(got.Draft, want) {
			t.Errorf("draft lacks %q:\n%s", want, got.Draft)
		}
	}
	if strings.Contains(got.Draft, "INV-3") {
		t.Errorf("another customer's document must never appear:\n%s", got.Draft)
	}
	// ปูนทอง's whole overdue amount is 52,000.00 but only 30,000.00 is listed, so the rest is stated as an amount.
	other, _ := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง", Tone: "formal"})
	if !strings.Contains(other.Draft, "INV-3") || !strings.Contains(other.Draft, "และรายการอื่นอีกรวม 22,000.00 บาท") {
		t.Errorf("formal draft:\n%s", other.Draft)
	}
}

func TestTheAssistantsReportToolDoesNotReturnTheDocumentList(t *testing.T) {
	service, _ := draftService(agingDashboard(), report.ARAging)
	got, err := service.Report(context.Background(), namedPrincipal, "ar_aging", "", "")
	if err != nil || got.Status != "READY" {
		t.Fatalf("report = %+v %v", got, err)
	}
	for _, visualization := range got.Visualizations {
		if strings.HasPrefix(visualization.Key, "agent_") {
			t.Errorf("%s is for the draft tool only", visualization.Key)
		}
	}
}

func TestAnOldDebtGetsNoReminderAndIsSentToTheAccountantInstead(t *testing.T) {
	service, store := draftService(agingDashboard(), report.ARAging)
	got, err := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "หนี้เก่า", Tone: "friendly"})
	if err != nil || got.Status != "NOT_CHASEABLE" || got.Draft != "" || got.Customer != "บริษัท หนี้เก่า จำกัด" || got.OverdueAmount != "108650.00" || got.MaxDaysPastDue != 2498 ||
		!strings.Contains(got.Message, "365 วัน") || !strings.Contains(got.Message, "ฝ่ายบัญชี") {
		t.Fatalf("old debt = %+v %v", got, err)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeNoData {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
	ready, _ := service.DraftCollection(context.Background(), namedPrincipal, DraftRequest{Customer: "ปูนทอง", Tone: "friendly"})
	joined := strings.Join(ready.Notes, " ")
	if ready.Status != "READY" || !strings.Contains(joined, "ไม่เกิน 365 วัน") {
		t.Errorf("a reminder must say it counts only debts up to a year past due: %+v", ready)
	}
}
