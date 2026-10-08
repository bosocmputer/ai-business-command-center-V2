package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

type fakeFacts struct {
	facts Facts
	err   error
}

func (fake fakeFacts) Facts(context.Context, uuid.UUID, time.Time) (Facts, error) {
	return fake.facts, fake.err
}

type fakeSQL struct {
	failReport string // a statement containing this text fails
	statements int
	answers    map[string][]map[string]string
}

func (fake *fakeSQL) Query(_ context.Context, statement string) ([]map[string]string, error) {
	fake.statements++
	if fake.failReport != "" && strings.Contains(statement, fake.failReport) {
		return nil, errors.New("SML_QUERY_FAILED")
	}
	for needle, rows := range fake.answers {
		if strings.Contains(statement, needle) {
			return rows, nil
		}
	}
	return nil, nil
}

var now = time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)

func readyFacts() Facts {
	tested := now.Add(-2 * time.Hour)
	return Facts{Found: true, Active: true, AccessEndsAt: now.AddDate(1, 0, 0), Timezone: "Asia/Bangkok", SMLReadiness: "READY", SMLTestedAt: &tested,
		ActiveRecipients: 2, VerifiedRecipients: 2, ActiveSchedules: 1, AssistantTokens: 1, AssistantMembers: 2,
		MasterCopies: map[string]MasterState{"CUSTOMER": {Rows: 5301, SyncedAt: &tested, Status: "OK"}}}
}

func find(result Result, key string) (Item, bool) {
	for _, item := range result.Items {
		if item.Key == key {
			return item, true
		}
	}
	return Item{}, false
}

func run(facts Facts, sql *fakeSQL) Result {
	return NewChecker(fakeFacts{facts: facts}, sql, func() time.Time { return now }).Run(context.Background(), uuid.New())
}

func TestAShopWhoseSMLIsNotReadyStopsAtThatAndSaysWhy(t *testing.T) {
	facts := readyFacts()
	facts.SMLReadiness = "FAILED"
	sql := &fakeSQL{}
	result := run(facts, sql)
	if item, ok := find(result, "sml"); !ok || item.Status != Fail || result.Ready() || sql.statements != 0 {
		t.Fatalf("sml = %+v ready=%v statements=%d", item, result.Ready(), sql.statements)
	}
	if missing := run(Facts{}, sql); len(missing.Items) != 1 || missing.Items[0].Status != Fail {
		t.Fatalf("unknown shop = %+v", missing)
	}
}

func TestConfigurationGapsAreWarningsAndAnExpiredShopFails(t *testing.T) {
	facts := readyFacts()
	facts.ActiveRecipients, facts.VerifiedRecipients, facts.ActiveSchedules, facts.AssistantTokens = 0, 0, 0, 0
	facts.AccessEndsAt = now.Add(-time.Hour)
	result := run(facts, &fakeSQL{})
	for key, want := range map[string]Status{"tenant": Fail, "recipients": Warn, "schedule": Warn, "assistant": Info, "master_supplier": Info, "master_customer": Pass} {
		if item, ok := find(result, key); !ok || item.Status != want {
			t.Errorf("%s = %+v (want %s)", key, item, want)
		}
	}
}

func TestEveryApprovedReportIsRunOnTheShopAndAFailureIsNamedByReport(t *testing.T) {
	sql := &fakeSQL{failReport: "ap_supplier"} // the payables report reads suppliers
	result := run(readyFacts(), sql)
	failed, got := 0, 0
	for _, item := range result.Items {
		if strings.HasPrefix(item.Key, "report_") {
			got++
			if item.Status == Fail {
				failed++
				if !strings.Contains(item.Detail, "SML_QUERY_FAILED") {
					t.Errorf("a failure names its short code: %+v", item)
				}
			}
		}
	}
	reports := 0
	for _, definition := range report.Definitions() {
		if definition.Status == report.StatusActive {
			reports++
		}
	}
	if got != reports || failed == 0 || result.Ready() {
		t.Fatalf("reports checked %d of %d, failed %d, ready=%v", got, reports, failed, result.Ready())
	}
}

