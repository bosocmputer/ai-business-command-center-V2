// Package lookup answers the assistant's narrow live questions about one customer or item by reading the shop's system
// (SML) at the moment of asking. The statements are fixed text; the only value put into them is a record code that the
// master data copy already knows, rendered as a quoted literal. Nothing is ever written, at most two statements run per
// question, and the number asked at once, the number asked per shop per hour and the time an answer is kept are all limited.
package lookup

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

type Connections interface {
	Open(ctx context.Context, tenantID uuid.UUID) (sml.Connection, error)
}

type Querier interface {
	Query(ctx context.Context, connection sml.Connection, sql string) ([]map[string]string, error)
}

type Runner struct {
	Connections      Connections
	Client           Querier
	Now              func() time.Time
	Concurrency      int
	PerTenantPerHour int
	CacheTTL         time.Duration
	QueryTimeout     time.Duration

	mu    sync.Mutex
	cache map[string]cached
	asked map[uuid.UUID][]time.Time
	slots chan struct{}
}

type cached struct {
	result agent.LookupResult
	until  time.Time
}

func NewRunner(connections Connections, client Querier, now func() time.Time) *Runner {
	return &Runner{Connections: connections, Client: client, Now: now, Concurrency: 2, PerTenantPerHour: 30, CacheTTL: 5 * time.Minute, QueryTimeout: 25 * time.Second}
}

func (runner *Runner) acquire(ctx context.Context) (func(), error) {
	runner.mu.Lock()
	if runner.slots == nil {
		runner.slots = make(chan struct{}, max(1, runner.Concurrency))
	}
	slots := runner.slots
	runner.mu.Unlock()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-timer.C:
		return nil, agent.ErrLookupBusy
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// admit counts a real read against the shop's hourly allowance. An answer served from the short memory costs nothing.
func (runner *Runner) admit(tenantID uuid.UUID, now time.Time) bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.asked == nil {
		runner.asked = map[uuid.UUID][]time.Time{}
	}
	recent := runner.asked[tenantID][:0:0]
	for _, at := range runner.asked[tenantID] {
		if now.Sub(at) < time.Hour {
			recent = append(recent, at)
		}
	}
	if len(recent) >= runner.PerTenantPerHour {
		runner.asked[tenantID] = recent
		return false
	}
	runner.asked[tenantID] = append(recent, now)
	return true
}

func (runner *Runner) Lookup(ctx context.Context, tenantID uuid.UUID, kind agent.LookupKind, code, asOfDate string) (agent.LookupResult, error) {
	now := runner.Now()
	key := fmt.Sprintf("%s|%s|%s|%s", tenantID, kind, code, asOfDate)
	runner.mu.Lock()
	if runner.cache == nil {
		runner.cache = map[string]cached{}
	}
	if entry, ok := runner.cache[key]; ok && now.Before(entry.until) {
		runner.mu.Unlock()
		result := entry.result
		result.Cached = true
		return result, nil
	}
	runner.mu.Unlock()

	statements, parse, err := plan(kind, code, asOfDate)
	if err != nil {
		return agent.LookupResult{}, err
	}
	if !runner.admit(tenantID, now) {
		return agent.LookupResult{}, agent.ErrLookupBusy
	}
	release, err := runner.acquire(ctx)
	if err != nil {
		return agent.LookupResult{}, err
	}
	defer release()
	connection, err := runner.Connections.Open(ctx, tenantID)
	if err != nil {
		return agent.LookupResult{}, err
	}
	results := make([][]map[string]string, 0, len(statements))
	for _, statement := range statements {
		queryCtx, cancel := context.WithTimeout(ctx, runner.QueryTimeout)
		rows, queryErr := runner.Client.Query(queryCtx, connection, statement)
		cancel()
		if queryErr != nil {
			return agent.LookupResult{}, queryErr
		}
		results = append(results, rows)
	}
	result, err := parse(results)
	if err != nil {
		return agent.LookupResult{}, err
	}
	result.AsOf = runner.Now()
	runner.mu.Lock()
	runner.cache[key] = cached{result: result, until: runner.Now().Add(runner.CacheTTL)}
	if len(runner.cache) > 500 {
		for k, entry := range runner.cache {
			if !runner.Now().Before(entry.until) {
				delete(runner.cache, k)
			}
		}
	}
	runner.mu.Unlock()
	return result, nil
}

// ---- the questions ---------------------------------------------------------

type parser func(results [][]map[string]string) (agent.LookupResult, error)

