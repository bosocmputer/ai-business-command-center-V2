package report

import (
	"strings"
	"testing"
)

func TestSummarizeProducesStableLineMetricsForAllReports(t *testing.T) {
	tests := []struct {
		key     Key
		steps   map[string][]map[string]string
		metrics map[string]string
	}{
		{SalesGoodsServices, map[string][]map[string]string{"headers": {{"doc_no": "S1", "total_amount": "0.10"}, {"doc_no": "S2", "total_amount": "0.20"}}, "details": {{"sum_amount": "0.30"}}}, map[string]string{"document_count": "2", "total_amount": "0.30"}},
		{PurchaseGoodsPayables, map[string][]map[string]string{"headers": {{"doc_no": "P1", "total_amount": "100.00"}}, "details": {}}, map[string]string{"document_count": "1", "total_amount": "100.00"}},
		{GrossProfitByProduct, map[string][]map[string]string{"rows": {{"amount_sale": "100", "cost_sale": "60", "amount_sale_return": "10", "cost_sale_return": "5"}}}, map[string]string{"gross_profit_amount": "35.00", "gross_margin_percent": "38.89"}},
		{GrossProfitByARCustomer, map[string][]map[string]string{"rows": {{"amount_sale": "200", "cost_sale": "150", "amount_sale_return": "0", "cost_sale_return": "0"}}}, map[string]string{"gross_profit_amount": "50.00", "gross_margin_percent": "25.00"}},
		{StockBalance, map[string][]map[string]string{"rows": {{"ic_code": "A", "balance_amount": "12.34"}, {"ic_code": "B", "balance_amount": "7.66"}}}, map[string]string{"item_count": "2", "balance_amount": "20.00"}},
		{StockReorder, map[string][]map[string]string{"rows": {{"ic_code": "A", "balance_qty": "2", "purchase_point": "5"}, {"ic_code": "B", "balance_qty": "-1", "purchase_point": "2"}}}, map[string]string{"reorder_item_count": "2", "shortage_qty": "6.0000"}},
		{ARCustomerMovement, map[string][]map[string]string{"rows": {{"cust_code": "C1", "doc_sort": "1", "amount": "100"}, {"cust_code": "C1", "doc_sort": "2", "amount": "10"}, {"cust_code": "C2", "doc_sort": "3", "amount": "20"}}}, map[string]string{"customer_count": "2", "net_movement_amount": "70.00"}},
		{ARDebtReceipt, map[string][]map[string]string{"rows": {{"doc_no": "R1", "total_net_value": "40"}, {"doc_no": "R2", "total_net_value": "60"}}}, map[string]string{"receipt_count": "2", "total_received_amount": "100.00"}},
		{CashBankReceipts, map[string][]map[string]string{"rows": {{"doc_no": "CB1", "total_amount": "25.50"}}}, map[string]string{"document_count": "1", "total_amount": "25.50"}},
		{CashBankPayments, map[string][]map[string]string{"rows": {{"doc_no": "CB2", "total_amount": "9.75"}}}, map[string]string{"document_count": "1", "total_amount": "9.75"}},
	}
	for _, test := range tests {
		t.Run(string(test.key), func(t *testing.T) {
			result, err := Summarize(test.key, test.steps)
			if err != nil {
				t.Fatalf("Summarize() error = %v", err)
			}
			for key, expected := range test.metrics {
				if got := result.Metrics[key]; got != expected {
					t.Errorf("metric %s = %q, want %q; all=%v", key, got, expected, result.Metrics)
				}
			}
			if result.RowCount == 0 || len(result.Rows) == 0 {
				t.Fatalf("result rows were not retained: %+v", result)
			}
		})
	}
}

func TestSummarizeRejectsMalformedMoneyInsteadOfSilentlyReturningZero(t *testing.T) {
	_, err := Summarize(CashBankReceipts, map[string][]map[string]string{"rows": {{"total_amount": "not-a-number"}}})
	if err == nil {
		t.Fatal("malformed monetary value was accepted")
	}
}

