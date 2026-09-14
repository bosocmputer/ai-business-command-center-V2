package line

import (
	"math/big"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

type FlexMetricPresentation struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit,omitempty"`
}

type FlexComparisonPresentation struct {
	Text      string                     `json:"text"`
	Direction report.ComparisonDirection `json:"direction"`
}

type FlexAttentionSeverity string

const (
	FlexAttentionInfo    FlexAttentionSeverity = "INFO"
	FlexAttentionWarning FlexAttentionSeverity = "WARNING"
	FlexAttentionDanger  FlexAttentionSeverity = "DANGER"
)

type FlexAttentionPresentation struct {
	Severity FlexAttentionSeverity `json:"severity"`
	Text     string                `json:"text"`
}

type FlexDataState string

const (
	FlexDataData FlexDataState = "DATA"
	FlexDataZero FlexDataState = "ZERO"
)

type FlexReportPresentation struct {
	Key           report.Key                  `json:"key"`
	Label         string                      `json:"label"`
	CategoryLabel string                      `json:"categoryLabel"`
	Primary       FlexMetricPresentation      `json:"primary"`
	Supporting    []FlexMetricPresentation    `json:"supporting"`
	Comparison    *FlexComparisonPresentation `json:"comparison,omitempty"`
	Attention     *FlexAttentionPresentation  `json:"attention,omitempty"`
	Highlights    []FlexMetricPresentation    `json:"highlights,omitempty"`
	DataState     FlexDataState               `json:"dataState"`
	StateText     string                      `json:"stateText,omitempty"`
	ActionURL     string                      `json:"actionUrl"`
}

// Executive wording for LINE cards; the dashboard's generic labels stay unchanged elsewhere.
var flexLabelOverrides = map[report.Key]map[string]string{
	report.SalesGoodsServices:    {"document_count": "บิลขาย", "average_per_document": "ยอดเฉลี่ยต่อบิล"},
	report.PurchaseGoodsPayables: {"document_count": "เอกสารซื้อ"},
	report.CashBankReceipts:      {"document_count": "เอกสาร"},
	report.CashBankPayments:      {"document_count": "เอกสาร"},
}

var flexCountUnits = map[string]string{
	"document_count":              "ใบ",
	"receipt_count":               "ใบ",
	"payment_split_missing_count": "ใบ",
	"item_count":                  "รายการ",
	"reorder_item_count":          "รายการ",
	"customer_count":              "ราย",
}

const (
	flexHighlightNameLimit = 38
	flexChannelLimit       = 3
)

type flexPresentationDefinition struct {
	primary    string
	supporting []string
	zeroText   string
}

var flexPresentationDefinitions = map[report.Key]flexPresentationDefinition{
	report.SalesGoodsServices:      {primary: "total_amount", supporting: []string{"document_count", "average_per_document"}, zeroText: "ไม่มีรายการขายในช่วงนี้"},
	report.PurchaseGoodsPayables:   {primary: "total_amount", supporting: []string{"document_count", "average_per_document"}, zeroText: "ไม่มีรายการซื้อในช่วงนี้"},
	report.GrossProfitByProduct:    {primary: "gross_profit_amount", supporting: []string{"gross_margin_percent", "net_amount"}, zeroText: "ไม่มีรายการขายสำหรับคำนวณกำไร"},
	report.GrossProfitByARCustomer: {primary: "gross_profit_amount", supporting: []string{"gross_margin_percent", "net_amount"}, zeroText: "ไม่มีรายการขายสำหรับคำนวณกำไร"},
	report.StockBalance:            {primary: "balance_amount", supporting: []string{"item_count"}, zeroText: "ไม่พบสินค้าคงเหลือ"},
	report.StockReorder:            {primary: "reorder_item_count", supporting: []string{"shortage_qty"}, zeroText: "ไม่มีสินค้าต่ำกว่าจุดสั่งซื้อ"},
	report.ARCustomerMovement:      {primary: "net_movement_amount", supporting: []string{"customer_count"}, zeroText: "ไม่มีความเคลื่อนไหวลูกหนี้"},
	report.ARDebtReceipt:           {primary: "total_received_amount", supporting: []string{"receipt_count", "average_per_receipt"}, zeroText: "ไม่มีรายการรับชำระหนี้"},
	report.CashBankReceipts:        {primary: "total_amount", supporting: []string{"document_count", "average_per_document"}, zeroText: "ไม่มีรายการรับเงิน"},
	report.CashBankPayments:        {primary: "total_amount", supporting: []string{"document_count", "average_per_document"}, zeroText: "ไม่มีรายการจ่ายเงิน"},
}

