package report

import (
	"strings"
	"testing"
)

// agingDetailFixture is a small receivable: two customers, one credit note, one
// document with no due date. Balances are checked by hand in the tests below.
func agingDetailFixture() []map[string]string {
	row := func(cust, name, doc, bucket, balance string) map[string]string {
		return map[string]string{"cust_code": cust, "cust_name": name, "doc_no": doc, "bucket": bucket, "balance": balance, "amount": balance, "paid_amount": "0"}
	}
	return []map[string]string{
		row("C1", "ลูกค้า 1", "D1", agingBucketNotDue, "1000.00"),
		row("C1", "ลูกค้า 1", "D2", agingBucketOverdue1, "500.00"),
		row("C1", "ลูกค้า 1", "D3", agingBucketOverdue5, "250.00"),
		row("C2", "ลูกค้า 2", "D4", agingBucketNoDueDate, "4000.00"),
		row("C2", "ลูกค้า 2", "D5", agingBucketOverdue3, "100.00"),
		row("C2", "ลูกค้า 2", "D6", agingBucketCredit, "-300.00"),
		row("C3", "ลูกค้า 3", "D7", agingBucketCredit, "-50.00"),
	}
}

func agingSummaryFixture() map[string][]map[string]string {
	metrics := map[string]string{
		"_metric_row_count": "7", "_metric_customer_count": "2", "_metric_total_balance": "5500.00", "_metric_overdue_amount": "850.00",
		"_metric_not_due_amount": "1000.00", "_metric_no_due_date_amount": "4000.00", "_metric_credit_amount": "-350.00",
	}
	merge := func(row map[string]string) map[string]string {
		for key, value := range metrics {
			row[key] = value
		}
		return row
	}
	return map[string][]map[string]string{"rows": {
		merge(map[string]string{"_summary_kind": "buckets", "bucket_not_due": "1000.00", "bucket_overdue_1_30": "500.00", "bucket_overdue_31_60": "0", "bucket_overdue_61_90": "100.00",
			"bucket_overdue_91_120": "0", "bucket_overdue_over_120": "250.00", "bucket_no_due_date": "4000.00", "bucket_credit": "-350.00"}),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C2", "cust_name": "ลูกค้า 2", "balance": "3800.00", "overdue_balance": "100.00"}),
		merge(map[string]string{"_summary_kind": "ranking", "cust_code": "C1", "cust_name": "ลูกค้า 1", "balance": "1750.00", "overdue_balance": "750.00"}),
	}}
}