func TestSummarizeUsesFullSetMetricsFromBoundedSummaryRows(t *testing.T) {
	steps := map[string][]map[string]string{"rows": {{
		"ic_code": "TOP-1", "ic_name": "สินค้าอันดับหนึ่ง", "balance_amount": "100.00", "amount_in": "10", "amount_out": "5",
		"_metric_item_count": "500", "_metric_balance_amount": "10000.00",
		"_metric_amount_in": "1200.00", "_metric_amount_out": "800.00", "_metric_row_count": "500",
	}}}
	result, err := Summarize(StockBalance, steps)
	if err != nil {
		t.Fatal(err)
	}
	if result.RowCount != 500 || result.Metrics["item_count"] != "500" || result.Metrics["balance_amount"] != "10000.00" {
		t.Fatalf("bounded summary lost full-set metrics: %+v", result)
	}
	dashboard, err := BuildDashboard(StockBalance,
		Period{Preset: AsOfRun, DateFrom: "2026-07-14", DateTo: "2026-07-14"},
		Period{Preset: Custom, DateFrom: "2026-07-13", DateTo: "2026-07-13"}, steps, map[string][]map[string]string{"rows": {}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.KPIs[0].Value != "10000.00" || dashboard.KPIs[2].Value != "1200.00" {
		t.Fatalf("dashboard KPIs were calculated from top rows instead of full-set metrics: %+v", dashboard.KPIs)
	}
}

func TestDecimalAcceptsPostgresScientificNotationExactly(t *testing.T) {
	tests := map[string]string{
		"0E-14":     "0",
		"0E-15":     "0",
		"0E-16":     "0",
		"0E-17":     "0",
		"0E-20":     "0",
		"0E-22":     "0",
		"-5.00E-13": "-1/2000000000000",
		"1E+3":      "1000",
	}
	for value, expected := range tests {
		t.Run(value, func(t *testing.T) {
			parsed, err := decimal(value)
			if err != nil {
				t.Fatalf("decimal(%q) error = %v", value, err)
			}
			if got := parsed.RatString(); got != expected {
				t.Fatalf("decimal(%q) = %s, want %s", value, got, expected)
			}
		})
	}
}

func TestDecimalRejectsUnsafeOrMalformedValues(t *testing.T) {
	for _, value := range []string{
		"NaN", "Infinity", "-Infinity", "1/2", " 1", "1 ", "1E", "1E10001", strings.Repeat("1", 257),
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := decimal(value); err == nil {
				t.Fatalf("decimal(%q) unexpectedly succeeded", value)
			}
		})
	}
}

func TestStockReportsAcceptScientificNotationReturnedByDATA1(t *testing.T) {
	period := Period{Preset: AsOfRun, DateFrom: "2026-07-14", DateTo: "2026-07-14"}
	comparison := Period{Preset: Custom, DateFrom: "2026-07-13", DateTo: "2026-07-13"}

	stockRows := map[string][]map[string]string{"rows": {
		{
			"ic_code": "A", "ic_name": "สินค้า A", "balance_amount": "12745001.62760995667801600", "balance_qty": "0E-20",
			"qty_in": "0E-14", "amount_in": "0", "qty_out": "0E-15", "amount_out": "0",
		},
		{
			"ic_code": "B", "ic_name": "สินค้า B", "balance_amount": "-5.00E-13", "balance_qty": "0E-20",
			"qty_in": "0", "amount_in": "0", "qty_out": "0", "amount_out": "0",
		},
	}}
	stockSummary, err := Summarize(StockBalance, stockRows)
	if err != nil {
		t.Fatalf("Summarize(stock_balance) error = %v", err)
	}
	if got := stockSummary.Metrics["balance_amount"]; got != "12745001.63" {
		t.Fatalf("stock balance amount = %q", got)
	}
	if _, err := BuildDashboard(StockBalance, period, comparison, stockRows, map[string][]map[string]string{"rows": {}}); err != nil {
		t.Fatalf("BuildDashboard(stock_balance) error = %v", err)
	}

	reorderRows := map[string][]map[string]string{"rows": {{
		"ic_code": "A", "ic_name": "สินค้า A", "balance_qty": "0E-22", "purchase_point": "5.0000", "purchase_balance_qty": "0E-14",
	}}}
	reorderSummary, err := Summarize(StockReorder, reorderRows)
	if err != nil {
		t.Fatalf("Summarize(stock_reorder) error = %v", err)
	}
	if got := reorderSummary.Metrics["shortage_qty"]; got != "5.0000" {
		t.Fatalf("stock reorder shortage = %q", got)
	}
	if _, err := BuildDashboard(StockReorder, period, comparison, reorderRows, map[string][]map[string]string{"rows": {}}); err != nil {
		t.Fatalf("BuildDashboard(stock_reorder) error = %v", err)
	}
}