func BuildFlexReportPresentation(input FlexReport) (FlexReportPresentation, error) {
	definition, ok := report.DefinitionFor(input.Key)
	presentationDefinition, configured := flexPresentationDefinitions[input.Key]
	if !ok || !configured {
		return FlexReportPresentation{}, ErrFlexInputInvalid
	}
	presentation := FlexReportPresentation{
		Key: input.Key, Label: definition.LabelTH, CategoryLabel: definition.CategoryLabelTH,
		Supporting: []FlexMetricPresentation{}, DataState: FlexDataData, ActionURL: input.ActionURL,
	}

	if input.Dashboard == nil {
		metric, metricOK := legacyMetric(definition, presentationDefinition.primary, input.Metrics)
		if !metricOK {
			return FlexReportPresentation{}, ErrFlexInputInvalid
		}
		formatted, err := presentMetric(input.Key, metric)
		if err != nil {
			return FlexReportPresentation{}, err
		}
		presentation.Primary = formatted
		for _, key := range presentationDefinition.supporting {
			metric, exists := legacyMetric(definition, key, input.Metrics)
			if !exists {
				continue
			}
			formatted, err := presentMetric(input.Key, metric)
			if err != nil {
				return FlexReportPresentation{}, err
			}
			presentation.Supporting = append(presentation.Supporting, formatted)
		}
		presentation.Attention = &FlexAttentionPresentation{Severity: FlexAttentionInfo, Text: "ไม่มีข้อมูลเปรียบเทียบ"}
		return presentation, nil
	}
	if input.Dashboard.ReportKey != input.Key {
		return FlexReportPresentation{}, ErrFlexInputInvalid
	}

	metrics := make(map[string]report.DashboardMetric, len(input.Dashboard.KPIs))
	for _, metric := range input.Dashboard.KPIs {
		metrics[metric.Key] = metric
	}
	primary, exists := metrics[presentationDefinition.primary]
	if !exists {
		return FlexReportPresentation{}, ErrFlexInputInvalid
	}
	formattedPrimary, err := presentMetric(input.Key, primary)
	if err != nil {
		return FlexReportPresentation{}, err
	}
	presentation.Primary = formattedPrimary
	presentation.Comparison, err = presentComparison(primary, input.Dashboard.ComparisonPeriod)
	if err != nil {
		return FlexReportPresentation{}, err
	}
	for _, key := range presentationDefinition.supporting {
		metric, exists := metrics[key]
		if !exists {
			return FlexReportPresentation{}, ErrFlexInputInvalid
		}
		formatted, err := presentMetric(input.Key, metric)
		if err != nil {
			return FlexReportPresentation{}, err
		}
		presentation.Supporting = append(presentation.Supporting, formatted)
	}
	presentation.Attention = attentionFor(input.Key, metrics, input.Dashboard.Visualizations, input.Dashboard.Quality)
	if trustedZeroDashboard(input.Dashboard, presentationDefinition, metrics) {
		presentation.DataState = FlexDataZero
		presentation.StateText = presentationDefinition.zeroText
		if !comparisonHasChange(primary.Comparison) {
			presentation.Comparison = nil
		}
		return presentation, nil
	}
	if err := addExecutiveHighlights(&presentation, primary, input.Dashboard.Visualizations); err != nil {
		return FlexReportPresentation{}, err
	}
	return presentation, nil
}

