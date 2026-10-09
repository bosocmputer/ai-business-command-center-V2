package lookup

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

// A document is found by the number written on it. The number is the only value put into the statements, as a quoted
// literal; the document codes searched come from the families the caller may read, and are fixed here.

const (
	headerLimit = 5
	linesLimit  = 12
)

// documentFlags maps a family of documents to the ic_trans document codes it covers. Debt receipts (239) live in
// ap_ar_trans and are handled apart.
func documentFlags(classes []agent.DocumentClass) (icFlags []int, debtReceipts bool) {
	for _, class := range classes {
		switch class {
		case agent.DocumentSales:
			icFlags = append(icFlags, 44, 46, 48)
		case agent.DocumentPurchase:
			icFlags = append(icFlags, 12)
		case agent.DocumentDebtReceipt:
			debtReceipts = true
		}
	}
	return icFlags, debtReceipts
}

func flagList(flags []int) string {
	parts := make([]string, len(flags))
	for index, flag := range flags {
		parts[index] = fmt.Sprint(flag)
	}
	return strings.Join(parts, ", ")
}

func documentPlan(number string, classes []agent.DocumentClass) ([]string, parser, error) {
	icFlags, debtReceipts := documentFlags(classes)
	if len(icFlags) == 0 && !debtReceipts {
		return nil, nil, errors.New("no document family is allowed")
	}
	var branches []string
	if len(icFlags) > 0 {
		branches = append(branches, `
  select 'ic' as src, h.trans_flag, h.doc_no, h.doc_date, h.cust_code, coalesce(c.name_1, s.name_1, '') as party_name,
    h.inquiry_type, h.total_amount, h.last_status, coalesce(h.doc_ref, '') as doc_ref, h.due_date
  from ic_trans h
  left join ar_customer c on c.code = h.cust_code
  left join ap_supplier s on s.code = h.cust_code
  where h.doc_no = $1 and h.trans_flag in (`+flagList(icFlags)+`)`)
	}
	if debtReceipts {
		branches = append(branches, `
  select 'ar' as src, a.trans_flag, a.doc_no, a.doc_date, a.cust_code, coalesce(c.name_1, '') as party_name,
    0 as inquiry_type, a.total_net_value as total_amount, a.last_status, coalesce(a.doc_ref, '') as doc_ref, null::date as due_date
  from ap_ar_trans a
  left join ar_customer c on c.code = a.cust_code
  where a.doc_no = $1 and a.trans_flag = 239`)
	}
	header := `
select x.src, x.trans_flag, trans_flag(x.trans_flag) as label, x.doc_no, x.doc_date, x.cust_code, x.party_name,
  x.inquiry_type, x.total_amount, x.last_status, x.doc_ref, x.due_date
from (` + strings.Join(branches, "\n  union all") + `
) x
order by x.doc_date desc, x.trans_flag
limit ` + fmt.Sprint(headerLimit)
	texts := []string{header}
	if len(icFlags) > 0 {
		texts = append(texts, `
select d.trans_flag, d.line_number, d.item_code, coalesce(i.name_1, d.item_name, '') as item_name, d.qty,
  coalesce(u.name_1, d.unit_code, '') as unit_name, d.price, d.sum_amount
from ic_trans_detail d
left join ic_inventory i on i.code = d.item_code
left join ic_unit u on u.code = d.unit_code
where d.doc_no = $1 and d.trans_flag in (`+flagList(icFlags)+`) and d.last_status = 0
order by d.trans_flag, d.line_number
limit `+fmt.Sprint(linesLimit), `
select
  (select count(*) from ic_trans_detail where doc_no = $1 and trans_flag in (`+flagList(icFlags)+`) and last_status = 0) as line_count,
  (select coalesce(sum(sum_amount), 0) from ic_trans_detail where doc_no = $1 and trans_flag in (`+flagList(icFlags)+`) and last_status = 0) as line_total,
  (select count(*) from ap_ar_trans_detail where trans_flag = 239 and coalesce(last_status, 0) = 0 and billing_no = $1) as receipt_lines,
  (select coalesce(sum(sum_pay_money), 0) from ap_ar_trans_detail where trans_flag = 239 and coalesce(last_status, 0) = 0 and billing_no = $1) as paid_amount`)
	}
	statements := make([]string, len(texts))
	for index, text := range texts {
		rendered, err := report.RenderSQL(report.Query{SQL: text, Args: []any{number}})
		if err != nil {
			return nil, nil, err
		}
		statements[index] = rendered
	}
	return statements, func(results [][]map[string]string) (agent.LookupResult, error) {
		return parseDocument(results, len(icFlags) > 0)
	}, nil
}

func statusWord(lastStatus string) string {
	if strings.TrimSpace(lastStatus) == "1" {
		return "ยกเลิกแล้ว"
	}
	return "ปกติ"
}

// payWord says cash or credit for the sale documents, where inquiry_type decides it.
func payWord(flag, inquiry string) string {
	switch strings.TrimSpace(flag) {
	case "44", "46", "48":
		switch strings.TrimSpace(inquiry) {
		case "0", "2":
			return "เงินเชื่อ"
		case "1", "3":
			return "เงินสด"
		}
	}
	return "-"
}

