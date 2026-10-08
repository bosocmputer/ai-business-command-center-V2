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

func TestQuantitiesDropTrailingZerosAndKeepRealDecimals(t *testing.T) {
	for text, want := range map[string]string{"1250": "1,250", "12.5": "12.5", "0": "0", "100.0000": "100", "2.12345": "2.1235", "-3": "-3"} {
		value, _ := new(big.Rat).SetString(text)
		if got := Qty(value); got != want {
			t.Errorf("qty(%s) = %q, want %q", text, got, want)
		}
	}
}