// addExecutiveHighlights adds the one-line context an owner scans first: the
// top product or supplier, and the cash/transfer split for cash-bank reports.
// Loss rankings are deliberately never named here.
func addExecutiveHighlights(presentation *FlexReportPresentation, primary report.DashboardMetric, visualizations []report.DashboardVisualization) error {
	switch presentation.Key {
	case report.SalesGoodsServices:
		name, amount, ok := topRanked(visualizations, "top_products")
		if !ok {
			return nil
		}
		formatted, err := formatMetricValue(amount, report.UnitTHB)
		if err != nil {
			return ErrFlexInputInvalid
		}
		presentation.Highlights = append(presentation.Highlights, FlexMetricPresentation{Label: "สินค้าขายดี", Value: truncateRunes(name, flexHighlightNameLimit) + ": " + formatted + " บาท"})
	case report.PurchaseGoodsPayables:
		name, amount, ok := topRanked(visualizations, "top_suppliers")
		if !ok {
			return nil
		}
		formatted, err := formatMetricValue(amount, report.UnitTHB)
		if err != nil {
			return ErrFlexInputInvalid
		}
		value := truncateRunes(name, flexHighlightNameLimit) + ": " + formatted + " บาท"
		if share, ok := sharePercent(amount, primary.Value); ok {
			value += " (" + share + "% ของยอดซื้อ)"
		}
		presentation.Highlights = append(presentation.Highlights, FlexMetricPresentation{Label: "ผู้จำหน่ายหลัก", Value: value})
	case report.CashBankReceipts, report.CashBankPayments:
		key := "cash_receipt_methods"
		if presentation.Key == report.CashBankPayments {
			key = "cash_payment_methods"
		}
		channels, err := channelRows(visualizations, key)
		if err != nil || len(channels) == 0 {
			return err
		}
		documents := make([]FlexMetricPresentation, 0, 1+len(channels))
		for _, item := range presentation.Supporting {
			if item.Unit == "ใบ" {
				documents = append(documents, item)
			}
		}
		presentation.Supporting = append(documents, channels...)
	}
	return nil
}

func topRanked(visualizations []report.DashboardVisualization, key string) (string, string, bool) {
	for _, item := range visualizations {
		if item.Key != key || len(item.Categories) == 0 || len(item.Series) == 0 || len(item.Series[0].Values) == 0 {
			continue
		}
		name := strings.TrimSpace(item.Categories[0])
		if name == "" {
			return "", "", false
		}
		return name, item.Series[0].Values[0], true
	}
	return "", "", false
}

func channelRows(visualizations []report.DashboardVisualization, key string) ([]FlexMetricPresentation, error) {
	for _, item := range visualizations {
		if item.Key != key || len(item.Series) == 0 {
			continue
		}
		rows := make([]FlexMetricPresentation, 0, flexChannelLimit)
		for index, label := range item.Categories {
			if index >= len(item.Series[0].Values) || len(rows) == flexChannelLimit {
				break
			}
			formatted, err := formatMetricValue(item.Series[0].Values[index], report.UnitTHB)
			if err != nil {
				return nil, ErrFlexInputInvalid
			}
			rows = append(rows, FlexMetricPresentation{Label: label, Value: formatted, Unit: "บาท"})
		}
		return rows, nil
	}
	return nil, nil
}

func sharePercent(part, total string) (string, bool) {
	partValue, partOK := new(big.Rat).SetString(normalizeNumber(part))
	totalValue, totalOK := new(big.Rat).SetString(normalizeNumber(total))
	if !partOK || !totalOK || totalValue.Sign() <= 0 {
		return "", false
	}
	share := new(big.Rat).Quo(partValue, totalValue)
	share.Mul(share, big.NewRat(100, 1))
	return share.FloatString(1), true
}

