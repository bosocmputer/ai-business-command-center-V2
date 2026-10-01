package report

import (
	"fmt"
	"math/big"
	"strconv"
)

// Purchase frequency shows how often each customer buys and whether one has gone
// quiet for longer than usual. A customer's rhythm is the average number of days
// between purchase days in the period: (last purchase day - first purchase day)
// divided by (purchase days - 1). Several code 44 bills on the same day are one
// purchase day, because one delivery round is not a shorter rhythm. Documents are
// counted as the sales report counts them (cancelled, copy and POS bills with a
// reference are left out).
//
// Each customer also carries the amount of their code 44 bills in the period, so
// the customers who have gone quiet can be ranked by what they used to buy.
//
// Days since the last purchase are measured to the last day of the period. A
// customer is flagged only when that is at least seven days, so a customer who
// buys every day or two is not called late after a weekend:
//
//   - more than twice the usual rhythm: OVERDUE
//   - more than one and a half times: LATE
//   - otherwise ON_TRACK
//
// A customer with a single purchase day has no rhythm to compare with and is
// listed as SINGLE, so the report accounts for every customer who bought. See
// docs/sml/numbers-dictionary.md.
//
// The query is not chunked: it returns one row per customer and a chunk would
// need a customer key and a merge that nothing needs yet. A shop that fails on
// size gets the failure with its parser evidence, not a retry as a chunked run.

const (
	frequencyStatusOverdue = "OVERDUE"
	frequencyStatusLate    = "LATE"
	frequencyStatusOnTrack = "ON_TRACK"
	frequencyStatusSingle  = "SINGLE"
	// frequencySummaryRowsText is the number of customers a summary ranks.
	frequencySummaryRowsText = "10"
)

// frequencyStatuses lists the statuses in the order they are shown.
var frequencyStatuses = []struct{ code, label string }{
	{frequencyStatusOverdue, "เงียบเกินรอบมาก"},
	{frequencyStatusLate, "ช้ากว่ารอบ"},
	{frequencyStatusOnTrack, "ซื้อตามรอบ"},
	{frequencyStatusSingle, "ซื้อครั้งเดียว"},
}

// purchaseFrequencyBaseSQL produces one row per customer who bought in the
// period. $1 is the first day and $2 the last day.
const purchaseFrequencyBaseSQL = `
purchase_days as (
  select t.cust_code, t.doc_date, count(*) as documents, sum(coalesce(t.total_amount, 0)) as day_amount
  from ic_trans t
  where t.trans_flag = 44
    and coalesce(t.last_status, 0) = 0
    and t.doc_date between $1::date and $2::date
    and coalesce(t.cust_code, '') <> ''
    and coalesce(t.is_doc_copy, 0) <> 1
    and (coalesce(t.doc_ref, '') = '' or coalesce(t.is_pos, 0) = 0)
  group by t.cust_code, t.doc_date
),
per_customer as (
  select p.cust_code, count(*) as purchase_days, sum(p.documents) as document_count, sum(p.day_amount) as sales_amount,
    min(p.doc_date) as first_purchase_date, max(p.doc_date) as last_purchase_date
  from purchase_days p
  group by p.cust_code
),
cadence as (
  select c.*, c.last_purchase_date - c.first_purchase_date as span_days,
    $2::date - c.last_purchase_date as days_since_last,
    case when c.purchase_days >= 2
      then (c.last_purchase_date - c.first_purchase_date)::numeric / (c.purchase_days - 1) end as gap
  from per_customer c
),
classified as (
  select c.cust_code, c.purchase_days, c.document_count, c.sales_amount, c.first_purchase_date, c.last_purchase_date,
    c.span_days, c.days_since_last,
    round(c.gap, 2) as avg_gap_days,
    case when c.gap is null then null else round(greatest(c.days_since_last - c.gap, 0), 2) end as days_late,
    case when c.gap is null then null else round(c.days_since_last / c.gap, 2) end as gap_ratio,
    case
      when c.gap is null then 'SINGLE'
      when c.days_since_last >= 7 and c.days_since_last > 2 * c.gap then 'OVERDUE'
      when c.days_since_last >= 7 and c.days_since_last > 1.5 * c.gap then 'LATE'
      else 'ON_TRACK'
    end as status
  from cadence c
)`

const purchaseFrequencyStatusLabelSQL = `
  case %s
    when 'OVERDUE' then 'เงียบเกินรอบมาก'
    when 'LATE' then 'ช้ากว่ารอบ'
    when 'ON_TRACK' then 'ซื้อตามรอบ'
    else 'ซื้อครั้งเดียว'
  end`

