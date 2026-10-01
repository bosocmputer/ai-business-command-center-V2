package report

import (
	"strings"
	"testing"
)

// rfmDetailFixture is four customers whose figures are checked by hand: C1 buys
// often and recently, C2 used to buy often and has gone quiet, C3 bought once
// long ago, C4 bought once recently.
func rfmDetailFixture() []map[string]string {
	row := func(cust, name, segment, monetary string) map[string]string {
		return map[string]string{"cust_code": cust, "cust_name": name, "segment": segment, "monetary": monetary, "frequency": "3"}
	}
	return []map[string]string{
		row("C1", "ลูกค้า 1", rfmSegmentChampion, "9000.00"),
		row("C2", "ลูกค้า 2", rfmSegmentAtRisk, "4000.00"),
		row("C3", "ลูกค้า 3", rfmSegmentHibernating, "500.00"),
		row("C4", "ลูกค้า 4", rfmSegmentPromising, "1500.00"),
	}
}

func rfmSummaryFixture() map[string][]map[string]string {
	metrics := map[string]string{
		"_metric_row_count": "4", "_metric_customer_count": "4", "_metric_total_amount": "15000.00", "_metric_champion_count": "1",
		"_metric_at_risk_count": "1", "_metric_at_risk_amount": "4000.00", "_metric_hibernating_count": "1",
	}
	merge := func(row map[string]string) map[string]string {
		for key, value := range metrics {
			row[key] = value
		}
		return row
	}
	segment := func(code, count, amount string) map[string]string {
		return merge(map[string]string{"_summary_kind": "segments", "segment": code, "customer_count": count, "monetary": amount})
	}
	return map[string][]map[string]string{"rows": {
		segment(rfmSegmentChampion, "1", "9000.00"), segment(rfmSegmentAtRisk, "1", "4000.00"),
		segment(rfmSegmentHibernating, "1", "500.00"), segment(rfmSegmentPromising, "1", "1500.00"),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C1", "cust_name": "ลูกค้า 1", "segment": rfmSegmentChampion, "monetary": "9000.00", "customer_count": "1"}),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C2", "cust_name": "ลูกค้า 2", "segment": rfmSegmentAtRisk, "monetary": "4000.00", "customer_count": "1"}),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C4", "cust_name": "ลูกค้า 4", "segment": rfmSegmentPromising, "monetary": "1500.00", "customer_count": "1"}),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C3", "cust_name": "ลูกค้า 3", "segment": rfmSegmentHibernating, "monetary": "500.00", "customer_count": "1"}),
	}}
}