func trustedZeroDashboard(dashboard *report.Dashboard, definition flexPresentationDefinition, metrics map[string]report.DashboardMetric) bool {
	if dashboard == nil || dashboard.Quality.Status != "OK" || len(dashboard.Quality.Warnings) != 0 {
		return false
	}
	keys := append([]string{definition.primary}, definition.supporting...)
	for _, key := range keys {
		metric, exists := metrics[key]
		if !exists {
			return false
		}
		number, valid := new(big.Rat).SetString(normalizeNumber(metric.Value))
		if !valid || number.Sign() != 0 {
			return false
		}
	}
	return true
}

func comparisonHasChange(comparison report.MetricComparison) bool {
	if comparison.Availability != report.ComparisonAvailable {
		return false
	}
	for _, value := range []string{comparison.Percent, comparison.Delta} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		number, valid := new(big.Rat).SetString(normalizeNumber(value))
		if valid && number.Sign() != 0 {
			return true
		}
	}
	return false
}

func legacyMetric(definition report.Definition, key string, metrics map[string]string) (report.DashboardMetric, bool) {
	value, exists := metrics[key]
	if !exists || strings.TrimSpace(value) == "" {
		return report.DashboardMetric{}, false
	}
	label := key
	for _, item := range definition.LineMetrics {
		if item.Key == key {
			label = item.LabelTH
			break
		}
	}
	return report.DashboardMetric{Key: key, Label: label, Value: value, Unit: unitForMetricKey(key)}, true
}

func unitForMetricKey(key string) report.MetricUnit {
	if strings.Contains(key, "percent") {
		return report.UnitPercent
	}
	if strings.Contains(key, "count") {
		return report.UnitCount
	}
	if strings.Contains(key, "qty") {
		return report.UnitQuantity
	}
	return report.UnitTHB
}

func presentMetric(key report.Key, metric report.DashboardMetric) (FlexMetricPresentation, error) {
	value, err := formatMetricValue(metric.Value, metric.Unit)
	if err != nil {
		return FlexMetricPresentation{}, ErrFlexInputInvalid
	}
	label := metric.Label
	if override := flexLabelOverrides[key][metric.Key]; override != "" {
		label = override
	}
	return FlexMetricPresentation{Label: label, Value: value, Unit: flexUnitLabel(metric)}, nil
}

func flexUnitLabel(metric report.DashboardMetric) string {
	switch metric.Unit {
	case report.UnitTHB:
		return "บาท"
	case report.UnitCount:
		return flexCountUnits[metric.Key]
	default:
		return ""
	}
}

func presentComparison(metric report.DashboardMetric, comparisonPeriod report.Period) (*FlexComparisonPresentation, error) {
	comparison := metric.Comparison
	if comparison.Availability != report.ComparisonAvailable {
		return nil, nil
	}
	arrow := "→"
	if comparison.Direction == report.DirectionUp {
		arrow = "↑"
	} else if comparison.Direction == report.DirectionDown {
		arrow = "↓"
	}
	value := comparison.Percent
	unit := report.UnitPercent
	if strings.TrimSpace(value) == "" {
		value, unit = comparison.Delta, metric.Unit
	}
	formatted, err := formatMetricValue(value, unit)
	if err != nil {
		return nil, ErrFlexInputInvalid
	}
	formatted = strings.TrimPrefix(strings.TrimPrefix(formatted, "−"), "-")
	reference := comparisonReferenceLabel(comparisonPeriod)
	if reference == "" {
		return &FlexComparisonPresentation{Text: arrow + " " + formatted + " จากช่วงก่อน", Direction: comparison.Direction}, nil
	}
	text := arrow + " " + formatted + " เทียบ " + reference
	if previous, err := formatMetricValue(comparison.PreviousValue, metric.Unit); err == nil && strings.TrimSpace(comparison.PreviousValue) != "" {
		if unit := flexUnitLabel(metric); unit != "" {
			previous += " " + unit
		}
		text += " (" + previous + ")"
	}
	return &FlexComparisonPresentation{Text: text, Direction: comparison.Direction}, nil
}

