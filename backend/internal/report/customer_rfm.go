package report

import (
	"fmt"
	"math/big"
	"strconv"
)

// Customer RFM ranks every customer who bought in the period by how recently
// (R), how often (F) and how much (M) they bought, then names a segment from R
// and F. Score 5 is always the best: recency is ordered so that a more recent
// last purchase scores higher, frequency and amount so that more scores higher.
// The smlmcpconnect report this replaces scored them the other way round and
// labelled its best customers "Hibernating", so its SQL must not be copied.
//
//   - Recency: days from the customer's last code 44 sale to the end of the period.
//   - Frequency: number of code 44 sales in the period.
//   - Monetary: net sales including VAT (44 + 46 - 48) in the period.
//
// Scores come from the customer's percentile position, so customers with the
// same value always get the same score (a plain NTILE would split a tie
// arbitrarily). The scores are relative to the shop's own customers in the same
// period, which is why the raw recency, frequency and amount are shown next to
// them. See docs/sml/numbers-dictionary.md.
//
// The query is not chunked: a percentile needs every customer at once, and the
// result has one row per customer, not per document. A shop that fails on size
// gets the failure with its parser evidence, not a retry as a chunked run.

const (
	rfmSegmentChampion    = "CHAMPION"
	rfmSegmentLoyal       = "LOYAL"
	rfmSegmentPromising   = "PROMISING"
	rfmSegmentNeedsCare   = "NEEDS_ATTENTION"
	rfmSegmentAtRisk      = "AT_RISK"
	rfmSegmentHibernating = "HIBERNATING"
	// rfmSummaryCustomerRowsText is the number of top customers a summary carries.
	rfmSummaryCustomerRowsText = "10"
)

// rfmSegments lists the segments in the order they are shown, with the Thai
// label and the rule written in the SQL below.
var rfmSegments = []struct{ code, label string }{
	{rfmSegmentChampion, "ลูกค้าดีเด่น"},
	{rfmSegmentLoyal, "ลูกค้าประจำ"},
	{rfmSegmentPromising, "ลูกค้าใหม่/เพิ่งกลับมา"},
	{rfmSegmentNeedsCare, "ต้องดูแล"},
	{rfmSegmentAtRisk, "เสี่ยงหาย"},
	{rfmSegmentHibernating, "เงียบหาย"},
}

// customerRFMBaseSQL produces one scored row per customer with at least one
// code 44 sale in the period. $1 is the first day and $2 the last day.
const customerRFMBaseSQL = `
sales_docs as (
  select t.cust_code, t.trans_flag, t.doc_date,
    case when t.trans_flag = 48 then -coalesce(t.total_amount, 0) else coalesce(t.total_amount, 0) end as signed_amount
  from ic_trans t
  where t.trans_flag in (44, 46, 48)
    and coalesce(t.last_status, 0) = 0
    and t.doc_date between $1::date and $2::date
    and coalesce(t.cust_code, '') <> ''
    and coalesce(t.is_doc_copy, 0) <> 1
    and (t.trans_flag <> 44 or coalesce(t.doc_ref, '') = '' or coalesce(t.is_pos, 0) = 0)
),
customers as (
  select s.cust_code,
    max(s.doc_date) filter (where s.trans_flag = 44) as last_purchase_date,
    count(*) filter (where s.trans_flag = 44) as frequency,
    sum(s.signed_amount) as monetary
  from sales_docs s
  group by s.cust_code
  having count(*) filter (where s.trans_flag = 44) > 0
),
ranked as (
  select c.*,
    $2::date - c.last_purchase_date as recency_days,
    percent_rank() over (order by $2::date - c.last_purchase_date desc) as r_rank,
    percent_rank() over (order by c.frequency) as f_rank,
    percent_rank() over (order by c.monetary) as m_rank
  from customers c
),
scored as (
  select r.cust_code, r.last_purchase_date, r.recency_days, r.frequency, r.monetary,
    least(5, 1 + floor(5 * r.r_rank))::int as r_score,
    least(5, 1 + floor(5 * r.f_rank))::int as f_score,
    least(5, 1 + floor(5 * r.m_rank))::int as m_score
  from ranked r
),
segmented as (
  select s.*,
    case
      when s.r_score >= 4 and s.f_score >= 4 then 'CHAMPION'
      when s.r_score >= 4 and s.f_score <= 2 then 'PROMISING'
      when s.r_score >= 3 and s.f_score >= 3 then 'LOYAL'
      when s.r_score <= 2 and s.f_score >= 3 then 'AT_RISK'
      when s.r_score <= 2 then 'HIBERNATING'
      else 'NEEDS_ATTENTION'
    end as segment
  from scored s
)`