func TestRFMDetailRowsSummariseToTheHandCheckedTotals(t *testing.T) {
	summary, err := Summarize(CustomerRFM, map[string][]map[string]string{"rows": rfmDetailFixture()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"customer_count": "4", "total_amount": "15000.00", "champion_count": "1",
		"at_risk_count": "1", "at_risk_amount": "4000.00", "hibernating_count": "1",
	}
	for name, value := range want {
		if summary.Metrics[name] != value {
			t.Errorf("%s = %q, want %q", name, summary.Metrics[name], value)
		}
	}
	if summary.RowCount != 4 {
		t.Errorf("row count = %d, want 4", summary.RowCount)
	}
}

func TestRFMSummaryProjectionGivesTheSameFigures(t *testing.T) {
	fromDetail, err := Summarize(CustomerRFM, map[string][]map[string]string{"rows": rfmDetailFixture()})
	if err != nil {
		t.Fatal(err)
	}
	fromSummary, err := Summarize(CustomerRFM, rfmSummaryFixture())
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range fromDetail.Metrics {
		if fromSummary.Metrics[name] != value {
			t.Errorf("%s: summary %q differs from detail %q", name, fromSummary.Metrics[name], value)
		}
	}
}

func TestRFMRejectsAnUnknownSegmentInsteadOfDroppingIt(t *testing.T) {
	if _, err := rfmTotalsFromDetail([]map[string]string{{"cust_code": "C9", "segment": "MYSTERY", "monetary": "1"}}); err == nil {
		t.Fatal("an unknown segment must be rejected, not silently dropped")
	}
}

func TestRFMDashboardIsTheSameFromDetailAndFromSummary(t *testing.T) {
	period := Period{Preset: Custom, DateFrom: "2026-04-03", DateTo: "2026-09-30"}
	for name, steps := range map[string]map[string][]map[string]string{
		"detail": {"rows": rfmDetailFixture()}, "summary": rfmSummaryFixture(),
	} {
		dashboard, err := BuildDashboard(CustomerRFM, period, period, steps, steps)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		kpis := map[string]string{}
		for _, kpi := range dashboard.KPIs {
			kpis[kpi.Key] = kpi.Value
			if kpi.Comparison.Availability != ComparisonUnavailable {
				t.Errorf("%s: a customer ranking has no comparison but %s says %s", name, kpi.Key, kpi.Comparison.Availability)
			}
		}
		if kpis["customer_count"] != "4" || kpis["total_amount"] != "15000.00" || kpis["at_risk_count"] != "1" || kpis["at_risk_amount"] != "4000.00" {
			t.Errorf("%s: KPIs = %v", name, kpis)
		}
		byKey := map[string]DashboardVisualization{}
		for _, visualization := range dashboard.Visualizations {
			byKey[visualization.Key] = visualization
		}
		segments, customers := byKey["customer_rfm_segments"], byKey["top_customers"]
		if len(segments.Categories) != 4 || segments.Categories[0] != "ลูกค้าดีเด่น" || segments.Series[0].Values[0] != "9000.00" {
			t.Errorf("%s: segment chart = %+v", name, segments)
		}
		if len(customers.Categories) != 4 || customers.Categories[0] != "ลูกค้า 1" || customers.Categories[3] != "ลูกค้า 3" {
			t.Errorf("%s: top customers = %+v; want largest first", name, customers.Categories)
		}
	}
}

// The segment rules are written once in SQL. This pins the direction of the
// scores, because the report this replaces had them inverted: the best customer
// must score 5, and recency is better when it is shorter.
func TestRFMScoresPointTheRightWay(t *testing.T) {
	for _, want := range []string{
		"order by $2::date - c.last_purchase_date desc", // shorter recency ranks higher
		"percent_rank() over (order by c.frequency)",    // more purchases ranks higher
		"percent_rank() over (order by c.monetary)",     // more money ranks higher
		"when s.r_score >= 4 and s.f_score >= 4 then 'CHAMPION'",
		"when s.r_score <= 2 and s.f_score >= 3 then 'AT_RISK'",
	} {
		if !strings.Contains(customerRFMBaseSQL, want) {
			t.Errorf("RFM base SQL lost %q", want)
		}
	}
	if strings.Contains(strings.ToLower(customerRFMBaseSQL), "ntile") {
		t.Error("NTILE splits tied customers arbitrarily; use percent_rank")
	}
}

func TestRFMQueriesShareOneBaseAndStayReadOnly(t *testing.T) {
	period := Period{Preset: Custom, DateFrom: "2026-04-03", DateTo: "2026-09-30"}
	for _, projection := range []ResultKind{ResultDetail, ResultSummary} {
		plan, err := BuildQueryPlanForProjection(CustomerRFM, period, projection)
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
	if !strings.Contains(customerRFMSQL, customerRFMBaseSQL) || !strings.Contains(customerRFMSummarySQL, customerRFMBaseSQL) {
		t.Fatal("detail and summary must be built from the same base query so their segments cannot drift")
	}
}

func TestRFMIsAStandardUnchunkedDateRangeReport(t *testing.T) {
	definition, ok := DefinitionFor(CustomerRFM)
	if !ok || definition.ChunkSafe || definition.ParameterKind != DateRange || !definition.Sensitive || definition.Category != "CUSTOMER" {
		t.Fatalf("definition = %+v", definition)
	}
	if ComparisonSupported(CustomerRFM, Period{Preset: Custom}) || ComparisonSupported(CustomerRFM, Period{Preset: Yesterday}) {
		t.Fatal("a ranking of customers by period has no meaningful previous period here")
	}
}
