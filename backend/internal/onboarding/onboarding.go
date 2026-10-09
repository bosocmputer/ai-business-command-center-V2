// Package onboarding checks whether a shop is ready for the assistant and the reports: its configuration in AI-BCC, that
// every approved report query runs on its SML, which document types the shop uses against the ones the reports count, and
// the gaps in its data that change what the numbers can say (no due dates, no reorder points). Everything it reads is a
// count or an aggregate; it never prints a name, a phone number or an amount per customer.
package onboarding

import (
	"context"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/thaifmt"
	"github.com/google/uuid"
)

type Status string

const (
	Pass Status = "PASS"
	Warn Status = "WARN"
	Fail Status = "FAIL"
	Info Status = "INFO"
)

type Item struct {
	Area   string `json:"area"`
	Key    string `json:"key"`
	Status Status `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

type Result struct {
	TenantID    uuid.UUID `json:"tenantId"`
	GeneratedAt time.Time `json:"generatedAt"`
	Items       []Item    `json:"items"`
}

// Counts says how many items there are of each status.
func (result Result) Counts() map[Status]int {
	counts := map[Status]int{}
	for _, item := range result.Items {
		counts[item.Status]++
	}
	return counts
}

// Ready is true when nothing failed. Warnings are for people to read.
func (result Result) Ready() bool { return result.Counts()[Fail] == 0 }

type MasterState struct {
	Rows     int
	SyncedAt *time.Time
	Status   string
}

// Facts is the shop's configuration in AI-BCC.
type Facts struct {
	Found                    bool
	Active                   bool
	AccessEndsAt             time.Time
	Timezone                 string
	SMLReadiness             string
	SMLTestedAt              *time.Time
	ActiveRecipients         int
	VerifiedRecipients       int
	RecipientsWithoutReports int
	ActiveSchedules          int
	AssistantTokens          int
	AssistantMembers         int
	MasterCopies             map[string]MasterState
}

type FactsStore interface {
	Facts(ctx context.Context, tenantID uuid.UUID, now time.Time) (Facts, error)
}

// SQL runs one read-only statement on the shop's SML and returns its rows.
type SQL interface {
	Query(ctx context.Context, statement string) ([]map[string]string, error)
}

type Checker struct {
	Store FactsStore
	SQL   SQL
	Now   func() time.Time
	// SlowAfter is the time from which a report query is flagged as slow.
	SlowAfter time.Duration
}

func NewChecker(store FactsStore, sql SQL, now func() time.Time) *Checker {
	return &Checker{Store: store, SQL: sql, Now: now, SlowAfter: 20 * time.Second}
}

// Run does every check. A check that cannot run reports why and the rest still run.
func (checker *Checker) Run(ctx context.Context, tenantID uuid.UUID) Result {
	now := checker.Now()
	result := Result{TenantID: tenantID, GeneratedAt: now}
	add := func(item Item) { result.Items = append(result.Items, item) }
	facts, err := checker.Store.Facts(ctx, tenantID, now)
	if err != nil || !facts.Found {
		add(Item{Area: "ตั้งค่า", Key: "tenant", Status: Fail, Title: "อ่านการตั้งค่าของร้านไม่ได้", Detail: "ไม่พบร้านนี้ หรืออ่านฐานข้อมูลไม่สำเร็จ"})
		return result
	}
	for _, item := range configurationItems(facts, now) {
		add(item)
	}
	if facts.SMLReadiness != "READY" {
		add(Item{Area: "ระบบของร้าน (SML)", Key: "sml", Status: Fail, Title: "ยังไม่ได้ตรวจการเชื่อมต่อระบบของร้านให้ผ่าน", Detail: "ทดสอบการเชื่อมต่อ SML ในหน้าร้านให้ผ่านก่อน แล้วรันการตรวจนี้อีกครั้ง จึงยังไม่ตรวจรายงานและข้อมูล"})
		return result
	}
	location, locErr := time.LoadLocation(facts.Timezone)
	if locErr != nil {
		location = time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	today := now.In(location)
	period := report.Period{Preset: report.Custom, DateFrom: today.AddDate(0, 0, -7).Format(time.DateOnly), DateTo: today.AddDate(0, 0, -1).Format(time.DateOnly)}
	metrics := checker.reportItems(ctx, period, &result)
	for _, item := range checker.coverageItems(ctx, period, today) {
		add(item)
	}
	for _, item := range checker.dataItems(ctx, metrics) {
		add(item)
	}
	return result
}

func configurationItems(facts Facts, now time.Time) []Item {
	area := "ตั้งค่า"
	var items []Item
	switch {
	case !facts.Active:
		items = append(items, Item{Area: area, Key: "tenant", Status: Fail, Title: "ร้านนี้ไม่ได้อยู่ในสถานะใช้งาน"})
	case !facts.AccessEndsAt.After(now):
		items = append(items, Item{Area: area, Key: "tenant", Status: Fail, Title: "สิทธิ์ใช้งานของร้านหมดอายุแล้ว", Detail: "หมดอายุเมื่อ " + facts.AccessEndsAt.Format(time.DateOnly)})
	default:
		items = append(items, Item{Area: area, Key: "tenant", Status: Pass, Title: "ร้านใช้งานได้ถึง " + facts.AccessEndsAt.Format(time.DateOnly)})
	}
	if facts.Timezone == "" {
		items = append(items, Item{Area: area, Key: "timezone", Status: Fail, Title: "ยังไม่ได้ตั้งเขตเวลาของร้าน"})
	} else {
		items = append(items, Item{Area: area, Key: "timezone", Status: Pass, Title: "เขตเวลา " + facts.Timezone})
	}
	switch {
	case facts.SMLReadiness != "READY":
		// reported by Run, which then stops
	case facts.SMLTestedAt == nil || now.Sub(*facts.SMLTestedAt) > 30*24*time.Hour:
		items = append(items, Item{Area: area, Key: "sml_tested", Status: Warn, Title: "การเชื่อมต่อ SML ไม่ได้ทดสอบมานานเกิน 30 วัน", Detail: "ทดสอบซ้ำในหน้าร้าน"})
	default:
		items = append(items, Item{Area: "ระบบของร้าน (SML)", Key: "sml", Status: Pass, Title: "การเชื่อมต่อ SML ผ่านการทดสอบ"})
	}
	switch {
	case facts.ActiveRecipients == 0:
		items = append(items, Item{Area: "ผู้รับ", Key: "recipients", Status: Warn, Title: "ยังไม่มีผู้รับที่ใช้งานอยู่", Detail: "เชิญเจ้าของร้านผ่านลิงก์เชิญแล้วให้เขายืนยัน LINE"})
	case facts.VerifiedRecipients == 0:
		items = append(items, Item{Area: "ผู้รับ", Key: "recipients", Status: Warn, Title: fmt.Sprintf("มีผู้รับ %d คน แต่ยังไม่มีใครยืนยันตัวตน", facts.ActiveRecipients)})
	default:
		items = append(items, Item{Area: "ผู้รับ", Key: "recipients", Status: Pass, Title: fmt.Sprintf("ผู้รับที่ใช้งาน %d คน ยืนยันตัวตนแล้ว %d คน", facts.ActiveRecipients, facts.VerifiedRecipients)})
	}
	if facts.RecipientsWithoutReports > 0 {
		items = append(items, Item{Area: "ผู้รับ", Key: "permissions", Status: Warn, Title: fmt.Sprintf("ผู้รับ %d คนยังไม่ได้รับสิทธิ์ดูรายงานใดเลย", facts.RecipientsWithoutReports), Detail: "ตั้งสิทธิ์รายงานในหน้าผู้รับ"})
	}
	if facts.ActiveSchedules == 0 {
		items = append(items, Item{Area: "การส่ง", Key: "schedule", Status: Warn, Title: "ยังไม่มีตารางส่งการ์ดที่เปิดใช้งาน", Detail: "ตั้งเวลาส่งหลังจากผู้รับพร้อมและทดสอบส่งสำเร็จ"})
	} else {
		items = append(items, Item{Area: "การส่ง", Key: "schedule", Status: Pass, Title: fmt.Sprintf("ตารางส่งที่เปิดใช้งาน %d ตาราง", facts.ActiveSchedules)})
	}
	if facts.AssistantTokens == 0 {
		items = append(items, Item{Area: "ผู้ช่วย AI", Key: "assistant", Status: Info, Title: "ยังไม่ได้เปิดผู้ช่วย AI ให้ร้านนี้", Detail: "เปิดสวิตช์คุยกับผู้ช่วยของผู้รับ ออก token แล้วตั้งค่า Hermes (ดู assistant/README.md)"})
	} else {
		items = append(items, Item{Area: "ผู้ช่วย AI", Key: "assistant", Status: Pass, Title: fmt.Sprintf("เปิดผู้ช่วยแล้ว token ที่ใช้งาน %d รายการ ผู้รับที่คุยได้ %d คน", facts.AssistantTokens, facts.AssistantMembers)})
	}
	for _, kind := range []string{"CUSTOMER", "SUPPLIER", "ITEM"} {
		state, ok := facts.MasterCopies[kind]
		label := map[string]string{"CUSTOMER": "ลูกค้า", "SUPPLIER": "ผู้จำหน่าย", "ITEM": "สินค้า"}[kind]
		switch {
		case !ok || state.SyncedAt == nil:
			items = append(items, Item{Area: "ผู้ช่วย AI", Key: "master_" + strings.ToLower(kind), Status: Info, Title: "ยังไม่มีสำเนาข้อมูลหลัก" + label, Detail: "ระบบคัดลอกให้เองวันละครั้งตอนเช้าเมื่อเปิดผู้ช่วยแล้ว"})
		case state.Status == "ERROR":
			items = append(items, Item{Area: "ผู้ช่วย AI", Key: "master_" + strings.ToLower(kind), Status: Warn, Title: fmt.Sprintf("สำเนาข้อมูลหลัก%s ครั้งล่าสุดล้มเหลว (มี %d รายการจากครั้งก่อน)", label, state.Rows)})
		default:
			items = append(items, Item{Area: "ผู้ช่วย AI", Key: "master_" + strings.ToLower(kind), Status: Pass, Title: fmt.Sprintf("สำเนาข้อมูลหลัก%s %d รายการ", label, state.Rows)})
		}
	}
	return items
}

// metricRow is what the report queries tell about the shop, read from the summary rows.
type metrics map[report.Key]map[string]string

func (checker *Checker) reportItems(ctx context.Context, period report.Period, result *Result) metrics {
	found := metrics{}
	for _, definition := range report.Definitions() {
		if definition.Status != report.StatusActive {
			continue
		}
		plan, err := report.BuildQueryPlanForProjection(definition.Key, period, report.ResultSummary)
		if err != nil {
			result.Items = append(result.Items, Item{Area: "รายงาน", Key: "report_" + string(definition.Key), Status: Fail, Title: definition.LabelTH + ": สร้างคำสั่งตรวจไม่ได้"})
			continue
		}
		var slowest time.Duration
		var failure string
		collected := map[string]string{}
		for _, step := range plan.Steps {
			statement, renderErr := report.RenderSQL(step.Query)
			if renderErr != nil {
				failure = "RENDER_FAILED"
				break
			}
			started := time.Now()
			rows, queryErr := checker.SQL.Query(ctx, statement)
			if elapsed := time.Since(started); elapsed > slowest {
				slowest = elapsed
			}
			if queryErr != nil {
				failure = safeCode(queryErr)
				break
			}
			for _, row := range rows {
				for key, value := range row {
					if strings.HasPrefix(key, "_metric_") && value != "" {
						collected[strings.TrimPrefix(key, "_metric_")] = value
					}
				}
			}
		}
		item := Item{Area: "รายงาน", Key: "report_" + string(definition.Key)}
		switch {
		case failure != "":
			item.Status, item.Title, item.Detail = Fail, definition.LabelTH+": รันคำสั่งบนระบบร้านไม่สำเร็จ", "รหัส "+failure+" ตรวจว่าตาราง/ฟิลด์ที่รายงานใช้มีในระบบร้าน"
		case checker.SlowAfter > 0 && slowest >= checker.SlowAfter:
			item.Status, item.Title, item.Detail = Warn, definition.LabelTH+": ผ่าน แต่ช้า", fmt.Sprintf("ใช้เวลา %.0f วินาที ให้วัดขนาดและตั้งโหมดรายงานของร้าน (แท็บโหมดรายงาน)", slowest.Seconds())
		default:
			item.Status, item.Title, item.Detail = Pass, definition.LabelTH+": ผ่าน", fmt.Sprintf("%.1f วินาที", slowest.Seconds())
		}
		result.Items = append(result.Items, item)
		found[definition.Key] = collected
	}
	return found
}

func safeCode(err error) string {
	text := strings.TrimSpace(err.Error())
	if len(text) > 40 || strings.ContainsAny(text, " \n\t:") {
		return "SML_QUERY_FAILED"
	}
	return text
}

var transFlagPattern = regexp.MustCompile(`trans_flag\s*(?:=\s*(\d+)|in\s*\(([\d,\s]+)\))`)

// countedFlags are the document types the approved reports count, found in their own SQL.
func countedFlags(period report.Period) map[int]struct{} {
	flags := map[int]struct{}{}
	for _, key := range report.Keys() {
		for _, projection := range []report.ResultKind{report.ResultDetail, report.ResultSummary} {
			plan, err := report.BuildQueryPlanForProjection(key, period, projection)
			if err != nil {
				continue
			}
			for _, step := range plan.Steps {
				for _, match := range transFlagPattern.FindAllStringSubmatch(strings.ToLower(step.Query.SQL), -1) {
					list := match[1]
					if list == "" {
						list = match[2]
					}
					for _, part := range strings.Split(list, ",") {
						if number, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
							flags[number] = struct{}{}
						}
					}
				}
			}
		}
	}
	return flags
}

const flagsWithLabelSQL = `select t.trans_flag as flag, trans_flag(t.trans_flag) as label, count(*) as docs from ic_trans t where t.doc_date >= current_date - 365 and coalesce(t.last_status, 0) = 0 group by t.trans_flag order by docs desc limit 80`

// The cash and bank reports read the cash book (cb_trans), not ic_trans, so the document types they count are the cash book's:
// every payment (pay_type 2) and every receipt (pay_type 1) except code 144.
const cashBookFlagsSQL = `select cb.trans_flag as flag, cb.pay_type as pay_type, count(*) as docs, round(sum(coalesce(cb.total_amount, 0))::numeric, 0) as total from cb_trans cb where cb.doc_date >= current_date - 365 and cb.status = 0 group by 1, 2`

// internalCashFlags are cash book rows that move money between the shop's own accounts (deposit, withdrawal, petty cash). The
// reports count them as money in or out, as the definition says, but they are not income or spending.
var internalCashFlags = map[int]bool{401: true, 402: true, 403: true, 301: true, 302: true, 303: true, 423: true}

const flagsSQL = `select t.trans_flag as flag, '' as label, count(*) as docs from ic_trans t where t.doc_date >= current_date - 365 and coalesce(t.last_status, 0) = 0 group by t.trans_flag order by docs desc limit 80`

func (checker *Checker) coverageItems(ctx context.Context, period report.Period, _ time.Time) []Item {
	area := "ประเภทเอกสาร"
	rows, err := checker.SQL.Query(ctx, flagsWithLabelSQL)
	if err != nil {
		rows, err = checker.SQL.Query(ctx, flagsSQL)
	}
	if err != nil {
		return []Item{{Area: area, Key: "doc_types", Status: Warn, Title: "อ่านประเภทเอกสารของร้านไม่ได้", Detail: "รหัส " + safeCode(err)}}
	}
	counted := countedFlags(period)
	cashRows, cashErr := checker.SQL.Query(ctx, cashBookFlagsSQL)
	var internalDocs, internalTotal int
	var internalParts []string
	if cashErr == nil {
		for _, row := range cashRows {
			flag, flagErr := strconv.Atoi(strings.TrimSpace(row["flag"]))
			payType := strings.TrimSpace(row["pay_type"])
			if flagErr != nil {
				continue
			}
			if payType == "2" || payType == "1" && flag != 144 {
				counted[flag] = struct{}{}
			}
			if internalCashFlags[flag] {
				docs, _ := strconv.Atoi(strings.TrimSpace(row["docs"]))
				total, _ := strconv.Atoi(strings.TrimSpace(strings.Split(row["total"], ".")[0]))
				internalDocs += docs
				internalTotal += total
				internalParts = append(internalParts, fmt.Sprintf("%s %d ใบ", flagName(flag, ""), docs))
			}
		}
	}
	type present struct {
		flag  int
		label string
		docs  int
	}
	var used, unused []present
	total, usedDocs := 0, 0
	seen := map[int]struct{}{}
	for _, row := range rows {
		flag, flagErr := strconv.Atoi(strings.TrimSpace(row["flag"]))
		docs, docsErr := strconv.Atoi(strings.TrimSpace(row["docs"]))
		if flagErr != nil || docsErr != nil {
			continue
		}
		seen[flag] = struct{}{}
		total += docs
		entry := present{flag: flag, label: strings.TrimSpace(row["label"]), docs: docs}
		if _, ok := counted[flag]; ok {
			used = append(used, entry)
			usedDocs += docs
		} else {
			unused = append(unused, entry)
		}
	}
	items := []Item{{Area: area, Key: "doc_types_used", Status: Info, Title: fmt.Sprintf("เอกสารใน 12 เดือนที่ผ่านมา %d ใบ รายงานนับประเภทที่ร้านใช้อยู่ %d ประเภท (%d ใบ)", total, len(used), usedDocs)}}
	sort.Slice(unused, func(i, j int) bool { return unused[i].docs > unused[j].docs })
	if len(unused) > 0 {
		parts := make([]string, 0, 8)
		heavy := false
		for index, entry := range unused {
			if index >= 8 {
				break
			}
			name := flagName(entry.flag, entry.label)
			parts = append(parts, fmt.Sprintf("รหัส %d %s %d ใบ", entry.flag, name, entry.docs))
			if total > 0 && entry.docs*10 >= total {
				heavy = true
			}
		}
		status := Info
		if heavy {
			status = Warn
		}
		items = append(items, Item{Area: area, Key: "doc_types_uncounted", Status: status, Title: "ประเภทเอกสารที่ร้านมี แต่ไม่มีรายงานไหนนับ",
			Detail: strings.Join(parts, " · ") + " ให้ถามเจ้าของว่าประเภทไหนควรนับเป็นยอดขาย/ซื้อ/รับ/จ่าย (ถ้ามีประเภทของร้านเองที่ใช้บ่อย รายงานอาจขาดยอด)"})
	}
	var missing []string
	for flag := range counted {
		if _, ok := seen[flag]; !ok {
			missing = append(missing, strconv.Itoa(flag))
		}
	}
	if internalDocs > 0 {
		sort.Strings(internalParts)
		items = append(items, Item{Area: area, Key: "internal_cash", Status: Info, Title: fmt.Sprintf("ในรายงานรับเงิน/จ่ายเงิน มีรายการย้ายเงินภายในร้านเอง %d ใบ รวม %s บาท ใน 12 เดือน", internalDocs, thaifmt.Group(strconv.Itoa(internalTotal))),
			Detail: strings.Join(internalParts, " · ") + " รายงานนับตามนิยาม (ทุกรายการในสมุดเงินสด/ธนาคาร) ยอดเหล่านี้ไม่ใช่รายได้หรือรายจ่ายจริง ควรอ่านยอดเงินรับ/จ่ายโดยรู้ข้อนี้"})
	}
	sort.Strings(missing)
	if len(missing) > 0 && len(missing) <= 40 {
		items = append(items, Item{Area: area, Key: "doc_types_absent", Status: Info, Title: fmt.Sprintf("รายงานนับ %d ประเภทที่ร้านไม่มีเอกสารเลยใน 12 เดือน", len(missing)), Detail: "รหัส " + strings.Join(missing, ", ")})
	}
	return items
}

const itemsSQL = `select count(*) filter (where coalesce(item_type, 0) <> 5) as items, count(*) filter (where coalesce(supplier_code, '') <> '') as with_supplier, count(*) filter (where coalesce(balance_qty, 0) < 0) as negative_stock, (select count(distinct ic_code) from ic_inventory_detail where coalesce(purchase_point, 0) > 0) as with_reorder_point from ic_inventory`
const priceSQL = `select count(*) as price_rows from ic_inventory_price`
const customersSQL = `select count(*) as customers, count(*) filter (where coalesce(telephone, '') <> '') as with_phone from ar_customer`

func (checker *Checker) dataItems(ctx context.Context, found metrics) []Item {
	area := "ข้อมูลของร้าน"
	var items []Item
	if aging, ok := found[report.ARAging]; ok && aging["total_balance"] != "" {
		total, _ := ratOf(aging["total_balance"])
		noDue, _ := ratOf(aging["no_due_date_amount"])
		overdue, _ := ratOf(aging["overdue_amount"])
		switch {
		case total == nil || total.Sign() <= 0:
			items = append(items, Item{Area: area, Key: "ar_due_dates", Status: Info, Title: "ไม่มียอดลูกหนี้ค้าง"})
		case noDue != nil && new(big.Rat).Mul(noDue, big.NewRat(2, 1)).Cmp(total) >= 0:
			share := new(big.Rat).Quo(new(big.Rat).Mul(noDue, big.NewRat(100, 1)), total)
			items = append(items, Item{Area: area, Key: "ar_due_dates", Status: Warn, Title: fmt.Sprintf("ลูกหนี้ %s%% ไม่มีวันครบกำหนดในระบบ", share.FloatString(0)),
				Detail: "ยอดเลยกำหนดจะไม่รวมเอกสารพวกนี้ และเลขาจะไม่ร่างทวงให้ ควรแจ้งเจ้าของให้บันทึกวันครบกำหนดหรือเครดิตวัน (ยอดเลยกำหนดที่เห็นตอนนี้ " + overdueText(overdue) + " บาท)"})
		default:
			items = append(items, Item{Area: area, Key: "ar_due_dates", Status: Pass, Title: "ลูกหนี้ส่วนใหญ่มีวันครบกำหนด"})
		}
	}
	if rows, err := checker.SQL.Query(ctx, itemsSQL); err != nil {
		items = append(items, Item{Area: area, Key: "items", Status: Warn, Title: "อ่านข้อมูลสินค้าไม่ได้", Detail: "รหัส " + safeCode(err)})
	} else if len(rows) == 1 {
		row := rows[0]
		itemCount, _ := strconv.Atoi(row["items"])
		withPoint, _ := strconv.Atoi(row["with_reorder_point"])
		withSupplier, _ := strconv.Atoi(row["with_supplier"])
		negative, _ := strconv.Atoi(row["negative_stock"])
		switch {
		case itemCount > 0 && withPoint == 0:
			items = append(items, Item{Area: area, Key: "reorder_points", Status: Warn, Title: fmt.Sprintf("ไม่มีสินค้าไหนตั้งจุดสั่งซื้อ (สินค้า %d รายการ)", itemCount), Detail: "รายงานและการเตือนสินค้าถึงจุดสั่งซื้อ รวมถึงร่างรายการสั่งซื้อ จะว่างเสมอจนกว่าร้านจะตั้งจุดสั่งซื้อ"})
		case itemCount > 0 && withPoint*5 < itemCount:
			items = append(items, Item{Area: area, Key: "reorder_points", Status: Info, Title: fmt.Sprintf("ตั้งจุดสั่งซื้อไว้ %d จาก %d รายการ", withPoint, itemCount)})
		default:
			items = append(items, Item{Area: area, Key: "reorder_points", Status: Pass, Title: fmt.Sprintf("ตั้งจุดสั่งซื้อไว้ %d จาก %d รายการ", withPoint, itemCount)})
		}
		if itemCount > 0 && withSupplier == 0 {
			items = append(items, Item{Area: area, Key: "item_suppliers", Status: Info, Title: "สินค้าไม่มีรหัสผู้จำหน่ายประจำ", Detail: "ร่างรายการสั่งซื้อจึงจัดกลุ่มตามผู้จำหน่ายไม่ได้"})
		}
		if negative > 0 {
			items = append(items, Item{Area: area, Key: "negative_stock", Status: Warn, Title: fmt.Sprintf("สินค้า %d รายการมียอดคงเหลือติดลบ", negative), Detail: "มูลค่าสต็อกอาจคลาดเคลื่อน ควรให้ฝ่ายคลังตรวจ"})
		}
	}
	if rows, err := checker.SQL.Query(ctx, priceSQL); err == nil && len(rows) == 1 {
		if count, _ := strconv.Atoi(rows[0]["price_rows"]); count == 0 {
			items = append(items, Item{Area: area, Key: "prices", Status: Info, Title: "ตารางราคาขายว่างเปล่า", Detail: "ค้นราคาสินค้าให้เจ้าของยังไม่ได้"})
		}
	}
	if rows, err := checker.SQL.Query(ctx, customersSQL); err == nil && len(rows) == 1 {
		customers, _ := strconv.Atoi(rows[0]["customers"])
		phones, _ := strconv.Atoi(rows[0]["with_phone"])
		if customers > 0 && phones*4 < customers {
			items = append(items, Item{Area: area, Key: "customer_phones", Status: Info, Title: fmt.Sprintf("ลูกค้ามีเบอร์โทรในช่องเบอร์ %d จาก %d ราย", phones, customers), Detail: "ค้นเบอร์โทรลูกค้าให้เจ้าของได้เฉพาะรายที่บันทึกไว้"})
		}
	}
	return items
}

func ratOf(text string) (*big.Rat, bool) {
	value, ok := new(big.Rat).SetString(strings.TrimSpace(text))
	return value, ok
}

func overdueText(value *big.Rat) string {
	if value == nil {
		return "0.00"
	}
	return value.FloatString(2)
}

// flagName gives the Thai name of a document code: the shop's own SML name when it has one, else the project's reference table.
func flagName(flag int, smlLabel string) string {
	if name, ok := flagNames[flag]; ok {
		return name
	}
	if strings.TrimSpace(smlLabel) != "" {
		return strings.TrimSpace(smlLabel)
	}
	return "ไม่ทราบชื่อ"
}
