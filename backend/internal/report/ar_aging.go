package report

import (
	"fmt"
	"math/big"
	"strconv"
)

// Receivable aging counts, as of a date, what each customer still owes on every
// open document. A document's balance is its amount minus the debt receipts
// (code 239) billed against it up to that date. Credit documents (returns,
// credit notes) carry a negative balance. A document with no due date is shown
// in its own bucket and is never aged from its document date, as decided by the
// shop owner. See docs/sml/numbers-dictionary.md.
//
// The query is not chunked: balances need every payment of a document, so a
// chunk key would have to be a customer and no merge has been written. The first
// shop measured it in under a second for 674 open documents. If a shop ever fails
// on size, the failure is reported with its parser evidence and is not retried
// as a chunked run.

const (
	agingBucketNotDue    = "NOT_DUE"
	agingBucketOverdue1  = "OVERDUE_1_30"
	agingBucketOverdue2  = "OVERDUE_31_60"
	agingBucketOverdue3  = "OVERDUE_61_90"
	agingBucketOverdue4  = "OVERDUE_91_120"
	agingBucketOverdue5  = "OVERDUE_120_PLUS"
	agingBucketNoDueDate = "NO_DUE_DATE"
	agingBucketCredit    = "CREDIT"
	// agingSummaryDebtorRowsText is the number of top debtors a summary carries.
	agingSummaryDebtorRowsText = "10"
)

// agingBuckets lists the overdue buckets in age order with their Thai labels and
// the summary column that carries each total.
var agingBuckets = []struct{ code, column, label string }{
	{agingBucketNotDue, "bucket_not_due", "ยังไม่ครบกำหนด"},
	{agingBucketOverdue1, "bucket_overdue_1_30", "เลยกำหนด 1–30 วัน"},
	{agingBucketOverdue2, "bucket_overdue_31_60", "เลยกำหนด 31–60 วัน"},
	{agingBucketOverdue3, "bucket_overdue_61_90", "เลยกำหนด 61–90 วัน"},
	{agingBucketOverdue4, "bucket_overdue_91_120", "เลยกำหนด 91–120 วัน"},
	{agingBucketOverdue5, "bucket_overdue_over_120", "เลยกำหนดเกิน 120 วัน"},
	{agingBucketNoDueDate, "bucket_no_due_date", "ไม่ระบุวันครบกำหนด"},
	{agingBucketCredit, "bucket_credit", "เครดิตคงค้าง"},
}

func agingOverdueBucket(code string) bool {
	switch code {
	case agingBucketOverdue1, agingBucketOverdue2, agingBucketOverdue3, agingBucketOverdue4, agingBucketOverdue5:
		return true
	}
	return false
}

// arAgingBaseSQL produces one row per open document with its balance and bucket.
// $1 is the as-of date.
const arAgingBaseSQL = `
open_docs as (
  select t.cust_code, t.doc_no, t.doc_date, t.due_date, t.trans_flag as doc_type_code,
    1 as direction, coalesce(t.total_amount, 0) as amount,
    coalesce((
      select sum(coalesce(p.sum_pay_money, 0)) from ap_ar_trans_detail p
      where coalesce(p.last_status, 0) = 0 and p.trans_flag = 239
        and p.billing_no = t.doc_no and p.billing_date = t.doc_date and p.doc_date <= $1::date
    ), 0) as paid_amount
  from ic_trans t
  where coalesce(t.last_status, 0) = 0 and t.doc_date <= $1::date and coalesce(t.cust_code, '') <> ''
    and ((t.trans_flag = 44 and t.inquiry_type in (0, 2)) or t.trans_flag in (46, 93, 99, 95, 101))
  union all
  select t.cust_code, t.doc_no, t.doc_date, t.due_date, t.trans_flag as doc_type_code,
    -1 as direction, coalesce(t.total_amount, 0) as amount,
    coalesce((
      select sum(coalesce(p.sum_pay_money, 0)) from ap_ar_trans_detail p
      where coalesce(p.last_status, 0) = 0 and p.trans_flag = 239
        and p.billing_no = t.doc_no and p.billing_date = t.doc_date and p.doc_date <= $1::date
    ), 0) as paid_amount
  from ic_trans t
  where coalesce(t.last_status, 0) = 0 and t.doc_date <= $1::date and coalesce(t.cust_code, '') <> ''
    and ((t.trans_flag = 48 and t.inquiry_type in (0, 2, 4)) or t.trans_flag in (97, 103))
),
aged as (
  select d.cust_code, d.doc_no, d.doc_date, d.due_date, d.doc_type_code, d.direction * d.amount as amount, d.paid_amount,
    d.direction * d.amount - d.paid_amount as balance,
    $1::date - d.doc_date as age_days,
    case when d.due_date is null then null else $1::date - d.due_date end as days_past_due
  from open_docs d
),
bucketed as (
  select a.*,
    case
      when a.balance < 0 then 'CREDIT'
      when a.due_date is null then 'NO_DUE_DATE'
      when a.days_past_due <= 0 then 'NOT_DUE'
      when a.days_past_due <= 30 then 'OVERDUE_1_30'
      when a.days_past_due <= 60 then 'OVERDUE_31_60'
      when a.days_past_due <= 90 then 'OVERDUE_61_90'
      when a.days_past_due <= 120 then 'OVERDUE_91_120'
      else 'OVERDUE_120_PLUS'
    end as bucket
  from aged a
  where abs(a.balance) >= 0.005
)`