func TestTheReorderReportCarriesAnAssistantOnlyListOfItemsBelowTheirPoint(t *testing.T) {
	period := Period{Preset: AsOfRun, DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	rows := map[string][]map[string]string{"rows": {
		{"ic_code": "A", "ic_name": "ปูน", "ic_unit_code": "BAG~ถุง", "balance_qty": "40", "purchase_point": "100", "purchase_balance_qty": "30"},
		{"ic_code": "B", "ic_name": "เหล็ก", "ic_unit_code": "PCS~เส้น", "balance_qty": "-5", "purchase_point": "10", "purchase_balance_qty": ""},
		{"ic_code": "C", "ic_name": "ทราย", "ic_unit_code": "M3~คิว", "balance_qty": "50", "purchase_point": "50"},
	}}
	dashboard, err := BuildDashboard(StockReorder, period, period, rows, map[string][]map[string]string{"rows": {}})
	if err != nil {
		t.Fatal(err)
	}
	var list DashboardVisualization
	for _, visualization := range dashboard.Visualizations {
		if visualization.Key == "agent_reorder_items" {
			list = visualization
		}
	}
	// B is short by 150% of its point, A by 60%, and C is exactly at its point so it is not below it.
	if got := strings.Join(list.Categories, ","); got != "B,A" {
		t.Fatalf("items = %q (%+v)", got, list)
	}
	if got := strings.Join(list.Series[0].Values, ","); got != "15.0000,60.0000" || strings.Join(list.Series[0].PointLabels, ",") != "เหล็ก,ปูน" ||
		strings.Join(list.Series[1].PointLabels, ",") != "เส้น,ถุง" || strings.Join(list.Series[3].Values, ",") != "0.0000,30.0000" {
		t.Errorf("series = %+v", list.Series)
	}
}

func TestCashReportsKeepTheirTotalAndSayHowMuchOfItIsMoneyMovedBetweenTheShopsOwnAccounts(t *testing.T) {
	rows := []map[string]string{
		{"doc_no": "A", "trans_flag_code": "239", "total_amount": "1000.00"},
		{"doc_no": "B", "trans_flag_code": "401", "total_amount": "300.00"}, // deposit
		{"doc_no": "C", "trans_flag_code": "402", "total_amount": "50.50"},  // withdrawal
		{"doc_no": "D", "trans_flag_code": "40", "total_amount": "200.00"},  // advance receipt: real money in
		{"doc_no": "E", "trans_flag_code": "301", "total_amount": "10.00"},  // petty cash return
		{"doc_no": "F", "trans_flag_code": "", "total_amount": "5.00"},      // no code: counted in the total, not as internal
	}
	for _, key := range []Key{CashBankReceipts, CashBankPayments} {
		summary, err := Summarize(key, map[string][]map[string]string{"rows": rows})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Metrics["total_amount"] != "1565.50" || summary.Metrics["internal_move_amount"] != "360.50" || summary.Metrics["external_amount"] != "1205.00" {
			t.Errorf("%s metrics = %v: the total must stay whole and the internal part is named beside it", key, summary.Metrics)
		}
	}
	// A summary run carries the figure as its own metric and it wins over the rows.
	summary, err := Summarize(CashBankPayments, map[string][]map[string]string{"rows": {{"_metric_total_amount": "900.00", "_metric_document_count": "3", "_metric_internal_move_amount": "250.00", "_summary_metric_row": "true"}}})
	if err != nil || summary.Metrics["internal_move_amount"] != "250.00" || summary.Metrics["total_amount"] != "900.00" || summary.Metrics["external_amount"] != "650.00" {
		t.Fatalf("summary metrics = %v %v", summary.Metrics, err)
	}
	// The debt receipt report is a different report and has no such figure.
	debt, err := Summarize(ARDebtReceipt, map[string][]map[string]string{"rows": {{"doc_no": "R1", "total_net_value": "10.00"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := debt.Metrics["internal_move_amount"]; has {
		t.Errorf("debt receipts have no internal moves: %v", debt.Metrics)
	}
}

func TestTheCashSummaryQueryCarriesTheInternalMoveFigureAndStaysReadOnly(t *testing.T) {
	period := Period{Preset: Custom, DateFrom: "2026-10-01", DateTo: "2026-10-07"}
	for _, key := range []Key{CashBankReceipts, CashBankPayments} {
		plan, err := BuildQueryPlanForProjection(key, period, ResultSummary)
		if err != nil || len(plan.Steps) != 1 {
			t.Fatalf("%s: %+v %v", key, plan, err)
		}
		sql := strings.ToLower(plan.Steps[0].Query.SQL)
		if !strings.Contains(sql, "_metric_internal_move_amount") || !strings.Contains(sql, "trans_flag_code in (301, 302, 303, 401, 402, 403, 423)") {
			t.Errorf("%s: the summary must compute the internal figure with the shared list:\n%s", key, sql)
		}
	}
	plan, _ := BuildQueryPlanForProjection(ARDebtReceipt, period, ResultSummary)
	if strings.Contains(strings.ToLower(plan.Steps[0].Query.SQL), "trans_flag_code in") {
		t.Error("the debt receipt report has no document code column and must not filter on it")
	}
}

// Net sales = sale documents (44) + debit notes (46) - credit notes / returns (48).
// Rows carry the sign already: a return is negative.
func TestSummarizeSalesIsNetOfDebitAndCreditNotesAndShowsEachPart(t *testing.T) {
	steps := map[string][]map[string]string{"headers": {
		{"doc_no": "S1", "trans_flag": "44", "total_amount": "100.00"},
		{"doc_no": "S2", "trans_flag": "44", "total_amount": "50.00"},
		{"doc_no": "D1", "trans_flag": "46", "total_amount": "10.00"},
		{"doc_no": "R1", "trans_flag": "48", "total_amount": "-30.00"},
		{"doc_no": "R2", "trans_flag": "48", "total_amount": "-5.50"},
	}, "details": {{"sum_amount": "124.50"}}}
	result, err := Summarize(SalesGoodsServices, steps)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"total_amount": "124.50", "document_count": "2", "sales_amount": "150.00",
		"debit_note_amount": "10.00", "debit_note_count": "1", "return_amount": "35.50", "return_count": "2",
	}
	for key, expected := range want {
		if got := result.Metrics[key]; got != expected {
			t.Errorf("metric %s = %q, want %q; all=%v", key, got, expected, result.Metrics)
		}
	}
	if result.Reconciliation["difference"] != "0.00" {
		t.Errorf("header and detail totals should agree, got %v", result.Reconciliation)
	}
}

func TestSummarizeSalesReadsBreakdownFromTheFullSetQueryMetrics(t *testing.T) {
	steps := map[string][]map[string]string{"headers": {{
		"doc_date": "2026-06-01", "total_amount": "3507260.19",
		"_metric_document_count": "5439", "_metric_total_amount": "140815932.93", "_metric_detail_total": "140815932.93", "_metric_row_count": "5794",
		"_metric_sales_amount": "142142365.29", "_metric_debit_note_count": "2", "_metric_debit_note_amount": "365.00",
		"_metric_return_count": "353", "_metric_return_amount": "1326797.36",
	}}}
	result, err := Summarize(SalesGoodsServices, steps)
	if err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]string{
		"total_amount": "140815932.93", "document_count": "5439", "sales_amount": "142142365.29",
		"debit_note_amount": "365.00", "debit_note_count": "2", "return_amount": "1326797.36", "return_count": "353",
	} {
		if got := result.Metrics[key]; got != expected {
			t.Errorf("metric %s = %q, want %q", key, got, expected)
		}
	}
}

func TestSalesQueriesNetTheNotesAndLeavePurchasesAlone(t *testing.T) {
	period := Period{DateFrom: "2026-06-01", DateTo: "2026-06-30"}
	sales, err := BuildQueryPlanForProjection(SalesGoodsServices, period, ResultSummary)
	if err != nil {
		t.Fatal(err)
	}
	headers := sales.Steps[0].Query.SQL
	for _, want := range []string{"trans_flag in (44, 46, 48)", "_metric_return_amount", "_metric_debit_note_amount", "count(*) filter (where trans_flag = 44)", "(h.trans_flag <> 44 or coalesce(h.doc_ref, '') = '' or h.is_pos = 0)"} {
		if !strings.Contains(headers, want) {
			t.Errorf("sales summary SQL is missing %q", want)
		}
	}
	for _, step := range sales.Steps {
		if strings.Contains(step.Query.SQL, "trans_flag in (44)") {
			t.Errorf("step %s still counts sale documents only", step.Name)
		}
	}
	if !strings.Contains(sales.Steps[1].Query.SQL, "when d.trans_flag = 48 then -1") {
		t.Error("detail lines of returns are not negated")
	}
	purchases, err := BuildQueryPlanForProjection(PurchaseGoodsPayables, period, ResultSummary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(purchases.Steps[0].Query.SQL, "_metric_return_amount") || !strings.Contains(purchases.Steps[0].Query.SQL, "count(*) as _metric_document_count") {
		t.Error("the purchase summary must not change")
	}
}

// The part of a cash book row that was settled with an advance already paid or received is not new money: it is counted once
// when the advance moves (document 10 or 40) and must not be counted again when it is applied.
func TestSummarizeCashReportsSeparateAdvancesAppliedFromInternalMoves(t *testing.T) {
	rows := []map[string]string{
		{"doc_no": "P1", "trans_flag_code": "19", "total_amount": "1000.00", "advance_applied_amount": "400.00"},
		{"doc_no": "P2", "trans_flag_code": "10", "total_amount": "500.00", "advance_applied_amount": "0"},
		{"doc_no": "P3", "trans_flag_code": "401", "total_amount": "200.00", "advance_applied_amount": "0"},
		{"doc_no": "P4", "trans_flag_code": "301", "total_amount": "50.00", "advance_applied_amount": "50.00"}, // internal rows are never counted twice
	}
	result, err := Summarize(CashBankPayments, map[string][]map[string]string{"rows": rows})
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"total_amount": "1750.00", "internal_move_amount": "250.00", "advance_applied_amount": "400.00", "external_amount": "1100.00"} {
		if got := result.Metrics[key]; got != want {
			t.Errorf("metric %s = %q, want %q; all=%v", key, got, want, result.Metrics)
		}
	}
	// Rows from stored data that predate the column count as no advance applied.
	old, err := Summarize(CashBankReceipts, map[string][]map[string]string{"rows": {{"doc_no": "R1", "trans_flag_code": "44", "total_amount": "10.00"}}})
	if err != nil {
		t.Fatal(err)
	}
	if old.Metrics["advance_applied_amount"] != "0.00" || old.Metrics["external_amount"] != "10.00" {
		t.Errorf("rows without the column must not change the figures: %v", old.Metrics)
	}
}

func TestCashSummaryQueriesCarryTheAdvanceMetricAndTheDebtReceiptReportDoesNot(t *testing.T) {
	period := Period{DateFrom: "2026-06-01", DateTo: "2026-06-30"}
	for _, key := range []Key{CashBankReceipts, CashBankPayments} {
		plan, err := BuildQueryPlanForProjection(key, period, ResultSummary)
		if err != nil {
			t.Fatal(err)
		}
		sql := plan.Steps[0].Query.SQL
		if !strings.Contains(sql, "_metric_advance_applied_amount") || !strings.Contains(sql, "coalesce(cb.deposit_amount, 0) as advance_applied_amount") || !strings.Contains(sql, "trans_flag_code not in (") {
			t.Errorf("%s summary SQL does not carry the advance metric", key)
		}
	}
	plan, err := BuildQueryPlanForProjection(ARDebtReceipt, period, ResultSummary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.Steps[0].Query.SQL, "0 as _metric_advance_applied_amount") {
		t.Error("the debt receipt report settles debts, so advances applied stay inside it")
	}
}
