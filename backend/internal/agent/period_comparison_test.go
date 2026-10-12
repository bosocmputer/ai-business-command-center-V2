package agent

import (
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

// Owners think of a range from the 1st as "the month so far": its comparison must be the same days of the month before.
func TestResolvePeriodMonthRangesCompareWithTheMonthBefore(t *testing.T) {
	location := time.FixedZone("ICT", 7*3600)
	now := time.Date(2026, 10, 12, 14, 0, 0, 0, location)
	definition, ok := report.DefinitionFor(report.SalesGoodsServices)
	if !ok {
		t.Fatal("sales report definition missing")
	}
	cases := []struct {
		from, to string
		preset   report.Preset
		compare  report.Period
	}{
		{"2026-10-01", "2026-10-12", report.MonthToDate, report.Period{Preset: report.Custom, DateFrom: "2026-09-01", DateTo: "2026-09-12"}},
		{"2026-09-01", "2026-09-30", report.MonthToDate, report.Period{Preset: report.Custom, DateFrom: "2026-08-01", DateTo: "2026-08-31"}},
		{"2026-10-05", "2026-10-11", report.Custom, report.Period{Preset: report.Custom, DateFrom: "2026-09-28", DateTo: "2026-10-04"}},
		{"2026-09-20", "2026-10-05", report.Custom, report.Period{Preset: report.Custom, DateFrom: "2026-09-04", DateTo: "2026-09-19"}},
	}
	for _, test := range cases {
		period, err := resolvePeriod(definition, location, now, test.from, test.to)
		if err != nil {
			t.Fatalf("%s..%s: %v", test.from, test.to, err)
		}
		if period.Preset != test.preset {
			t.Fatalf("%s..%s: preset %s, want %s", test.from, test.to, period.Preset, test.preset)
		}
		got, err := report.ResolveComparisonPeriod(period)
		if err != nil || got != test.compare {
			t.Fatalf("%s..%s: comparison %+v (%v), want %+v", test.from, test.to, got, err, test.compare)
		}
	}
}