const arAgingSQL = `
with` + arAgingBaseSQL + `
select b.cust_code, coalesce(c.name_1, '') as cust_name, b.doc_no, b.doc_date, b.due_date,
  b.doc_type_code, trans_flag(b.doc_type_code) as doc_type_label,
  b.amount, b.paid_amount, b.balance, b.age_days, b.days_past_due, b.bucket,
  case b.bucket
    when 'NOT_DUE' then 'ยังไม่ครบกำหนด'
    when 'OVERDUE_1_30' then 'เลยกำหนด 1–30 วัน'
    when 'OVERDUE_31_60' then 'เลยกำหนด 31–60 วัน'
    when 'OVERDUE_61_90' then 'เลยกำหนด 61–90 วัน'
    when 'OVERDUE_91_120' then 'เลยกำหนด 91–120 วัน'
    when 'OVERDUE_120_PLUS' then 'เลยกำหนดเกิน 120 วัน'
    when 'NO_DUE_DATE' then 'ไม่ระบุวันครบกำหนด'
    else 'เครดิตคงค้าง'
  end as bucket_label
from bucketed b
left join ar_customer c on c.code = b.cust_code
order by b.cust_code, b.doc_date, b.doc_no
`

// arAgingSummarySQL returns bounded rows: one row of bucket totals, the ten
// customers owing the most, and the metric columns on every row.
const arAgingSummarySQL = `
with` + arAgingBaseSQL + `,
summary_metrics as (
  select count(*) as _metric_row_count,
    count(distinct cust_code) filter (where bucket <> 'CREDIT') as _metric_customer_count,
    coalesce(sum(balance), 0) as _metric_total_balance,
    coalesce(sum(balance) filter (where bucket in ('OVERDUE_1_30', 'OVERDUE_31_60', 'OVERDUE_61_90', 'OVERDUE_91_120', 'OVERDUE_120_PLUS')), 0) as _metric_overdue_amount,
    coalesce(sum(balance) filter (where bucket = 'NOT_DUE'), 0) as _metric_not_due_amount,
    coalesce(sum(balance) filter (where bucket = 'NO_DUE_DATE'), 0) as _metric_no_due_date_amount,
    coalesce(sum(balance) filter (where bucket = 'CREDIT'), 0) as _metric_credit_amount
  from bucketed
),
bucket_row as (
  select 'buckets'::text as _summary_kind, null::text as cust_code, null::text as cust_name,
    null::numeric as balance, null::numeric as overdue_balance,
    coalesce(sum(balance) filter (where bucket = 'NOT_DUE'), 0) as bucket_not_due,
    coalesce(sum(balance) filter (where bucket = 'OVERDUE_1_30'), 0) as bucket_overdue_1_30,
    coalesce(sum(balance) filter (where bucket = 'OVERDUE_31_60'), 0) as bucket_overdue_31_60,
    coalesce(sum(balance) filter (where bucket = 'OVERDUE_61_90'), 0) as bucket_overdue_61_90,
    coalesce(sum(balance) filter (where bucket = 'OVERDUE_91_120'), 0) as bucket_overdue_91_120,
    coalesce(sum(balance) filter (where bucket = 'OVERDUE_120_PLUS'), 0) as bucket_overdue_over_120,
    coalesce(sum(balance) filter (where bucket = 'NO_DUE_DATE'), 0) as bucket_no_due_date,
    coalesce(sum(balance) filter (where bucket = 'CREDIT'), 0) as bucket_credit
  from bucketed
),
debtor_rows as (
  select 'ranking'::text as _summary_kind, b.cust_code, max(coalesce(c.name_1, '')) as cust_name,
    sum(b.balance) as balance,
    coalesce(sum(b.balance) filter (where b.bucket in ('OVERDUE_1_30', 'OVERDUE_31_60', 'OVERDUE_61_90', 'OVERDUE_91_120', 'OVERDUE_120_PLUS')), 0) as overdue_balance,
    null::numeric as bucket_not_due, null::numeric as bucket_overdue_1_30, null::numeric as bucket_overdue_31_60,
    null::numeric as bucket_overdue_61_90, null::numeric as bucket_overdue_91_120, null::numeric as bucket_overdue_over_120,
    null::numeric as bucket_no_due_date, null::numeric as bucket_credit
  from bucketed b
  left join ar_customer c on c.code = b.cust_code
  group by b.cust_code
  having sum(b.balance) > 0
  order by sum(b.balance) desc, b.cust_code
  limit ` + agingSummaryDebtorRowsText + `
),
selected_rows as (
  select * from bucket_row
  union all
  select * from debtor_rows
)
select selected_rows.*, summary_metrics.*,
  (selected_rows._summary_kind is null)::text as _summary_metric_row
from summary_metrics left join selected_rows on true
limit 20
`

