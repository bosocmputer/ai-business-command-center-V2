package line

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

const (
	softFlexPayloadBytes    = 32 * 1024
	maximumFlexPayloadBytes = 40 * 1024
	FlexPresentationVersion = "ai-bcc-executive-report-v2"
)

// Colors and layout below mirror AI-Business Command-Center's
// buildExecutiveDigestFlexMessage "executive_report_v2" bubble exactly, so
// recipients see the same card style across both systems.
const (
	flexHeaderBackground    = "#F8FAFC"
	flexKickerColor         = "#2563EB"
	flexTitleColor          = "#111827"
	flexSubtitleColor       = "#6B7280"
	flexMutedColor          = "#6B7280"
	flexValueColor          = "#111827"
	flexActionColor         = "#2563EB"
	flexStatusReadyColor    = "#047857"
	flexStatusNoticeColor   = "#B45309"
	flexStatusCriticalColor = "#B42318"
)

var ErrFlexInputInvalid = errors.New("LINE Flex input is invalid")

type FlexReport struct {
	Key        report.Key
	Metrics    map[string]string
	Dashboard  *report.Dashboard
	Period     report.Period
	FinishedAt time.Time
	ActionURL  string
}

type FlexInput struct {
	TenantName  string
	Timezone    string
	Period      report.Period
	GeneratedAt time.Time
	ActionURL   string
	Reports     []FlexReport
}

type FlexRenderResult struct {
	Message             json.RawMessage
	PresentationVersion string
	PayloadBytes        int
	ReportCount         int
	ZeroReportCount     int
	MixedPeriods        bool
	Duration            time.Duration
}

func RenderFlex(input FlexInput) (json.RawMessage, error) {
	result, err := RenderFlexWithStats(input)
	return result.Message, err
}

func RenderFlexWithStats(input FlexInput) (result FlexRenderResult, err error) {
	startedAt := time.Now()
	defer func() { result.Duration = time.Since(startedAt) }()
	input.TenantName = strings.TrimSpace(input.TenantName)
	if input.TenantName == "" || utf8.RuneCountInString(input.TenantName) > 160 || len(input.Reports) < 1 || len(input.Reports) > 10 || input.GeneratedAt.IsZero() {
		return result, ErrFlexInputInvalid
	}
	overviewURL, err := validHTTPSURL(input.ActionURL)
	periodFrom, fromErr := time.Parse(time.DateOnly, input.Period.DateFrom)
	periodTo, toErr := time.Parse(time.DateOnly, input.Period.DateTo)
	if err != nil || fromErr != nil || toErr != nil || periodTo.Before(periodFrom) {
		return result, ErrFlexInputInvalid
	}
	timezone := strings.TrimSpace(input.Timezone)
	if timezone == "" {
		timezone = "Asia/Bangkok"
	}
	if timezone != "Asia/Bangkok" {
		return result, ErrFlexInputInvalid
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return result, ErrFlexInputInvalid
	}

	presentations := make([]FlexReportPresentation, 0, len(input.Reports))
	reportPeriods := make([]report.Period, 0, len(input.Reports))
	seen := make(map[report.Key]struct{}, len(input.Reports))
	for _, item := range input.Reports {
		if _, duplicate := seen[item.Key]; duplicate {
			return result, ErrFlexInputInvalid
		}
		seen[item.Key] = struct{}{}
		if strings.TrimSpace(item.ActionURL) == "" {
			item.ActionURL = overviewURL.String()
		}
		itemPeriod := item.Period
		if itemPeriod.DateFrom == "" && itemPeriod.DateTo == "" {
			itemPeriod = input.Period
		}
		itemFrom, itemFromErr := time.Parse(time.DateOnly, itemPeriod.DateFrom)
		itemTo, itemToErr := time.Parse(time.DateOnly, itemPeriod.DateTo)
		if itemFromErr != nil || itemToErr != nil || itemTo.Before(itemFrom) {
			return result, ErrFlexInputInvalid
		}
		reportURL, parseErr := validHTTPSURL(item.ActionURL)
		if parseErr != nil || !strings.EqualFold(reportURL.Host, overviewURL.Host) {
			return result, ErrFlexInputInvalid
		}
		presentation, presentationErr := BuildFlexReportPresentation(item)
		if presentationErr != nil {
			return result, presentationErr
		}
		presentations = append(presentations, presentation)
		reportPeriods = append(reportPeriods, itemPeriod)
	}
	mixedPeriods := false
	for _, itemPeriod := range reportPeriods[1:] {
		if itemPeriod != reportPeriods[0] {
			mixedPeriods = true
			break
		}
	}
	localGeneratedAt := input.GeneratedAt.In(location)
	// Compact on purpose: the status row is one line and truncates longer labels on narrow phones.
	generatedAtLabel := thaiShortDate(localGeneratedAt) + " " + localGeneratedAt.Format("15:04")
	summaryLabel := flexReportSummary(input.Period, input.Reports, mixedPeriods, localGeneratedAt)

	zeroReportCount := 0
	bubbles := make([]map[string]any, 0, len(presentations))
	for index, item := range presentations {
		if item.DataState == FlexDataZero {
			zeroReportCount++
		}
		subtitle := input.TenantName + " · " + periodLabel(reportPeriods[index])
		contextNote := flexContextNote(reportPeriods[index])
		if mixedPeriods {
			subtitle = input.TenantName + " · " + flexReportPeriodLabel(item.Key, reportPeriods[index], input.GeneratedAt.In(location))
			contextNote = ""
		}
		kicker := item.CategoryLabel + " · " + flexCadenceLabel(item.Key, reportPeriods[index])
		bubbles = append(bubbles, buildExecutiveReportBubble(item, kicker, subtitle, generatedAtLabel, contextNote))
	}

	altText := flexAltTextLabel(input.TenantName, summaryLabel, len(presentations))
	var contents any
	if len(bubbles) == 1 {
		contents = bubbles[0]
	} else {
		contents = map[string]any{"type": "carousel", "contents": bubbles}
	}
	message := map[string]any{
		"type":     "flex",
		"altText":  altText,
		"contents": contents,
	}
	payload, err := json.Marshal(message)
	if err != nil || len(payload) >= maximumFlexPayloadBytes {
		return result, ErrFlexInputInvalid
	}
	result.Message = payload
	result.PresentationVersion = FlexPresentationVersion
	result.PayloadBytes = len(payload)
	result.ReportCount = len(presentations)
	result.ZeroReportCount = zeroReportCount
	result.MixedPeriods = mixedPeriods
	return result, nil
}