func plan(kind agent.LookupKind, code, asOfDate string) ([]string, parser, error) {
	render := func(sql string, args ...any) (string, error) {
		return report.RenderSQL(report.Query{SQL: sql, Args: args})
	}
	var statements []string
	var texts []string
	var args [][]any
	var parse parser
	switch kind {
	case agent.LookupCustomerBalance:
		texts, args, parse = []string{customerBalanceSQL, customerDocumentsSQL}, [][]any{{asOfDate, code}, {asOfDate, code}}, parseCustomerBalance
	case agent.LookupCustomerSales:
		texts, args, parse = []string{customerSalesSQL, customerSalesListSQL}, [][]any{{code, asOfDate}, {code}}, parseCustomerSales
	case agent.LookupItemStock:
		texts, args, parse = []string{itemStockSQL}, [][]any{{code}}, parseItemStock
	default:
		return nil, nil, fmt.Errorf("lookup kind %q is not known", kind)
	}
	for index, text := range texts {
		statement, err := render(text, args[index]...)
		if err != nil {
			return nil, nil, err
		}
		statements = append(statements, statement)
	}
	return statements, parse, nil
}

const overdueBuckets = `('OVERDUE_1_30', 'OVERDUE_31_60', 'OVERDUE_61_90', 'OVERDUE_91_120', 'OVERDUE_120_PLUS')`

// customerBalanceSQL uses the receivable report's own open-document base, so a customer's figures agree with the report.
// $1 is the as-of date, $2 the customer code.
var customerBalanceSQL = `
with` + report.ARAgingBaseSQL + `
select
  coalesce(sum(balance), 0) as total_balance,
  coalesce(sum(balance) filter (where bucket in ` + overdueBuckets + ` and days_past_due <= ` + fmt.Sprint(report.AgingChaseableDays) + `), 0) as overdue_recent,
  coalesce(sum(balance) filter (where bucket in ` + overdueBuckets + ` and days_past_due > ` + fmt.Sprint(report.AgingChaseableDays) + `), 0) as overdue_old,
  coalesce(sum(balance) filter (where bucket = 'NOT_DUE'), 0) as not_due,
  coalesce(sum(balance) filter (where bucket = 'NO_DUE_DATE'), 0) as no_due_date,
  coalesce(sum(balance) filter (where bucket = 'CREDIT'), 0) as credit,
  count(*) as documents
from bucketed
where cust_code = $2`

var customerDocumentsSQL = `
with` + report.ARAgingBaseSQL + `
select doc_no, doc_date, due_date, balance, days_past_due, bucket
from bucketed
where cust_code = $2 and balance > 0
order by days_past_due desc nulls last, doc_date, doc_no
limit 5`

const customerSalesSQL = `
select count(*) as documents, coalesce(sum(h.total_amount), 0) as total, max(h.doc_date) as last_date
from ic_trans h
where h.trans_flag in (44) and h.last_status = 0 and h.cust_code = $1
  and h.doc_date > $2::date - 365 and h.doc_date <= $2::date
  and (coalesce(h.doc_ref, '') = '' or h.is_pos = 0) and h.is_doc_copy <> 1`

const customerSalesListSQL = `
select h.doc_no, h.doc_date, h.total_amount
from ic_trans h
where h.trans_flag in (44) and h.last_status = 0 and h.cust_code = $1
  and (coalesce(h.doc_ref, '') = '' or h.is_pos = 0) and h.is_doc_copy <> 1
order by h.doc_date desc, h.doc_no desc
limit 5`

const itemStockSQL = `
select coalesce(i.unit_standard_name, '') as unit,
  coalesce(i.balance_qty, 0) as on_hand, coalesce(i.accrued_in_qty, 0) as to_receive,
  coalesce(i.accrued_out_qty, 0) as to_deliver, coalesce(i.book_out_qty, 0) as reserved,
  coalesce((select max(coalesce(d.purchase_point, 0)) from ic_inventory_detail d where d.ic_code = i.code), 0) as reorder_point
from ic_inventory i
where i.code = $1`

// ---- reading the answers ---------------------------------------------------

func rat(text string) (*big.Rat, error) {
	value, ok := new(big.Rat).SetString(strings.TrimSpace(text))
	if !ok {
		return nil, errors.New("a figure from the shop's system is not a number")
	}
	return value, nil
}

func money(text string) (string, error) {
	value, err := rat(text)
	if err != nil {
		return "", err
	}
	return value.FloatString(2), nil
}

// quantity writes a quantity without trailing zeros: 1250 or 12.5.
func quantity(text string) (string, error) {
	value, err := rat(text)
	if err != nil {
		return "", err
	}
	out := value.FloatString(4)
	if strings.Contains(out, ".") {
		out = strings.TrimSuffix(strings.TrimRight(out, "0"), ".")
	}
	return out, nil
}

func day(text string) string {
	if len(text) >= 10 && text[4] == '-' && text[7] == '-' {
		return text[:10]
	}
	return text
}

