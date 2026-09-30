package main

import (
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

func TestSameRowsIgnoresOrderButNotContentOrDuplicates(t *testing.T) {
	a := []map[string]string{{"code": "1", "amount": "5"}, {"code": "2", "amount": "7"}}
	b := []map[string]string{{"code": "2", "amount": "7"}, {"code": "1", "amount": "5"}}
	if !sameRows(a, b) {
		t.Fatal("the same rows in a different order must match")
	}
	if sameRows(a, []map[string]string{{"code": "1", "amount": "5"}, {"code": "2", "amount": "8"}}) {
		t.Fatal("a changed value must not match")
	}
	if sameRows(a, append(b, map[string]string{"code": "1", "amount": "5"})) {
		t.Fatal("a different row count must not match")
	}
	if sameRows([]map[string]string{{"c": "1"}, {"c": "1"}}, []map[string]string{{"c": "1"}, {"c": "2"}}) {
		t.Fatal("duplicates must be counted, not treated as a set")
	}
}

func TestCompareReportsOnlyNamesOfDifferences(t *testing.T) {
	row := func(code, balance string) map[string]string {
		return map[string]string{"ic_code": code, "ic_name": "n" + code, "balance_amount": balance, "amount_in": "1", "amount_out": "1"}
	}
	period := report.Period{Preset: report.AsOfRun, DateFrom: "2026-09-30", DateTo: "2026-09-30"}
	same := stepRows{"rows": {row("1", "10"), row("2", "20")}}
	reordered := stepRows{"rows": {row("2", "20"), row("1", "10")}}
	if diff := compare(report.StockBalance, period, report.ResultDetail, same, reordered); len(diff) != 0 {
		t.Fatalf("reordered rows must be equal: %v", diff)
	}
	changed := stepRows{"rows": {row("1", "10"), row("2", "99")}}
	diff := compare(report.StockBalance, period, report.ResultDetail, same, changed)
	if len(diff) == 0 || diff[0] != "detail rows" {
		t.Fatalf("a changed value must be reported by name: %v", diff)
	}
}

func TestCanonicalRowsTreatsTrailingZerosAsEqualButNotDifferentNumbers(t *testing.T) {
	left := []map[string]string{{"code": "001", "amount": "12.50"}}
	if !sameRows(canonicalRows(left), canonicalRows([]map[string]string{{"code": "001", "amount": "12.5000"}})) {
		t.Fatal("12.50 and 12.5000 must be equal once canonical")
	}
	if sameRows(canonicalRows(left), canonicalRows([]map[string]string{{"code": "001", "amount": "12.51"}})) {
		t.Fatal("different numbers must stay different")
	}
	if sameRows(canonicalRows(left), canonicalRows([]map[string]string{{"code": "1", "amount": "12.50"}})) {
		t.Fatal("codes like 001 and 1 are text keys, not numbers to be merged")
	}
}
