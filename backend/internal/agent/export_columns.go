package agent

import "github.com/bosocmputer/nextstep-dashboard-backend/internal/report"

// ExportColumn is one column of an exported report: the key the rows carry, the Thai heading the web page shows, and
// whether the column is text, a number or a date, so the file can store numbers as numbers.
type ExportColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Type  string `json:"type"`
	// Total says the column is an amount that makes sense to add up (money). Prices, quantities in mixed units, days and
	// scores do not, so the file shows no total for them.
	Total bool `json:"total,omitempty"`
}

const (
	columnText   = "text"
	columnNumber = "number"
	columnDate   = "date"
)

func textColumn(key, label string) ExportColumn {
	return ExportColumn{Key: key, Label: label, Type: columnText}
}
func numberColumn(key, label string) ExportColumn {
	return ExportColumn{Key: key, Label: label, Type: columnNumber, Total: moneyColumns[key]}
}
func dateColumn(key, label string) ExportColumn {
	return ExportColumn{Key: key, Label: label, Type: columnDate}
}

// moneyColumns are the number columns that are amounts of money and so are added up in the file's totals.
var moneyColumns = map[string]bool{
	"sum_amount": true, "total_amount": true, "amount_sale": true, "cost_sale": true, "amount_sale_return": true, "cost_sale_return": true,
	"balance_amount": true, "amount_in": true, "amount_out": true, "monetary": true, "sales_amount": true, "amount": true, "paid_amount": true,
	"balance": true, "total_net_value": true, "cash_amount": true, "transfer_amount": true, "card_amount": true, "chq_amount": true,
	"coupon_amount": true, "petty_cash_amount": true, "total_value": true, "total_discount": true, "total_except_vat": true,
	"total_vat_value": true, "total_except_discount": true,
}

// exportValueLabels turn a code the rows carry into the words the owner reads.
var exportValueLabels = map[string]map[string]string{
	"vat_type": {"E": "VAT แยกนอก", "I": "VAT รวมใน", "C": "VAT 0%", "3": "ไม่มีผลต่อภาษี"},
}