func parseCustomerBalance(results [][]map[string]string) (agent.LookupResult, error) {
	if len(results) != 2 || len(results[0]) != 1 {
		return agent.LookupResult{}, errors.New("customer balance answer has an unexpected shape")
	}
	row := results[0][0]
	labels := []struct{ key, label string }{
		{"total_balance", "ยอดค้างรวม"}, {"overdue_recent", fmt.Sprintf("เลยกำหนดไม่เกิน %d วัน (ทวงได้)", report.AgingChaseableDays)},
		{"overdue_old", fmt.Sprintf("เลยกำหนดเกิน %d วัน (หนี้เก่า ควรทบทวน)", report.AgingChaseableDays)}, {"not_due", "ยังไม่ครบกำหนด"},
		{"no_due_date", "ไม่ระบุวันครบกำหนด (ไม่นับว่าเลยกำหนด)"}, {"credit", "เครดิตคงค้าง (ใบลดหนี้/รับคืน)"},
	}
	result := agent.LookupResult{Found: true}
	for _, item := range labels {
		value, err := money(row[item.key])
		if err != nil {
			return agent.LookupResult{}, err
		}
		result.Figures = append(result.Figures, agent.Figure{Key: item.key, Label: item.label, Unit: "THB", Value: value})
	}
	count, err := rat(row["documents"])
	if err != nil {
		return agent.LookupResult{}, err
	}
	result.Figures = append(result.Figures, agent.Figure{Key: "documents", Label: "จำนวนเอกสารที่ยังค้าง", Unit: "COUNT", Value: count.FloatString(0)})
	table := agent.Table{Title: "เอกสารค้างที่เลยกำหนดนานที่สุด (สูงสุด 5 ใบ)", Columns: []string{"เลขที่เอกสาร", "วันที่เอกสาร", "วันครบกำหนด", "ยอดค้าง", "เลยกำหนด (วัน)"}}
	for _, doc := range results[1] {
		balance, err := money(doc["balance"])
		if err != nil {
			return agent.LookupResult{}, err
		}
		due := day(doc["due_date"])
		if due == "" {
			due = "ไม่ระบุ"
		}
		table.Rows = append(table.Rows, []string{doc["doc_no"], day(doc["doc_date"]), due, balance, doc["days_past_due"]})
	}
	if len(table.Rows) > 0 {
		result.Tables = append(result.Tables, table)
	}
	return result, nil
}

func parseCustomerSales(results [][]map[string]string) (agent.LookupResult, error) {
	if len(results) != 2 || len(results[0]) != 1 {
		return agent.LookupResult{}, errors.New("customer sales answer has an unexpected shape")
	}
	row := results[0][0]
	count, err := rat(row["documents"])
	if err != nil {
		return agent.LookupResult{}, err
	}
	total, err := money(row["total"])
	if err != nil {
		return agent.LookupResult{}, err
	}
	result := agent.LookupResult{Found: true, Figures: []agent.Figure{
		{Key: "documents_365d", Label: "จำนวนใบขายใน 365 วันที่ผ่านมา", Unit: "COUNT", Value: count.FloatString(0)},
		{Key: "total_365d", Label: "ยอดขายรวมใน 365 วันที่ผ่านมา", Unit: "THB", Value: total},
	}, Warnings: []string{"นับเฉพาะใบขาย ไม่หักใบรับคืนสินค้า"}}
	table := agent.Table{Title: "ใบขายล่าสุด (สูงสุด 5 ใบ)", Columns: []string{"เลขที่เอกสาร", "วันที่", "ยอดรวม"}}
	for _, doc := range results[1] {
		amount, err := money(doc["total_amount"])
		if err != nil {
			return agent.LookupResult{}, err
		}
		table.Rows = append(table.Rows, []string{doc["doc_no"], day(doc["doc_date"]), amount})
	}
	if len(table.Rows) > 0 {
		result.Tables = append(result.Tables, table)
	}
	return result, nil
}

func parseItemStock(results [][]map[string]string) (agent.LookupResult, error) {
	if len(results) != 1 {
		return agent.LookupResult{}, errors.New("item stock answer has an unexpected shape")
	}
	if len(results[0]) == 0 {
		return agent.LookupResult{}, nil
	}
	row := results[0][0]
	unit := row["unit"]
	result := agent.LookupResult{Found: true, Warnings: []string{"ค้างรับ ค้างส่ง และจอง เป็นยอดที่ระบบของร้านบันทึกไว้กับสินค้า"}}
	for _, item := range []struct{ key, label string }{
		{"on_hand", "คงเหลือ"}, {"to_receive", "ค้างรับ (สั่งซื้อแล้วยังไม่รับ)"}, {"to_deliver", "ค้างส่ง"}, {"reserved", "จอง"}, {"reorder_point", "จุดสั่งซื้อ (0 = ไม่ได้ตั้งไว้)"},
	} {
		value, err := quantity(row[item.key])
		if err != nil {
			return agent.LookupResult{}, err
		}
		result.Figures = append(result.Figures, agent.Figure{Key: item.key, Label: item.label, Unit: unitOrQuantity(unit), Value: value})
	}
	return result, nil
}

func unitOrQuantity(unit string) string {
	if unit == "" {
		return "QUANTITY"
	}
	return unit
}