const customerRFMSegmentLabelSQL = `
  case %s
    when 'CHAMPION' then 'ลูกค้าดีเด่น'
    when 'LOYAL' then 'ลูกค้าประจำ'
    when 'PROMISING' then 'ลูกค้าใหม่/เพิ่งกลับมา'
    when 'NEEDS_ATTENTION' then 'ต้องดูแล'
    when 'AT_RISK' then 'เสี่ยงหาย'
    else 'เงียบหาย'
  end`

func customerRFMSegmentLabel(column string) string {
	return fmt.Sprintf(customerRFMSegmentLabelSQL, column)
}

var customerRFMSQL = `
with` + customerRFMBaseSQL + `
select g.cust_code, coalesce(c.name_1, '') as cust_name, g.last_purchase_date, g.recency_days,
  g.frequency, g.monetary, g.monetary / g.frequency as average_per_order,
  g.r_score, g.f_score, g.m_score,
  g.r_score::text || g.f_score::text || g.m_score::text as rfm_code,
  g.segment,` + customerRFMSegmentLabel("g.segment") + ` as segment_label
from segmented g
left join ar_customer c on c.code = g.cust_code
order by g.monetary desc, g.cust_code
`

// customerRFMSummarySQL returns bounded rows: one row per segment, the ten
// customers who bought the most, and the metric columns on every row.
var customerRFMSummarySQL = `
with` + customerRFMBaseSQL + `,
summary_metrics as (
  select count(*) as _metric_row_count,
    count(*) as _metric_customer_count,
    coalesce(sum(monetary), 0) as _metric_total_amount,
    count(*) filter (where segment = 'CHAMPION') as _metric_champion_count,
    count(*) filter (where segment = 'AT_RISK') as _metric_at_risk_count,
    coalesce(sum(monetary) filter (where segment = 'AT_RISK'), 0) as _metric_at_risk_amount,
    count(*) filter (where segment = 'HIBERNATING') as _metric_hibernating_count
  from segmented
),
segment_rows as (
  select 'segments'::text as _summary_kind, g.segment, ` + customerRFMSegmentLabel("g.segment") + ` as segment_label,
    null::text as cust_code, null::text as cust_name,
    count(*) as customer_count, sum(g.monetary) as monetary, null::int as frequency
  from segmented g
  group by g.segment
),
customer_rows as (
  select 'ranking'::text as _summary_kind, g.segment, ` + customerRFMSegmentLabel("g.segment") + ` as segment_label,
    g.cust_code, coalesce(c.name_1, '') as cust_name,
    1::bigint as customer_count, g.monetary, g.frequency
  from segmented g
  left join ar_customer c on c.code = g.cust_code
  order by g.monetary desc, g.cust_code
  limit ` + rfmSummaryCustomerRowsText + `
),
selected_rows as (
  select * from segment_rows
  union all
  select * from customer_rows
)
select selected_rows.*, summary_metrics.*,
  (selected_rows._summary_kind is null)::text as _summary_metric_row
from summary_metrics left join selected_rows on true
limit 30
`

type rfmSegmentTotal struct {
	customers int
	amount    *big.Rat
}

// rfmTotals are the figures every projection must be able to give.
type rfmTotals struct {
	customers int
	total     *big.Rat
	segments  map[string]*rfmSegmentTotal
}

func newRFMTotals() *rfmTotals {
	totals := &rfmTotals{total: new(big.Rat), segments: map[string]*rfmSegmentTotal{}}
	for _, segment := range rfmSegments {
		totals.segments[segment.code] = &rfmSegmentTotal{amount: new(big.Rat)}
	}
	return totals
}