func purchaseFrequencyStatusLabel(column string) string {
	return fmt.Sprintf(purchaseFrequencyStatusLabelSQL, column)
}

var purchaseFrequencySQL = `
with` + purchaseFrequencyBaseSQL + `
select g.cust_code, coalesce(c.name_1, '') as cust_name, g.purchase_days, g.document_count, g.sales_amount,
  g.first_purchase_date, g.last_purchase_date, g.span_days, g.avg_gap_days, g.days_since_last,
  g.days_late, g.gap_ratio, g.status,` + purchaseFrequencyStatusLabel("g.status") + ` as status_label
from classified g
left join ar_customer c on c.code = g.cust_code
order by case g.status when 'OVERDUE' then 0 when 'LATE' then 1 when 'ON_TRACK' then 2 else 3 end,
  g.sales_amount desc, g.cust_code
`

// purchaseFrequencySummarySQL returns bounded rows: one row per status, the ten
// customers furthest behind their rhythm, and the metric columns on every row.
var purchaseFrequencySummarySQL = `
with` + purchaseFrequencyBaseSQL + `,
summary_metrics as (
  select count(*) as _metric_row_count,
    count(*) filter (where status <> 'SINGLE') as _metric_customer_count,
    count(*) filter (where status = 'SINGLE') as _metric_single_count,
    count(*) filter (where status = 'OVERDUE') as _metric_overdue_count,
    coalesce(sum(sales_amount) filter (where status = 'OVERDUE'), 0) as _metric_overdue_amount,
    count(*) filter (where status = 'LATE') as _metric_late_count,
    count(*) filter (where status = 'ON_TRACK') as _metric_on_track_count,
    coalesce(round(sum(span_days)::numeric / nullif(sum(purchase_days - 1), 0), 2), 0) as _metric_average_gap_days
  from classified
),
status_rows as (
  select 'statuses'::text as _summary_kind, g.status, ` + purchaseFrequencyStatusLabel("g.status") + ` as status_label,
    null::text as cust_code, null::text as cust_name,
    count(*) as customer_count, null::bigint as purchase_days, null::numeric as avg_gap_days,
    null::integer as days_since_last, null::numeric as days_late, sum(g.sales_amount) as sales_amount
  from classified g
  group by g.status
),
late_rows as (
  select 'ranking'::text as _summary_kind, g.status, ` + purchaseFrequencyStatusLabel("g.status") + ` as status_label,
    g.cust_code, coalesce(c.name_1, '') as cust_name,
    1::bigint as customer_count, g.purchase_days, g.avg_gap_days, g.days_since_last, g.days_late, g.sales_amount
  from classified g
  left join ar_customer c on c.code = g.cust_code
  where g.status in ('OVERDUE', 'LATE')
  order by g.sales_amount desc, g.cust_code
  limit ` + frequencySummaryRowsText + `
),
selected_rows as (
  select * from status_rows
  union all
  select * from late_rows
)
select selected_rows.*, summary_metrics.*,
  (selected_rows._summary_kind is null)::text as _summary_metric_row
from summary_metrics left join selected_rows on true
limit 30
`

// frequencyTotals are the figures every projection must be able to give.
type frequencyTotals struct {
	customers, singles, rows int
	byStatus                 map[string]int
	averageGap               *big.Rat
	overdueAmount            *big.Rat
}

func newFrequencyTotals() *frequencyTotals {
	totals := &frequencyTotals{byStatus: map[string]int{}, averageGap: new(big.Rat), overdueAmount: new(big.Rat)}
	for _, status := range frequencyStatuses {
		totals.byStatus[status.code] = 0
	}
	return totals
}

// frequencyTotalsFromSteps reads the totals from a summary projection, or adds
// them up from per-customer rows.
func frequencyTotalsFromSteps(steps map[string][]map[string]string) (*frequencyTotals, error) {
	rows := steps["rows"]
	if _, summary := summaryMetric(steps, "customer_count"); summary {
		return frequencyTotalsFromSummary(steps, rows)
	}
	return frequencyTotalsFromDetail(realSummaryRows(rows))
}

