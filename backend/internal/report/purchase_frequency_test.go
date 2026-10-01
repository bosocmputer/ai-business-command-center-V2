package report

import (
	"strings"
	"testing"
)

// frequencyDetailFixture is five customers, checked by hand:
//
//	C1 bought on 5 days across 40 days: rhythm 10 days, 12 days since the last
//	C2 bought on 3 days across 30 days: rhythm 15 days, 40 days since the last (late by 25)
//	C3 bought on 2 days across 10 days: rhythm 10 days, 16 days since the last (late by 6)
//	C4 bought on 1 day only: no rhythm
//	C5 bought on 4 days across 6 days: rhythm 2 days, 3 days since the last (under seven days, on track)
func frequencyDetailFixture() []map[string]string {
	row := func(cust, name, status, days, span, late, amount string) map[string]string {
		return map[string]string{"cust_code": cust, "cust_name": name, "status": status, "purchase_days": days, "span_days": span, "days_late": late, "sales_amount": amount}
	}
	return []map[string]string{
		row("C2", "ลูกค้า 2", frequencyStatusOverdue, "3", "30", "25.00", "7000.00"),
		row("C3", "ลูกค้า 3", frequencyStatusLate, "2", "10", "6.00", "9000.00"),
		row("C1", "ลูกค้า 1", frequencyStatusOnTrack, "5", "40", "2.00", "500.00"),
		row("C5", "ลูกค้า 5", frequencyStatusOnTrack, "4", "6", "1.00", "400.00"),
		row("C4", "ลูกค้า 4", frequencyStatusSingle, "1", "0", "", "100.00"),
	}
}

func frequencySummaryFixture() map[string][]map[string]string {
	// Average rhythm over the four repeat customers: (30+10+40+6) / (2+1+4+3) = 8.60.
	metrics := map[string]string{
		"_metric_row_count": "5", "_metric_customer_count": "4", "_metric_single_count": "1", "_metric_overdue_count": "1",
		"_metric_late_count": "1", "_metric_on_track_count": "2", "_metric_average_gap_days": "8.60", "_metric_overdue_amount": "7000.00",
	}
	merge := func(row map[string]string) map[string]string {
		for key, value := range metrics {
			row[key] = value
		}
		return row
	}
	status := func(code, count string) map[string]string {
		return merge(map[string]string{"_summary_kind": "statuses", "status": code, "customer_count": count})
	}
	return map[string][]map[string]string{"rows": {
		status(frequencyStatusOverdue, "1"), status(frequencyStatusLate, "1"), status(frequencyStatusOnTrack, "2"), status(frequencyStatusSingle, "1"),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C2", "cust_name": "ลูกค้า 2", "status": frequencyStatusOverdue, "days_late": "25.00", "sales_amount": "7000.00"}),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C3", "cust_name": "ลูกค้า 3", "status": frequencyStatusLate, "days_late": "6.00", "sales_amount": "9000.00"}),
	}}
}