// agingTotals are the figures every projection must be able to give.
type agingTotals struct {
	documents, customers int
	total, overdue       *big.Rat
	notDue, noDueDate    *big.Rat
	credit               *big.Rat
	buckets              map[string]*big.Rat
}

func newAgingTotals() *agingTotals {
	totals := &agingTotals{total: new(big.Rat), overdue: new(big.Rat), notDue: new(big.Rat), noDueDate: new(big.Rat), credit: new(big.Rat), buckets: map[string]*big.Rat{}}
	for _, bucket := range agingBuckets {
		totals.buckets[bucket.code] = new(big.Rat)
	}
	return totals
}

// agingTotalsFromSteps reads the totals from a summary projection's metric
// columns, or adds them up from detail rows.
func agingTotalsFromSteps(steps map[string][]map[string]string) (*agingTotals, error) {
	rows := steps["rows"]
	if _, summary := summaryMetric(steps, "total_balance"); summary {
		return agingTotalsFromSummary(steps, rows)
	}
	return agingTotalsFromDetail(realSummaryRows(rows))
}

func agingTotalsFromSummary(steps map[string][]map[string]string, rows []map[string]string) (*agingTotals, error) {
	totals := newAgingTotals()
	read := func(metric string) (*big.Rat, error) {
		parsed, err := decimal(summaryMetricOr(steps, metric, "0"))
		if err != nil {
			return nil, fieldDecimalError("_metric_"+metric, err)
		}
		return parsed, nil
	}
	var err error
	if totals.total, err = read("total_balance"); err != nil {
		return nil, err
	}
	if totals.overdue, err = read("overdue_amount"); err != nil {
		return nil, err
	}
	if totals.notDue, err = read("not_due_amount"); err != nil {
		return nil, err
	}
	if totals.noDueDate, err = read("no_due_date_amount"); err != nil {
		return nil, err
	}
	if totals.credit, err = read("credit_amount"); err != nil {
		return nil, err
	}
	totals.customers, _ = strconv.Atoi(integerText(summaryMetricOr(steps, "customer_count", "0")))
	totals.documents, _ = strconv.Atoi(integerText(summaryMetricOr(steps, "row_count", "0")))
	for _, bucketRow := range rowsForSummaryKind(rows, "buckets") {
		for _, bucket := range agingBuckets {
			if bucketRow[bucket.column] == "" {
				continue
			}
			value, parseErr := decimal(bucketRow[bucket.column])
			if parseErr != nil {
				return nil, fieldDecimalError(bucket.column, parseErr)
			}
			totals.buckets[bucket.code].Add(totals.buckets[bucket.code], value)
		}
	}
	return totals, nil
}