// buildExecutiveReportBubble mirrors AI-BCC's buildExecutiveReportV2Bubble
// (packages/reports/src/line-flex.ts): header kicker/title/subtitle, a status
// + generated-at row, a primary amount with its unit, up to four supporting
// rows, then only the context an owner needs (comparison with its reference
// date, highlights, warnings). Everything else stays behind the button.
func buildExecutiveReportBubble(item FlexReportPresentation, kicker, subtitle, generatedAtLabel, contextNote string) map[string]any {
	statusText, statusColor := flexStatusFor(item)

	headerContents := []any{
		flexKickerText(kicker),
		map[string]any{"type": "text", "text": flexShortTitle(item.Label), "weight": "bold", "size": "lg", "color": flexTitleColor, "wrap": true, "maxLines": 2, "margin": "xs"},
		map[string]any{"type": "text", "text": subtitle, "size": "sm", "color": flexSubtitleColor, "margin": "sm", "wrap": true, "maxLines": 2},
	}

	bodyContents := []any{
		map[string]any{
			"type": "box", "layout": "horizontal", "spacing": "sm",
			"contents": []any{
				map[string]any{"type": "text", "text": statusText, "size": "xs", "weight": "bold", "color": statusColor, "flex": 0, "maxLines": 1},
				map[string]any{"type": "text", "text": "อัปเดต " + generatedAtLabel, "size": "xs", "color": flexMutedColor, "align": "end", "flex": 1, "maxLines": 1},
			},
		},
		flexPrimaryAmountBaseline(item.Primary.Value, item.Primary.Unit),
	}
	if item.DataState != FlexDataZero && len(item.Supporting) > 0 {
		metricRows := make([]any, 0, len(item.Supporting))
		for _, metric := range item.Supporting[:min(4, len(item.Supporting))] {
			metricRows = append(metricRows, flexMetricRow(metric.Label, withUnit(metric.Value, metric.Unit)))
		}
		bodyContents = append(bodyContents, map[string]any{"type": "box", "layout": "vertical", "spacing": "sm", "contents": metricRows})
	}

	details := make([]any, 0, 4)
	if item.DataState == FlexDataZero {
		details = append(details, flexInfoBlock("สิ่งที่ควรดู", item.StateText))
	}
	if item.Comparison != nil {
		details = append(details, flexInfoBlock("เทียบยอด", item.Comparison.Text))
	}
	for _, highlight := range item.Highlights {
		details = append(details, flexInfoBlock(highlight.Label, withUnit(highlight.Value, highlight.Unit)))
	}
	if note, tone := flexNoteFor(item); note != "" {
		details = append(details, flexNoteBlock(note, tone))
	}
	if contextNote != "" {
		details = append(details, flexNoteBlock(contextNote, "info"))
	}
	if len(details) > 0 {
		bodyContents = append(bodyContents, map[string]any{"type": "separator", "margin": "md"})
		bodyContents = append(bodyContents, details...)
	}

	return map[string]any{
		"type": "bubble", "size": "mega",
		"header": map[string]any{"type": "box", "layout": "vertical", "paddingAll": "16px", "backgroundColor": flexHeaderBackground, "contents": headerContents},
		"body":   map[string]any{"type": "box", "layout": "vertical", "paddingAll": "16px", "spacing": "sm", "contents": bodyContents},
		"footer": map[string]any{
			"type": "box", "layout": "vertical", "paddingAll": "16px",
			"contents": []any{map[string]any{
				"type": "button", "style": "primary", "color": flexActionColor, "height": "sm",
				"action": map[string]any{"type": "uri", "label": "เปิดรายละเอียด", "uri": item.ActionURL},
			}},
		},
	}
}