func TestDocumentTypeCoverageNamesWhatTheShopUsesThatNoReportCounts(t *testing.T) {
	sql := &fakeSQL{answers: map[string][]map[string]string{
		"group by t.trans_flag": {
			{"flag": "44", "label": "ขาย", "docs": "700"},
			{"flag": "9999", "label": "ใบพิเศษของร้าน", "docs": "300"},
			{"flag": "46", "label": "เพิ่มหนี้", "docs": "10"},
		}}}
	result := run(readyFacts(), sql)
	used, _ := find(result, "doc_types_used")
	if !strings.Contains(used.Title, "1010") {
		t.Errorf("used = %+v", used)
	}
	uncounted, ok := find(result, "doc_types_uncounted")
	if !ok || uncounted.Status != Warn || !strings.Contains(uncounted.Detail, "รหัส 9999 ใบพิเศษของร้าน 300 ใบ") {
		t.Fatalf("a type that is 30%% of the documents and counted by nothing must warn: %+v", uncounted)
	}
	if absent, ok := find(result, "doc_types_absent"); !ok || !strings.Contains(absent.Detail, "48") {
		t.Errorf("types the reports count and the shop has none of: %+v", absent)
	}
}

func TestDataGapsThatChangeWhatTheNumbersCanSayAreExplained(t *testing.T) {
	sql := &fakeSQL{answers: map[string][]map[string]string{
		"_metric_total_balance": {{"_metric_total_balance": "6608675.10", "_metric_no_due_date_amount": "6310670.10", "_metric_overdue_amount": "298505.00"}},
		"with_supplier":         {{"items": "518", "with_supplier": "0", "negative_stock": "3", "with_reorder_point": "0"}},
		"price_rows":            {{"price_rows": "0"}},
		"with_phone":            {{"customers": "5301", "with_phone": "427"}},
	}}
	result := run(readyFacts(), sql)
	for key, want := range map[string]Status{"ar_due_dates": Warn, "reorder_points": Warn, "item_suppliers": Info, "negative_stock": Warn, "prices": Info, "customer_phones": Info} {
		if item, ok := find(result, key); !ok || item.Status != want {
			t.Errorf("%s = %+v (want %s)", key, item, want)
		}
	}
	if item, _ := find(result, "ar_due_dates"); !strings.Contains(item.Title, "95%") || !strings.Contains(item.Detail, "ไม่ร่างทวง") {
		t.Errorf("receivable advice = %+v", item)
	}
}

func TestACleanShopHasNoWarningsAboutItsData(t *testing.T) {
	sql := &fakeSQL{answers: map[string][]map[string]string{
		"_metric_total_balance": {{"_metric_total_balance": "1000.00", "_metric_no_due_date_amount": "0.00", "_metric_overdue_amount": "100.00"}},
		"with_supplier":         {{"items": "100", "with_supplier": "90", "negative_stock": "0", "with_reorder_point": "80"}},
		"price_rows":            {{"price_rows": "400"}},
		"with_phone":            {{"customers": "100", "with_phone": "90"}},
	}}
	result := run(readyFacts(), sql)
	for _, key := range []string{"ar_due_dates", "reorder_points"} {
		if item, ok := find(result, key); !ok || item.Status != Pass {
			t.Errorf("%s = %+v", key, item)
		}
	}
	for _, key := range []string{"negative_stock", "prices", "customer_phones", "item_suppliers"} {
		if _, ok := find(result, key); ok {
			t.Errorf("%s should be silent for a clean shop", key)
		}
	}
}

func TestTheDocumentTypesTheReportsCountAreReadFromTheirOwnSQL(t *testing.T) {
	flags := countedFlags(report.Period{Preset: report.Custom, DateFrom: "2026-10-01", DateTo: "2026-10-07"})
	for _, want := range []int{44, 46, 48, 239} {
		if _, ok := flags[want]; !ok {
			t.Errorf("flag %d missing from %v", want, flags)
		}
	}
}