// exportColumns are the columns of each report's detail rows, in the order and with the headings the web page uses. The
// internal columns the page hides (row numbers, raw codes of a label) are left out. A column the report's rows do not
// carry is simply empty, and the assistant's file drops it.
var exportColumns = map[report.Key][]ExportColumn{
	report.SalesGoodsServices: {
		dateColumn("doc_date", "วันที่"), textColumn("doc_time", "เวลา"), textColumn("doc_no", "เลขที่เอกสาร"), textColumn("doc_type", "ประเภทเอกสาร"),
		textColumn("doc_ref", "เอกสารอ้างอิง"), textColumn("cust_code", "รหัสลูกค้า"), textColumn("cust_name", "ลูกค้า"),
		textColumn("item_code", "รหัสสินค้า"), textColumn("item_name", "สินค้า"), numberColumn("qty", "จำนวน"), textColumn("unit_code", "หน่วย"),
		numberColumn("price", "ราคา"), textColumn("discount", "ส่วนลด"), numberColumn("sum_amount", "มูลค่ารายการ"),
		numberColumn("total_value", "ยอดก่อนส่วนลด"), numberColumn("total_discount", "ส่วนลดท้ายบิล"), numberColumn("total_except_discount", "ยอดหลังส่วนลด"), numberColumn("total_except_vat", "ยอดก่อน VAT"),
		numberColumn("vat_rate", "อัตรา VAT (%)"), numberColumn("total_vat_value", "VAT"), numberColumn("total_amount", "ยอดขายสุทธิ (รวม VAT)"),
		textColumn("vat_type", "ประเภทภาษี"), textColumn("cashier_code", "ผู้ทำรายการ"),
		textColumn("branch_code", "สาขา"), textColumn("wh_code", "คลัง"), textColumn("shelf_code", "ที่เก็บ"),
	},
	report.PurchaseGoodsPayables: {
		dateColumn("doc_date", "วันที่"), textColumn("doc_time", "เวลา"), textColumn("doc_no", "เลขที่เอกสาร"), textColumn("doc_ref", "เอกสารอ้างอิง"),
		dateColumn("doc_ref_date", "วันที่เอกสารอ้างอิง"), textColumn("cust_code", "รหัสผู้จำหน่าย"), textColumn("cust_name", "ผู้จำหน่าย"),
		textColumn("item_code", "รหัสสินค้า"), textColumn("item_name", "สินค้า"), numberColumn("qty", "จำนวน"), textColumn("unit_code", "หน่วย"),
		numberColumn("price", "ราคา"), textColumn("discount", "ส่วนลด"), numberColumn("sum_amount", "มูลค่ารายการ"),
		numberColumn("total_value", "ยอดก่อนส่วนลด"), numberColumn("total_discount", "ส่วนลดท้ายบิล"), numberColumn("total_except_discount", "ยอดหลังส่วนลด"), numberColumn("total_except_vat", "ยอดก่อน VAT"),
		numberColumn("vat_rate", "อัตรา VAT (%)"), numberColumn("total_vat_value", "VAT"), numberColumn("total_amount", "ยอดซื้อสุทธิ (รวม VAT)"),
		textColumn("vat_type", "ประเภทภาษี"), textColumn("cashier_code", "ผู้ทำรายการ"),
		textColumn("branch_code", "สาขา"), textColumn("wh_code", "คลัง"), textColumn("shelf_code", "ที่เก็บ"),
	},
	report.GrossProfitByProduct: {
		textColumn("code", "รหัสสินค้า"), textColumn("name_1", "ชื่อสินค้า"), textColumn("unit_name", "หน่วย"), numberColumn("qty_sale", "จำนวนขาย"),
		numberColumn("amount_sale", "ยอดขาย"), numberColumn("cost_sale", "ต้นทุนขาย"), numberColumn("qty_sale_return", "จำนวนคืน"),
		numberColumn("amount_sale_return", "ยอดคืน"), numberColumn("cost_sale_return", "ต้นทุนคืน"),
	},
	report.GrossProfitByARCustomer: {
		textColumn("ar_code", "รหัสลูกหนี้"), textColumn("ar_detail", "ชื่อลูกหนี้"), numberColumn("qty_sale", "จำนวนขาย"), numberColumn("amount_sale", "ยอดขาย"),
		numberColumn("cost_sale", "ต้นทุนขาย"), numberColumn("qty_sale_return", "จำนวนคืน"), numberColumn("amount_sale_return", "ยอดคืน"),
		numberColumn("cost_sale_return", "ต้นทุนคืน"),
	},
	report.StockBalance: {
		textColumn("ic_code", "รหัสสินค้า"), textColumn("ic_name", "ชื่อสินค้า"), textColumn("ic_unit_code", "หน่วย"), numberColumn("balance_qty", "คงเหลือ"),
		numberColumn("balance_amount", "มูลค่าคงเหลือ"), numberColumn("average_cost_end", "ต้นทุนล่าสุด"), numberColumn("average_cost", "ต้นทุนเฉลี่ย"),
		numberColumn("qty_in", "รับเข้า"), numberColumn("amount_in", "มูลค่ารับเข้า"), numberColumn("qty_out", "จ่ายออก"), numberColumn("amount_out", "มูลค่าจ่ายออก"),
		numberColumn("average_cost_in", "ต้นทุนรับเข้า"), numberColumn("average_cost_out", "ต้นทุนจ่ายออก"),
	},
	report.StockReorder: {
		textColumn("ic_code", "รหัสสินค้า"), textColumn("ic_name", "ชื่อสินค้า"), textColumn("ic_unit_code", "หน่วย"), numberColumn("balance_qty", "คงเหลือ"),
		numberColumn("purchase_point", "จุดสั่งซื้อ"), numberColumn("purchase_balance_qty", "สินค้ารอรับ"),
	},
	report.PurchaseFrequency: {
		textColumn("cust_code", "รหัสลูกค้า"), textColumn("cust_name", "ลูกค้า"), textColumn("status_label", "รอบการซื้อ"), numberColumn("avg_gap_days", "รอบซื้อเฉลี่ย (วัน)"),
		numberColumn("days_since_last", "ไม่ได้ซื้อมา (วัน)"), numberColumn("days_late", "ช้ากว่ารอบ (วัน)"), numberColumn("sales_amount", "ยอดที่ซื้อ"),
		numberColumn("purchase_days", "ซื้อกี่วัน"), dateColumn("first_purchase_date", "ซื้อครั้งแรก"), dateColumn("last_purchase_date", "ซื้อล่าสุด"),
		numberColumn("document_count", "จำนวนใบขาย"), numberColumn("span_days", "ช่วงที่ซื้อ (วัน)"), numberColumn("gap_ratio", "ไม่ได้ซื้อ / รอบปกติ (เท่า)"),
	},
	report.CustomerRFM: {
		textColumn("cust_code", "รหัสลูกค้า"), textColumn("cust_name", "ลูกค้า"), textColumn("segment_label", "กลุ่มลูกค้า"), numberColumn("monetary", "ยอดซื้อสุทธิ"),
		numberColumn("frequency", "ซื้อกี่ครั้ง"), dateColumn("last_purchase_date", "ซื้อล่าสุด"), numberColumn("recency_days", "ห่างมากี่วัน"),
		textColumn("rfm_code", "คะแนน R-F-M"), numberColumn("average_per_order", "เฉลี่ยต่อใบ"), numberColumn("r_score", "คะแนนความใหม่"),
		numberColumn("f_score", "คะแนนความถี่"), numberColumn("m_score", "คะแนนมูลค่า"),
	},
	report.ARAging: {
		textColumn("cust_code", "รหัสลูกหนี้"), textColumn("cust_name", "ลูกหนี้"), textColumn("doc_no", "เลขที่เอกสาร"), textColumn("doc_type_label", "ประเภทเอกสาร"),
		dateColumn("doc_date", "วันที่ออกใบ"), dateColumn("due_date", "ครบกำหนด"), numberColumn("amount", "ยอดเอกสาร"), numberColumn("paid_amount", "ชำระแล้ว"),
		numberColumn("balance", "ยอดค้าง"), textColumn("bucket_label", "อายุหนี้"), textColumn("doc_age_label", "อายุจากวันที่ออกใบ"),
		numberColumn("days_past_due", "เลยกำหนด (วัน)"), numberColumn("age_days", "อายุเอกสาร (วัน)"),
	},
	report.ARCustomerMovement: {
		textColumn("cust_code", "รหัสลูกหนี้"), textColumn("cust_name", "ลูกหนี้"), dateColumn("doc_date", "วันที่"), textColumn("doc_no", "เลขที่เอกสาร"),
		numberColumn("amount", "ยอดเคลื่อนไหว"), numberColumn("credit_day", "เครดิต (วัน)"), textColumn("tax_doc_no", "เลขที่ใบกำกับ"), textColumn("doc_ref", "เอกสารอ้างอิง"),
	},
	report.ARDebtReceipt: {
		dateColumn("doc_date", "วันที่"), textColumn("doc_no", "เลขที่รับชำระ"), textColumn("cust_code", "รหัสลูกค้า"), textColumn("cust_name", "ลูกค้า"),
		numberColumn("total_net_value", "ยอดรับชำระ"), numberColumn("cash_amount", "เงินสด"), numberColumn("transfer_amount", "เงินโอน"), dateColumn("billing_date", "วันที่วางบิล"),
	},
	report.CashBankReceipts: {
		dateColumn("doc_date", "วันที่"), textColumn("doc_time", "เวลา"), textColumn("doc_no", "เลขที่เอกสาร"), textColumn("trans_flag_label", "ประเภทรายการ"),
		textColumn("ap_ar_code", "รหัสลูกค้า"), textColumn("ap_ar_name", "ผู้ชำระ/รายละเอียด"), numberColumn("total_amount", "ยอดรับเงิน"), numberColumn("cash_amount", "เงินสด"),
		numberColumn("transfer_amount", "เงินโอน"), numberColumn("card_amount", "บัตร"), numberColumn("chq_amount", "เช็ค"), numberColumn("coupon_amount", "คูปอง"),
	},
	report.CashBankPayments: {
		dateColumn("doc_date", "วันที่"), textColumn("doc_time", "เวลา"), textColumn("doc_no", "เลขที่เอกสาร"), textColumn("trans_flag_label", "ประเภทรายการ"),
		textColumn("ap_ar_code", "รหัสผู้จำหน่าย"), textColumn("ap_ar_name", "ผู้รับ/รายละเอียด"), numberColumn("total_amount", "ยอดจ่ายเงิน"), numberColumn("cash_amount", "เงินสด"),
		numberColumn("transfer_amount", "เงินโอน"), numberColumn("card_amount", "บัตร"), numberColumn("chq_amount", "เช็ค"), numberColumn("petty_cash_amount", "เงินสดย่อย"),
	},
}