// comparisonReferenceLabel names the compared window, e.g. "12 ก.ย. 2569",
// so a percentage is never shown without saying what it is relative to.
func comparisonReferenceLabel(period report.Period) string {
	if period.DateFrom == "" || period.DateTo == "" {
		return ""
	}
	from, fromErr := time.Parse(time.DateOnly, period.DateFrom)
	to, toErr := time.Parse(time.DateOnly, period.DateTo)
	if fromErr != nil || toErr != nil || to.Before(from) {
		return ""
	}
	return strings.TrimPrefix(strings.TrimPrefix(periodLabel(period), "ข้อมูล ณ "), "ข้อมูล ")
}

func attentionFor(key report.Key, metrics map[string]report.DashboardMetric, visualizations []report.DashboardVisualization, quality report.DashboardQuality) *FlexAttentionPresentation {
	if key == report.GrossProfitByProduct || key == report.GrossProfitByARCustomer {
		if metricSign(metrics["gross_profit_amount"].Value) < 0 {
			return &FlexAttentionPresentation{Severity: FlexAttentionDanger, Text: "ขาดทุนขั้นต้น"}
		}
		visualizationKey, label := "loss_products", "พบสินค้าที่ขาดทุน"
		if key == report.GrossProfitByARCustomer {
			visualizationKey, label = "loss_customers", "พบลูกค้าที่ขาดทุน"
		}
		if hasVisualizationData(visualizations, visualizationKey) {
			return &FlexAttentionPresentation{Severity: FlexAttentionWarning, Text: label}
		}
	}
	if key == report.StockReorder && metricSign(metrics["reorder_item_count"].Value) > 0 {
		return &FlexAttentionPresentation{Severity: FlexAttentionWarning, Text: "ต่ำกว่าจุดสั่งซื้อ"}
	}
	if key == report.ARDebtReceipt && metricSign(metrics["payment_split_missing_count"].Value) > 0 {
		return &FlexAttentionPresentation{Severity: FlexAttentionWarning, Text: "ข้อมูลวิธีรับชำระไม่ครบ"}
	}
	for _, warning := range quality.Warnings {
		if warning == "COMPARISON_QUERY_FAILED" {
			return &FlexAttentionPresentation{Severity: FlexAttentionInfo, Text: "ข้อมูลเปรียบเทียบไม่พร้อม"}
		}
	}
	return nil
}

func hasVisualizationData(items []report.DashboardVisualization, key string) bool {
	for _, item := range items {
		if item.Key == key && len(item.Categories) > 0 && len(item.Series) > 0 {
			return true
		}
	}
	return false
}

func metricSign(value string) int {
	number, ok := new(big.Rat).SetString(normalizeNumber(value))
	if !ok {
		return 0
	}
	return number.Sign()
}

func formatMetricValue(value string, unit report.MetricUnit) (string, error) {
	number, ok := new(big.Rat).SetString(normalizeNumber(value))
	if !ok {
		return "", ErrFlexInputInvalid
	}
	digits := 2
	if unit == report.UnitCount {
		digits = 0
	}
	raw := number.FloatString(digits)
	negative := strings.HasPrefix(raw, "-")
	raw = strings.TrimPrefix(raw, "-")
	parts := strings.SplitN(raw, ".", 2)
	parts[0] = groupThousands(parts[0])
	formatted := strings.Join(parts, ".")
	suffix := ""
	if unit == report.UnitPercent {
		suffix = "%"
	}
	if negative {
		return "−" + formatted + suffix, nil
	}
	return formatted + suffix, nil
}

func normalizeNumber(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), ",", ""), "−", "-")
}

func groupThousands(value string) string {
	if len(value) <= 3 {
		return value
	}
	first := len(value) % 3
	if first == 0 {
		first = 3
	}
	var result strings.Builder
	result.WriteString(value[:first])
	for index := first; index < len(value); index += 3 {
		result.WriteByte(',')
		result.WriteString(value[index : index+3])
	}
	return result.String()
}