func rfmKnownSegment(totals *rfmTotals, code string) (*rfmSegmentTotal, error) {
	segment, known := totals.segments[code]
	if !known {
		return nil, fmt.Errorf("customer RFM segment %q is not defined", code)
	}
	return segment, nil
}

// rfmTotalsFromSteps reads the totals from a summary projection, or adds them up
// from per-customer rows.
func rfmTotalsFromSteps(steps map[string][]map[string]string) (*rfmTotals, error) {
	rows := steps["rows"]
	if _, summary := summaryMetric(steps, "customer_count"); summary {
		return rfmTotalsFromSummary(steps, rows)
	}
	return rfmTotalsFromDetail(realSummaryRows(rows))
}

func rfmTotalsFromSummary(steps map[string][]map[string]string, rows []map[string]string) (*rfmTotals, error) {
	totals := newRFMTotals()
	total, err := decimal(summaryMetricOr(steps, "total_amount", "0"))
	if err != nil {
		return nil, fieldDecimalError("_metric_total_amount", err)
	}
	totals.total = total
	totals.customers, _ = strconv.Atoi(integerText(summaryMetricOr(steps, "customer_count", "0")))
	for _, row := range rowsForSummaryKind(rows, "segments") {
		segment, err := rfmKnownSegment(totals, row["segment"])
		if err != nil {
			return nil, err
		}
		count, err := strconv.Atoi(integerText(row["customer_count"]))
		if err != nil {
			return nil, fmt.Errorf("report field customer_count is invalid: %w", err)
		}
		amount, err := decimal(row["monetary"])
		if err != nil {
			return nil, fieldDecimalError("monetary", err)
		}
		segment.customers += count
		segment.amount.Add(segment.amount, amount)
	}
	return totals, nil
}

func rfmTotalsFromDetail(rows []map[string]string) (*rfmTotals, error) {
	totals := newRFMTotals()
	for _, row := range rows {
		segment, err := rfmKnownSegment(totals, row["segment"])
		if err != nil {
			return nil, err
		}
		amount, err := decimal(row["monetary"])
		if err != nil {
			return nil, fieldDecimalError("monetary", err)
		}
		segment.customers++
		segment.amount.Add(segment.amount, amount)
		totals.total.Add(totals.total, amount)
		totals.customers++
	}
	return totals, nil
}

func (totals *rfmTotals) metrics() map[string]string {
	return map[string]string{
		"customer_count":    strconv.Itoa(totals.customers),
		"total_amount":      money(totals.total),
		"champion_count":    strconv.Itoa(totals.segments[rfmSegmentChampion].customers),
		"at_risk_count":     strconv.Itoa(totals.segments[rfmSegmentAtRisk].customers),
		"at_risk_amount":    money(totals.segments[rfmSegmentAtRisk].amount),
		"hibernating_count": strconv.Itoa(totals.segments[rfmSegmentHibernating].customers),
	}
}

// buildRFMVisualizations draws the amount bought by each segment and the ten
// customers who bought the most, whichever projection the rows came from.
func buildRFMVisualizations(steps map[string][]map[string]string) ([]DashboardVisualization, error) {
	rows := steps["rows"]
	totals, err := rfmTotalsFromSteps(steps)
	if err != nil {
		return nil, err
	}
	segmentRow := make(map[string]string, len(rfmSegments))
	fields := make([]fieldLabel, 0, len(rfmSegments))
	for _, segment := range rfmSegments {
		segmentRow[segment.code] = money(totals.segments[segment.code].amount)
		fields = append(fields, fieldLabel{segment.code, segment.label})
	}
	composition, err := buildComposition("customer_rfm_segments", "ยอดซื้อตามกลุ่มลูกค้า", []map[string]string{segmentRow}, fields)
	if err != nil {
		return nil, err
	}
	customers, err := buildRanking("top_customers", "ลูกค้าซื้อสูงสุด", UnitTHB, rows, "cust_code", "cust_name", func(row map[string]string) (*big.Rat, error) {
		return decimal(row["monetary"])
	}, false)
	if err != nil {
		return nil, err
	}
	return compactVisualizations(composition, customers), nil
}