// exportNameColumns are the columns that hold a customer's or supplier's name. A token that may not see names gets a
// stable code in their place, the same code the rest of the assistant's answers use.
var exportNameColumns = map[report.Key][]string{
	report.SalesGoodsServices:      {"cust_name"},
	report.PurchaseGoodsPayables:   {"cust_name"},
	report.GrossProfitByARCustomer: {"ar_detail"},
	report.PurchaseFrequency:       {"cust_name"},
	report.CustomerRFM:             {"cust_name"},
	report.ARAging:                 {"cust_name"},
	report.ARCustomerMovement:      {"cust_name"},
	report.ARDebtReceipt:           {"cust_name"},
	report.CashBankReceipts:        {"ap_ar_name"},
	report.CashBankPayments:        {"ap_ar_name"},
}

// exportNotes say, per report, what the rows are, so a file does not get read as something it is not.
var exportNotes = map[report.Key]string{
	report.SalesGoodsServices:    "ข้อมูลมีสองแบบ แถวระดับเอกสาร (ยอดขายรายใบ รวม VAT ใบคืนติดลบ) และแถวระดับรายการสินค้า (มูลค่ารายการ) ยอดขายของรายงานคือผลรวมของแถวระดับเอกสาร ผลรวมของรายการสินค้าอาจต่างได้เมื่อมีส่วนลดท้ายบิลหรือ VAT",
	report.PurchaseGoodsPayables: "ข้อมูลมีสองแบบ แถวระดับเอกสาร (ยอดซื้อรายใบ) และแถวระดับรายการสินค้า (มูลค่ารายการ) ยอดซื้อของรายงานคือผลรวมของแถวระดับเอกสาร ผลรวมของรายการสินค้าอาจต่างได้เมื่อมีส่วนลดท้ายบิลหรือ VAT",
}