func TestFrequencyDetailRowsSummariseToTheHandCheckedTotals(t *testing.T) {
	summary, err := Summarize(PurchaseFrequency, map[string][]map[string]string{"rows": frequencyDetailFixture()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"customer_count": "4", "single_count": "1", "overdue_count": "1", "overdue_amount": "7000.00", "late_count": "1", "on_track_count": "2", "average_gap_days": "8.60",
	}
	for name, value := range want {
		if summary.Metrics[name] != value {
			t.Errorf("%s = %q, want %q", name, summary.Metrics[name], value)
		}
	}
	if summary.RowCount != 5 {
		t.Errorf("row count = %d, want 5 (every customer who bought is accounted for)", summary.RowCount)
	}
}

func TestFrequencySummaryProjectionGivesTheSameFigures(t *testing.T) {
	fromDetail, err := Summarize(PurchaseFrequency, map[string][]map[string]string{"rows": frequencyDetailFixture()})
	if err != nil {
		t.Fatal(err)
	}
	fromSummary, err := Summarize(PurchaseFrequency, frequencySummaryFixture())
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range fromDetail.Metrics {
		if fromSummary.Metrics[name] != value {
			t.Errorf("%s: summary %q differs from detail %q", name, fromSummary.Metrics[name], value)
		}
	}
	if fromSummary.RowCount != fromDetail.RowCount {
		t.Errorf("row count: summary %d, detail %d", fromSummary.RowCount, fromDetail.RowCount)
	}
}

func TestFrequencyRejectsAnUnknownStatusInsteadOfDroppingIt(t *testing.T) {
	if _, err := frequencyTotalsFromDetail([]map[string]string{{"cust_code": "C9", "status": "MYSTERY"}}); err == nil {
		t.Fatal("an unknown status must be rejected, not silently dropped")
	}
}

func TestFrequencyDashboardIsTheSameFromDetailAndFromSummary(t *testing.T) {
	period := Period{Preset: Custom, DateFrom: "2026-04-03", DateTo: "2026-09-30"}
	for name, steps := range map[string]map[string][]map[string]string{
		"detail": {"rows": frequencyDetailFixture()}, "summary": frequencySummaryFixture(),
	} {
		dashboard, err := BuildDashboard(PurchaseFrequency, period, period, steps, steps)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		kpis := map[string]string{}
		for _, kpi := range dashboard.KPIs {
			kpis[kpi.Key] = kpi.Value
			if kpi.Comparison.Availability != ComparisonUnavailable {
				t.Errorf("%s: purchase frequency has no comparison but %s says %s", name, kpi.Key, kpi.Comparison.Availability)
			}
		}
		if kpis["customer_count"] != "4" || kpis["overdue_count"] != "1" || kpis["overdue_amount"] != "7000.00" || kpis["late_count"] != "1" || kpis["single_count"] != "1" || kpis["average_gap_days"] != "8.6000" {
			t.Errorf("%s: KPIs = %v", name, kpis)
		}
		byKey := map[string]DashboardVisualization{}
		for _, visualization := range dashboard.Visualizations {
			byKey[visualization.Key] = visualization
		}
		statuses, behind := byKey["purchase_frequency_statuses"], byKey["customers_behind_rhythm"]
		if len(statuses.Categories) != 4 || statuses.Categories[0] != "เงียบเกินรอบมาก" || statuses.Series[0].Values[2] != "2" {
			t.Errorf("%s: status chart = %+v", name, statuses)
		}
		if len(behind.Categories) != 2 || behind.Categories[0] != "ลูกค้า 3" || behind.Categories[1] != "ลูกค้า 2" {
			t.Errorf("%s: behind rhythm = %+v; want the two quiet customers, the one who bought most first (C3 9,000 before C2 7,000)", name, behind.Categories)
		}
	}
}

// The rules are written once in SQL. This pins the pieces that decide who is
// called late, because a wrong rule would tell an owner a good customer is lost.
func TestFrequencyRulesStayAsDocumented(t *testing.T) {
	for _, want := range []string{
		"group by t.cust_code, t.doc_date",                         // several bills on one day are one purchase day
		"(c.purchase_days - 1)",                                    // rhythm = span / (days - 1)
		"c.days_since_last >= 7 and c.days_since_last > 2 * c.gap", // overdue needs a week and double the rhythm
		"c.days_since_last >= 7 and c.days_since_last > 1.5 * c.gap",
		"when c.gap is null then 'SINGLE'",
		"coalesce(t.is_doc_copy, 0) <> 1",
	} {
		if !strings.Contains(purchaseFrequencyBaseSQL, want) {
			t.Errorf("purchase frequency base SQL lost %q", want)
		}
	}
}

func TestFrequencyQueriesShareOneBaseAndStayReadOnly(t *testing.T) {
	period := Period{Preset: Custom, DateFrom: "2026-04-03", DateTo: "2026-09-30"}
	for _, projection := range []ResultKind{ResultDetail, ResultSummary} {
		plan, err := BuildQueryPlanForProjection(PurchaseFrequency, period, projection)
		if err != nil || len(plan.Steps) != 1 {
			t.Fatalf("%s plan = %+v, %v", projection, plan, err)
		}
		query := plan.Steps[0].Query
		if len(query.Args) != 2 || query.Args[0] != "2026-04-03" || query.Args[1] != "2026-09-30" {
			t.Fatalf("%s args = %v; the first and last day are the only parameters", projection, query.Args)
		}
		rendered, err := RenderSQL(query)
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(rendered)
		for _, forbidden := range []string{"insert ", "update ", "delete ", "drop ", "alter ", "truncate "} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s SQL contains %q", projection, forbidden)
			}
		}
	}
	if !strings.Contains(purchaseFrequencySQL, purchaseFrequencyBaseSQL) || !strings.Contains(purchaseFrequencySummarySQL, purchaseFrequencyBaseSQL) {
		t.Fatal("detail and summary must be built from the same base query so their statuses cannot drift")
	}
}

func TestFrequencyIsAStandardUnchunkedDateRangeReport(t *testing.T) {
	definition, ok := DefinitionFor(PurchaseFrequency)
	if !ok || definition.ChunkSafe || definition.ParameterKind != DateRange || !definition.Sensitive || definition.Category != "CUSTOMER" {
		t.Fatalf("definition = %+v", definition)
	}
	if ComparisonSupported(PurchaseFrequency, Period{Preset: Custom}) || ComparisonSupported(PurchaseFrequency, Period{Preset: Yesterday}) {
		t.Fatal("a rhythm per customer has no meaningful previous period here")
	}
}
