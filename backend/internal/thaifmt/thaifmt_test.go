package thaifmt

import (
	"math/big"
	"testing"
	"time"
)

func TestDatesUseTheBuddhistEraAndAmountsKeepSatang(t *testing.T) {
	if got := Date(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)); got != "7 ต.ค. 2569" {
		t.Errorf("date = %q", got)
	}
	for text, want := range map[string]string{"6566875.1": "6,566,875.10", "0": "0.00", "-1234.5": "-1,234.50", "999.995": "1,000.00"} {
		value, _ := new(big.Rat).SetString(text)
		if got := Baht(value); got != want {
			t.Errorf("baht(%s) = %q, want %q", text, got, want)
		}
	}
}
