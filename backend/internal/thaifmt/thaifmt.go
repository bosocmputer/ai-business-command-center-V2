// Package thaifmt writes dates and amounts the way a Thai shop reads them: Buddhist Era years, thousands separators
// and two decimals for baht. Messages that AI-BCC writes itself (alerts, reminder drafts) share it so they agree.
package thaifmt

import (
	"fmt"
	"math/big"
	"strings"
	"time"
)

var months = [...]string{"ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}

// Date writes a date in Buddhist Era, e.g. 7 ต.ค. 2569.
func Date(day time.Time) string {
	return fmt.Sprintf("%d %s %d", day.Day(), months[day.Month()-1], day.Year()+543)
}

// Group puts a comma every three digits of a whole-number string.
func Group(digits string) string {
	for index := len(digits) - 3; index > 0; index -= 3 {
		digits = digits[:index] + "," + digits[index:]
	}
	return digits
}

// Baht writes an amount with thousands separators and satang: 6,566,875.10
func Baht(value *big.Rat) string {
	whole, fraction, _ := strings.Cut(value.FloatString(2), ".")
	sign := ""
	if strings.HasPrefix(whole, "-") {
		sign, whole = "-", whole[1:]
	}
	return sign + Group(whole) + "." + fraction
}

// Qty writes a quantity with thousands separators and no trailing zeros: 1,250 or 12.5 (up to four decimals).
func Qty(value *big.Rat) string {
	text := value.FloatString(4)
	if strings.Contains(text, ".") {
		text = strings.TrimSuffix(strings.TrimRight(text, "0"), ".")
	}
	sign := ""
	if strings.HasPrefix(text, "-") {
		sign, text = "-", text[1:]
	}
	whole, fraction, hasFraction := strings.Cut(text, ".")
	out := sign + Group(whole)
	if hasFraction {
		out += "." + fraction
	}
	return out
}