func flexKickerText(kicker string) map[string]any {
	return map[string]any{"type": "text", "text": truncateRunes(kicker, 40), "weight": "bold", "size": "xs", "color": flexKickerColor, "maxLines": 1}
}

func flexShortTitle(label string) string {
	if short := strings.TrimSpace(strings.TrimPrefix(label, "รายงาน")); short != "" {
		return short
	}
	return label
}

func flexCadenceLabel(key report.Key, period report.Period) string {
	if definition, ok := report.DefinitionFor(key); ok && definition.ParameterKind == report.CurrentOnly {
		return "ณ เวลาส่ง"
	}
	if period.DateFrom != "" && period.DateFrom == period.DateTo {
		return "รายวัน"
	}
	return "สะสม"
}

func withUnit(value, unit string) string {
	if unit == "" {
		return value
	}
	return value + " " + unit
}

func flexStatusFor(item FlexReportPresentation) (text string, color string) {
	if item.DataState == FlexDataZero {
		return "ไม่มีรายการ", flexStatusNoticeColor
	}
	if item.Attention != nil && item.Attention.Severity == FlexAttentionDanger {
		return "ควรตรวจสอบ", flexStatusCriticalColor
	}
	if item.Attention != nil && item.Attention.Severity == FlexAttentionWarning {
		return "ควรตรวจสอบ", flexStatusNoticeColor
	}
	return "พร้อมใช้", flexStatusReadyColor
}

func flexNoteFor(item FlexReportPresentation) (note string, tone string) {
	if item.Attention == nil {
		return "", ""
	}
	switch item.Attention.Severity {
	case FlexAttentionWarning, FlexAttentionDanger:
		return item.Attention.Text, "warning"
	case FlexAttentionInfo:
		if item.Comparison != nil {
			return "", ""
		}
		return item.Attention.Text, "neutral"
	default:
		return "", ""
	}
}

func flexNoteBlock(value, tone string) map[string]any {
	background, color := "#F8FAFC", "#475569"
	switch tone {
	case "warning":
		background, color = "#FFF7ED", "#9A3412"
	case "info":
		background, color = "#EFF6FF", "#1D4ED8"
	}
	return map[string]any{
		"type": "box", "layout": "vertical", "backgroundColor": background, "cornerRadius": "6px", "paddingAll": "8px", "margin": "md",
		"contents": []any{map[string]any{"type": "text", "text": truncateRunes(value, 96), "size": "xs", "color": color, "wrap": true, "maxLines": 3}},
	}
}

// flexPrimaryAmountBaseline follows AI-BCC's getPrimaryAmountSizes: the unit is
// rendered smaller beside the number, and long numbers step down a size.
func flexPrimaryAmountBaseline(value, unit string) map[string]any {
	valueSize, unitSize := "xl", "md"
	if utf8.RuneCountInString(strings.ReplaceAll(value, " ", "")) >= 14 {
		valueSize, unitSize = "lg", "sm"
	}
	contents := []any{
		map[string]any{"type": "text", "text": value, "weight": "bold", "size": valueSize, "color": flexValueColor, "flex": 0, "wrap": true, "maxLines": 2},
	}
	if unit != "" {
		contents = append(contents, map[string]any{"type": "text", "text": unit, "weight": "bold", "size": unitSize, "color": flexValueColor, "flex": 1, "wrap": true, "maxLines": 1})
	}
	return map[string]any{"type": "box", "layout": "baseline", "spacing": "sm", "contents": contents}
}

func flexMetricRow(label, value string) map[string]any {
	return map[string]any{
		"type": "box", "layout": "horizontal",
		"contents": []any{
			map[string]any{"type": "text", "text": label, "size": "sm", "color": flexMutedColor, "flex": 2, "wrap": true, "maxLines": 2},
			map[string]any{"type": "text", "text": truncateRunes(value, 42), "size": "sm", "color": flexValueColor, "align": "end", "weight": "bold", "flex": 3, "wrap": true, "maxLines": 2},
		},
	}
}