func parseDocument(results [][]map[string]string, hasLines bool) (agent.LookupResult, error) {
	if len(results) == 0 || (hasLines && len(results) != 3) {
		return agent.LookupResult{}, errors.New("document answer has an unexpected shape")
	}
	headers := results[0]
	if len(headers) == 0 {
		return agent.LookupResult{}, nil
	}
	result := agent.LookupResult{Found: true}
	table := agent.Table{Title: "เอกสาร", Columns: []string{"เลขที่", "ประเภท", "วันที่", "รหัสคู่ค้า", agent.DocumentNameColumn, "ยอดรวมหัวเอกสาร (รวม VAT)", "สถานะ", "เงินสด/เชื่อ", "เอกสารอ้างอิง", "ครบกำหนด"}}
	cancelled := false
	for _, row := range headers {
		amount, err := money(row["total_amount"])
		if err != nil {
			return agent.LookupResult{}, err
		}
		due := day(row["due_date"])
		if due == "" {
			due = "-"
		}
		reference := strings.TrimSpace(row["doc_ref"])
		if reference == "" {
			reference = "-"
		}
		if strings.TrimSpace(row["last_status"]) == "1" {
			cancelled = true
		}
		table.Rows = append(table.Rows, []string{row["doc_no"], row["label"], day(row["doc_date"]), row["cust_code"], row["party_name"], amount,
			statusWord(row["last_status"]), payWord(row["trans_flag"], row["inquiry_type"]), reference, due})
	}
	result.Tables = append(result.Tables, table)
	if len(headers) > 1 {
		result.Warnings = append(result.Warnings, "เลขที่นี้ตรงกับเอกสารมากกว่าหนึ่งชนิด (เช่น ใบขายกับใบยกเลิก) ดูคอลัมน์ประเภทและสถานะ")
	}
	if cancelled {
		result.Warnings = append(result.Warnings, "มีเอกสารที่ยกเลิกแล้ว ยอดของใบที่ยกเลิกไม่นับในรายงาน")
	}
	if hasLines {
		lines := agent.Table{Title: "รายการสินค้า", Columns: []string{"ลำดับ", "รหัสสินค้า", "ชื่อสินค้า", "จำนวน", "หน่วย", "ราคาต่อหน่วย", "ยอด"}}
		for _, row := range results[1] {
			qty, err := quantity(row["qty"])
			if err != nil {
				return agent.LookupResult{}, err
			}
			price, err := money(row["price"])
			if err != nil {
				return agent.LookupResult{}, err
			}
			amount, err := money(row["sum_amount"])
			if err != nil {
				return agent.LookupResult{}, err
			}
			lines.Rows = append(lines.Rows, []string{lineNumber(row["line_number"]), row["item_code"], row["item_name"], qty, row["unit_name"], price, amount})
		}
		if len(results[2]) == 1 {
			summary := results[2][0]
			count, err := rat(summary["line_count"])
			if err != nil {
				return agent.LookupResult{}, err
			}
			total, err := money(summary["line_total"])
			if err != nil {
				return agent.LookupResult{}, err
			}
			result.Figures = append(result.Figures,
				agent.Figure{Key: "line_count", Label: "จำนวนรายการสินค้าในเอกสาร", Unit: "COUNT", Value: count.FloatString(0)},
				agent.Figure{Key: "line_total", Label: "ยอดรวมของรายการสินค้า (ตามบรรทัด ไม่ใช่ยอดหัวเอกสาร)", Unit: "THB", Value: total})
			if shown, _ := count.Float64(); shown > float64(len(lines.Rows)) {
				lines.Title = fmt.Sprintf("รายการสินค้า (แสดง %d จาก %s รายการ)", len(lines.Rows), count.FloatString(0))
			}
			receipts, err := rat(summary["receipt_lines"])
			if err != nil {
				return agent.LookupResult{}, err
			}
			if receipts.Sign() > 0 {
				paid, err := money(summary["paid_amount"])
				if err != nil {
					return agent.LookupResult{}, err
				}
				result.Figures = append(result.Figures, agent.Figure{Key: "paid_amount", Label: "รับชำระแล้วตามใบรับชำระหนี้ที่วางบิลกับเอกสารนี้", Unit: "THB", Value: paid})
			}
		}
		if len(lines.Rows) > 0 {
			result.Tables = append(result.Tables, lines)
		}
		result.Warnings = append(result.Warnings, "ยอดหัวเอกสารรวม VAT ส่วนยอดรายการอาจต่างกันตามชนิดภาษีของเอกสาร")
	}
	return result, nil
}

// lineNumber is the line's position as people count it: SML numbers lines from zero.
func lineNumber(text string) string {
	value, err := rat(text)
	if err != nil {
		return strings.TrimSpace(text)
	}
	return value.Add(value, big.NewRat(1, 1)).FloatString(0)
}