func frequencyTotalsFromSummary(steps map[string][]map[string]string, rows []map[string]string) (*frequencyTotals, error) {
	totals := newFrequencyTotals()
	average, err := decimal(summaryMetricOr(steps, "average_gap_days", "0"))
	if err != nil {
		return nil, fieldDecimalError("_metric_average_gap_days", err)
	}
	totals.averageGap = average
	overdue, err := decimal(summaryMetricOr(steps, "overdue_amount", "0"))
	if err != nil {
		return nil, fieldDecimalError("_metric_overdue_amount", err)
	}
	totals.overdueAmount = overdue
	totals.customers, _ = strconv.Atoi(integerText(summaryMetricOr(steps, "customer_count", "0")))
	totals.singles, _ = strconv.Atoi(integerText(summaryMetricOr(steps, "single_count", "0")))
	totals.rows, _ = strconv.Atoi(integerText(summaryMetricOr(steps, "row_count", "0")))
	for _, row := range rowsForSummaryKind(rows, "statuses") {
		if _, known := totals.byStatus[row["status"]]; !known {
			return nil, fmt.Errorf("purchase frequency status %q is not defined", row["status"])
		}
		count, err := strconv.Atoi(integerText(row["customer_count"]))
		if err != nil {
			return nil, fmt.Errorf("report field customer_count is invalid: %w", err)
		}
		totals.byStatus[row["status"]] += count
	}
	return totals, nil
}

func frequencyTotalsFromDetail(rows []map[string]string) (*frequencyTotals, error) {
	totals := newFrequencyTotals()
	span, repeats := new(big.Rat), new(big.Rat)
	for _, row := range rows {
		status := row["status"]
		if _, known := totals.byStatus[status]; !known {
			return nil, fmt.Errorf("purchase frequency status %q is not defined", status)
		}
		totals.byStatus[status]++
		totals.rows++
		if status == frequencyStatusOverdue {
			amount, err := decimal(row["sales_amount"])
			if err != nil {
				return nil, fieldDecimalError("sales_amount", err)
			}
			totals.overdueAmount.Add(totals.overdueAmount, amount)
		}
		if status == frequencyStatusSingle {
			totals.singles++
			continue
		}
		totals.customers++
		spanDays, err := decimal(row["span_days"])
		if err != nil {
			return nil, fieldDecimalError("span_days", err)
		}
		purchaseDays, err := decimal(row["purchase_days"])
		if err != nil {
			return nil, fieldDecimalError("purchase_days", err)
		}
		span.Add(span, spanDays)
		repeats.Add(repeats, purchaseDays.Sub(purchaseDays, big.NewRat(1, 1)))
	}
	if repeats.Sign() > 0 {
		totals.averageGap = new(big.Rat).Quo(span, repeats)
	}
	return totals, nil
}

func (totals *frequencyTotals) metrics() map[string]string {
	return map[string]string{
		"customer_count":   strconv.Itoa(totals.customers),
		"single_count":     strconv.Itoa(totals.singles),
		"overdue_count":    strconv.Itoa(totals.byStatus[frequencyStatusOverdue]),
		"overdue_amount":   money(totals.overdueAmount),
		"late_count":       strconv.Itoa(totals.byStatus[frequencyStatusLate]),
		"on_track_count":   strconv.Itoa(totals.byStatus[frequencyStatusOnTrack]),
		"average_gap_days": money(totals.averageGap),
	}
}

// buildFrequencyVisualizations draws how many customers are in each status and
// the ten customers furthest behind their own rhythm, whichever projection the
// rows came from.
func buildFrequencyVisualizations(steps map[string][]map[string]string) ([]DashboardVisualization, error) {
	totals, err := frequencyTotalsFromSteps(steps)
	if err != nil {
		return nil, err
	}
	categories, values := make([]string, 0, len(frequencyStatuses)), make([]string, 0, len(frequencyStatuses))
	for _, status := range frequencyStatuses {
		if count := totals.byStatus[status.code]; count > 0 {
			categories = append(categories, status.label)
			values = append(values, strconv.Itoa(count))
		}
	}
	var composition DashboardVisualization
	if len(categories) > 0 {
		composition = DashboardVisualization{
			Key: "purchase_frequency_statuses", Title: "จำนวนลูกค้าตามรอบการซื้อ", Intent: IntentComposition, Unit: UnitCount,
			Categories: categories, Series: []VisualizationSeries{{Key: "customers", Label: "จำนวนลูกค้า", Values: values}},
		}
	}
	behind, err := buildRanking("customers_behind_rhythm", "ลูกค้าที่เงียบเกินรอบ เรียงตามยอดที่เคยซื้อ", UnitTHB, steps["rows"], "cust_code", "cust_name", func(row map[string]string) (*big.Rat, error) {
		// Only customers flagged late are ranked. A customer on track can still be
		// a few days past the usual rhythm, and that is not behind it.
		if row["status"] != frequencyStatusOverdue && row["status"] != frequencyStatusLate {
			return new(big.Rat), nil
		}
		return decimal(row["sales_amount"])
	}, false)
	if err != nil {
		return nil, err
	}
	return compactVisualizations(composition, behind), nil
}