func TestAgingDetailRowsSummariseToTheHandCheckedTotals(t *testing.T) {
	summary, err := Summarize(ARAging, map[string][]map[string]string{"rows": agingDetailFixture()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"customer_count": "2", "document_count": "7", "total_balance": "5500.00", "overdue_amount": "850.00",
		"not_due_amount": "1000.00", "no_due_date_amount": "4000.00", "credit_amount": "-350.00",
	}
	for name, value := range want {
		if summary.Metrics[name] != value {
			t.Errorf("%s = %q, want %q", name, summary.Metrics[name], value)
		}
	}
	if summary.RowCount != 7 {
		t.Errorf("row count = %d, want 7", summary.RowCount)
	}
}

func TestAgingSummaryProjectionGivesTheSameFigures(t *testing.T) {
	fromDetail, err := Summarize(ARAging, map[string][]map[string]string{"rows": agingDetailFixture()})
	if err != nil {
		t.Fatal(err)
	}
	fromSummary, err := Summarize(ARAging, agingSummaryFixture())
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range fromDetail.Metrics {
		if fromSummary.Metrics[name] != value {
			t.Errorf("%s: summary %q differs from detail %q", name, fromSummary.Metrics[name], value)
		}
	}
}

func TestAgingNeverInventsADueDateAndKeepsCreditsSeparate(t *testing.T) {
	totals, err := agingTotalsFromDetail(agingDetailFixture())
	if err != nil {
		t.Fatal(err)
	}
	if got := money(totals.buckets[agingBucketNoDueDate]); got != "4000.00" {
		t.Fatalf("no-due-date bucket = %s", got)
	}
	if got := money(totals.overdue); got != "850.00" {
		t.Fatalf("overdue must not include documents without a due date or not yet due: %s", got)
	}
	if got := money(totals.credit); got != "-350.00" {
		t.Fatalf("credit bucket = %s", got)
	}
	// A credit-only customer is not a debtor.
	if totals.customers != 2 {
		t.Fatalf("customers = %d, want 2 (C3 only holds a credit)", totals.customers)
	}
	if _, err := agingTotalsFromDetail([]map[string]string{{"cust_code": "C9", "bucket": "MYSTERY", "balance": "1"}}); err == nil {
		t.Fatal("an unknown bucket must be rejected, not silently dropped")
	}
}

func TestAgingDashboardIsTheSameFromDetailAndFromSummary(t *testing.T) {
	period := Period{Preset: AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	for name, steps := range map[string]map[string][]map[string]string{
		"detail": {"rows": agingDetailFixture()}, "summary": agingSummaryFixture(),
	} {
		dashboard, err := BuildDashboard(ARAging, period, period, steps, steps)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		kpis := map[string]string{}
		for _, kpi := range dashboard.KPIs {
			kpis[kpi.Key] = kpi.Value
			if kpi.Comparison.Availability != ComparisonUnavailable {
				t.Errorf("%s: aging has no comparison but %s says %s", name, kpi.Key, kpi.Comparison.Availability)
			}
		}
		if kpis["total_balance"] != "5500.00" || kpis["overdue_amount"] != "850.00" || kpis["no_due_date_amount"] != "4000.00" || kpis["customer_count"] != "2" {
			t.Errorf("%s: KPIs = %v", name, kpis)
		}
		byKey := map[string]DashboardVisualization{}
		for _, visualization := range dashboard.Visualizations {
			byKey[visualization.Key] = visualization
		}
		buckets, debtors := byKey["ar_aging_buckets"], byKey["top_debtors"]
		if len(buckets.Categories) == 0 || buckets.Categories[0] != "ยังไม่ครบกำหนด" {
			t.Errorf("%s: bucket chart = %+v", name, buckets)
		}
		if len(debtors.Categories) != 2 || debtors.Categories[0] != "ลูกค้า 2" {
			t.Errorf("%s: top debtors = %+v; want C2 (3,800) before C1 (1,750) and no credit-only customer", name, debtors.Categories)
		}
	}
}

func TestAgingQueriesShareOneBaseAndStayReadOnly(t *testing.T) {
	period := Period{Preset: AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	for _, projection := range []ResultKind{ResultDetail, ResultSummary} {
		plan, err := BuildQueryPlanForProjection(ARAging, period, projection)
		if err != nil || len(plan.Steps) != 1 {
			t.Fatalf("%s plan = %+v, %v", projection, plan, err)
		}
		query := plan.Steps[0].Query
		if len(query.Args) != 1 || query.Args[0] != "2026-10-01" {
			t.Fatalf("%s args = %v; the as-of date is the only parameter", projection, query.Args)
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
		if !strings.Contains(rendered, "ap_ar_trans_detail") || !strings.Contains(rendered, "NO_DUE_DATE") {
			t.Fatalf("%s SQL lost the payment join or the no-due-date bucket", projection)
		}
	}
	if !strings.Contains(arAgingSQL, arAgingBaseSQL) || !strings.Contains(arAgingSummarySQL, arAgingBaseSQL) {
		t.Fatal("detail and summary must be built from the same base query so their totals cannot drift")
	}
}

func TestAgingIsHeavyUnchunkedAndHasNoComparison(t *testing.T) {
	definition, ok := DefinitionFor(ARAging)
	if !ok || definition.RefreshClass != RefreshHeavy || definition.ChunkSafe || definition.ParameterKind != AsOfDate || !definition.Sensitive {
		t.Fatalf("definition = %+v", definition)
	}
	if ComparisonSupported(ARAging, Period{Preset: AsOfRun}) || ComparisonSupported(ARAging, Period{Preset: Yesterday}) {
		t.Fatal("aging as of a date has no meaningful previous period")
	}
}

// Aging and the receivable movement report must count the same document types,
// or their totals drift apart on a shop that uses the rarer ones. Each rule is
// written the same way in both queries.
func TestAgingCountsTheSameDocumentTypesAsMovement(t *testing.T) {
	for _, rule := range []string{
		"t.trans_flag in (44, 250) and t.inquiry_type in (0, 2)",
		"t.trans_flag = 48 and t.inquiry_type in (0, 2, 4)",
		"t.trans_flag = 262 and t.inquiry_type not in (1, 3)",
		"t.trans_flag in (93, 99, 95, 101, 254, 418)",
		"t.trans_flag in (97, 103)",
	} {
		if !strings.Contains(arAgingBaseSQL, rule) {
			t.Errorf("aging lost %q", rule)
		}
		if !strings.Contains(arCustomerMovementSQL, rule) {
			t.Errorf("movement no longer has %q, so aging and movement may disagree", rule)
		}
	}
}
