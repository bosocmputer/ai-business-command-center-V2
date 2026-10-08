// Package alert checks, once a day, the rules an owner set up by talking to the assistant, writes the message for each
// one that is true, and hands it to the assistant's webhook. The words and figures are written here, not by a model,
// so an alert says exactly what AI-BCC computed.
package alert

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/thaifmt"
)

var thaiWeekdays = [...]string{"อาทิตย์", "จันทร์", "อังคาร", "พุธ", "พฤหัสบดี", "ศุกร์", "เสาร์"}

// thaiDate writes a date in Buddhist Era, e.g. 7 ต.ค. 2569.
func thaiDate(day time.Time) string { return thaifmt.Date(day) }

func thaiWeekday(day time.Time) string { return thaiWeekdays[day.Weekday()] }

// thaiDateTime writes the time SML was read, e.g. 8 ต.ค. 2569 เวลา 10:54 น. An unreadable value gives "".
func thaiDateTime(rfc3339 string, location *time.Location) string {
	moment, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	moment = moment.In(location)
	return fmt.Sprintf("%s เวลา %s น.", thaiDate(moment), moment.Format("15:04"))
}

// baht writes an amount with thousands separators and satang: 6,566,875.10
func baht(value *big.Rat) string { return thaifmt.Baht(value) }

func whole(value *big.Rat) string {
	text := value.FloatString(0)
	if strings.HasPrefix(text, "-") {
		return "-" + thaifmt.Group(text[1:])
	}
	return thaifmt.Group(text)
}

// percent writes one decimal place, dropping a trailing .0.
func percent(value *big.Rat) string {
	return strings.TrimSuffix(value.FloatString(1), ".0")
}

func asOf(collectedAt string, location *time.Location) string {
	if text := thaiDateTime(collectedAt, location); text != "" {
		return "\nข้อมูล ณ " + text
	}
	return ""
}