func agingTotalsFromDetail(rows []map[string]string) (*agingTotals, error) {
	totals := newAgingTotals()
	customers := make(map[string]struct{})
	for _, row := range rows {
		balance, err := decimal(row["balance"])
		if err != nil {
			return nil, fieldDecimalError("balance", err)
		}
		bucket := row["bucket"]
		accumulator, known := totals.buckets[bucket]
		if !known {
			return nil, fmt.Errorf("receivable aging bucket %q is not defined", bucket)
		}
		accumulator.Add(accumulator, balance)
		totals.total.Add(totals.total, balance)
		if bucket != agingBucketCredit {
			customers[row["cust_code"]] = struct{}{}
		}
		totals.documents++
	}
	for _, bucket := range agingBuckets {
		amount := totals.buckets[bucket.code]
		switch {
		case agingOverdueBucket(bucket.code):
			totals.overdue.Add(totals.overdue, amount)
		case bucket.code == agingBucketNotDue:
			totals.notDue.Set(amount)
		case bucket.code == agingBucketNoDueDate:
			totals.noDueDate.Set(amount)
		case bucket.code == agingBucketCredit:
			totals.credit.Set(amount)
		}
	}
	totals.customers = len(customers)
	return totals, nil
}

func (totals *agingTotals) metrics() map[string]string {
	return map[string]string{
		"customer_count":     strconv.Itoa(totals.customers),
		"document_count":     strconv.Itoa(totals.documents),
		"total_balance":      money(totals.total),
		"overdue_amount":     money(totals.overdue),
		"not_due_amount":     money(totals.notDue),
		"no_due_date_amount": money(totals.noDueDate),
		"credit_amount":      money(totals.credit),
	}
}

// agingBucketRow gives one map of bucket totals whatever the projection, so the
// composition chart can be built the same way for both.
func agingBucketRow(rows []map[string]string) (map[string]string, error) {
	totals := newAgingTotals()
	detail := false
	for _, row := range realSummaryRows(rows) {
		if row["bucket"] == "" {
			continue
		}
		detail = true
		balance, err := decimal(row["balance"])
		if err != nil {
			return nil, fieldDecimalError("balance", err)
		}
		if accumulator, known := totals.buckets[row["bucket"]]; known {
			accumulator.Add(accumulator, balance)
		}
	}
	if !detail {
		for _, bucketRow := range rowsForSummaryKind(rows, "buckets") {
			if bucketRow["bucket_not_due"] != "" || bucketRow["bucket_no_due_date"] != "" {
				return bucketRow, nil
			}
		}
		return map[string]string{}, nil
	}
	merged := make(map[string]string, len(agingBuckets))
	for _, bucket := range agingBuckets {
		merged[bucket.column] = money(totals.buckets[bucket.code])
	}
	return merged, nil
}

func buildAgingVisualizations(rows []map[string]string) ([]DashboardVisualization, error) {
	bucketRow, err := agingBucketRow(rows)
	if err != nil {
		return nil, err
	}
	fields := make([]fieldLabel, 0, len(agingBuckets))
	for _, bucket := range agingBuckets {
		fields = append(fields, fieldLabel{bucket.column, bucket.label})
	}
	composition, err := buildComposition("ar_aging_buckets", "ยอดค้างตามอายุหนี้", []map[string]string{bucketRow}, fields)
	if err != nil {
		return nil, err
	}
	debtors, err := buildRanking("top_debtors", "ลูกหนี้ค้างสูงสุด", UnitTHB, rows, "cust_code", "cust_name", func(row map[string]string) (*big.Rat, error) {
		return decimal(row["balance"])
	}, false)
	if err != nil {
		return nil, err
	}
	return compactVisualizations(composition, debtors), nil
}