func flexInfoBlock(label, value string) map[string]any {
	return map[string]any{
		"type": "box", "layout": "vertical", "spacing": "xs",
		"contents": []any{
			map[string]any{"type": "text", "text": label, "size": "xs", "color": flexMutedColor, "weight": "bold", "maxLines": 1},
			map[string]any{"type": "text", "text": truncateRunes(value, 96), "size": "sm", "color": flexValueColor, "wrap": true, "maxLines": 3},
		},
	}
}

func truncateRunes(value string, maxLength int) string {
	if utf8.RuneCountInString(value) <= maxLength {
		return value
	}
	runes := []rune(value)
	return string(runes[:max(0, maxLength-1)]) + "…"
}

func flexPeriodSummary(period report.Period, mixed bool) string {
	if mixed {
		return "ช่วงข้อมูลแตกต่างตามรายงาน"
	}
	return periodLabel(period)
}

func flexReportSummary(period report.Period, reports []FlexReport, mixed bool, generatedAt time.Time) string {
	if mixed {
		return flexPeriodSummary(period, true)
	}
	if len(reports) == 1 {
		if definition, ok := report.DefinitionFor(reports[0].Key); ok && definition.ParameterKind == report.CurrentOnly {
			return "สถานะ ณ เวลาส่ง " + thaiShortDate(generatedAt) + " · " + generatedAt.Format("15:04") + " น."
		}
	}
	return periodLabel(period)
}

func flexReportPeriodLabel(key report.Key, period report.Period, generatedAt time.Time) string {
	definition, ok := report.DefinitionFor(key)
	if ok && definition.ParameterKind == report.CurrentOnly {
		return "สถานะ ณ เวลาส่ง " + thaiShortDate(generatedAt) + " · " + generatedAt.Format("15:04") + " น."
	}
	return periodLabel(period)
}

func validHTTPSURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, ErrFlexInputInvalid
	}
	return parsed, nil
}

func periodLabel(period report.Period) string {
	from, fromErr := time.Parse(time.DateOnly, period.DateFrom)
	to, toErr := time.Parse(time.DateOnly, period.DateTo)
	if fromErr != nil || toErr != nil || to.Before(from) {
		return "ข้อมูล " + period.DateFrom + " ถึง " + period.DateTo
	}
	if from.Equal(to) {
		return "ข้อมูล ณ " + thaiShortDate(to)
	}
	if from.Year() == to.Year() && from.Month() == to.Month() {
		return "ข้อมูล " + strconv.Itoa(from.Day()) + "–" + thaiShortDate(to)
	}
	if from.Year() == to.Year() {
		return "ข้อมูล " + thaiDayMonth(from) + "–" + thaiShortDate(to)
	}
	return "ข้อมูล " + thaiShortDate(from) + "–" + thaiShortDate(to)
}

var thaiShortMonths = [...]string{"ม.ค.", "ก.พ.", "มี.ค.", "เม.ย.", "พ.ค.", "มิ.ย.", "ก.ค.", "ส.ค.", "ก.ย.", "ต.ค.", "พ.ย.", "ธ.ค."}

func thaiDayMonth(value time.Time) string {
	return strconv.Itoa(value.Day()) + " " + thaiShortMonths[value.Month()-1]
}

func thaiShortDate(value time.Time) string {
	return thaiDayMonth(value) + " " + strconv.Itoa(value.Year()+543)
}

func runeCountLabel(count int) string { return strconv.Itoa(count) + " รายงาน" }

func flexAltText(tenantName string, period report.Period, reportCount int) string {
	return flexAltTextWithMode(tenantName, period, reportCount, false)
}

func flexAltTextWithMode(tenantName string, period report.Period, reportCount int, mixed bool) string {
	return flexAltTextLabel(tenantName, flexPeriodSummary(period, mixed), reportCount)
}

func flexAltTextLabel(tenantName, summaryLabel string, reportCount int) string {
	text := "สรุปผู้บริหาร " + tenantName + ": " + summaryLabel + " (" + runeCountLabel(reportCount) + ")"
	if utf8.RuneCountInString(text) <= 400 {
		return text
	}
	runes := []rune(text)
	return string(runes[:397]) + "..."
}

func flexContextNote(period report.Period) string {
	if period.Preset == report.TodayToNow {
		return "วันนี้ยังไม่มีช่วงเวลาเปรียบเทียบที่เท่ากัน"
	}
	return ""
}
