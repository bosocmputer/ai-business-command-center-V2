package agent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

// A live lookup asks the shop's system one narrow question about one customer or item the owner named ("how much does
// this customer owe right now", "how much of this item is in stock"). The answer is not a report snapshot: it changes with
// the record, so it is read live, kept for a few minutes, limited in how often and how many at once, and logged. The record
// must exist in the shop's master data copy, so no text from outside is ever put into a query as anything but a known code.

type LookupKind string

const (
	LookupCustomerBalance LookupKind = "customer_balance"
	LookupCustomerSales   LookupKind = "customer_recent_sales"
	LookupItemStock       LookupKind = "item_stock"
	// LookupDocument finds one document by the number written on it (an invoice, a purchase or a debt receipt). Unlike the
	// others it asks about no master record: the number is checked for its shape, put into the statement as a quoted literal,
	// and only the kinds of document the recipient has a report for are searched.
	LookupDocument LookupKind = "document"
)

type LookupDef struct {
	Kind   LookupKind
	Label  string
	Master MasterKind
	// Any one of these reports makes the lookup the recipient's business; none of them answers like a missing report.
	Reports []report.Key
}

var lookupCatalog = []LookupDef{
	{Kind: LookupCustomerBalance, Label: "ยอดค้างชำระและเอกสารค้างของลูกค้ารายหนึ่ง", Master: MasterCustomer, Reports: []report.Key{report.ARAging}},
	{Kind: LookupCustomerSales, Label: "ยอดขายและใบขายล่าสุดของลูกค้ารายหนึ่ง", Master: MasterCustomer, Reports: []report.Key{report.SalesGoodsServices}},
	{Kind: LookupDocument, Label: "เอกสารหนึ่งใบตามเลขที่เอกสาร (ใบขาย ใบเพิ่มหนี้ ใบรับคืน ใบซื้อ ใบรับชำระหนี้)", Reports: []report.Key{report.SalesGoodsServices, report.PurchaseGoodsPayables, report.ARDebtReceipt}},
	{Kind: LookupItemStock, Label: "สต็อกคงเหลือ ค้างรับ ค้างส่ง และจองของสินค้ารายการหนึ่ง", Master: MasterItem, Reports: []report.Key{report.StockBalance, report.StockReorder}},
}

func LookupCatalog() []LookupDef { return append([]LookupDef(nil), lookupCatalog...) }

func lookupFor(raw string) (LookupDef, bool) {
	raw = strings.TrimSpace(raw)
	for _, def := range lookupCatalog {
		if string(def.Kind) == raw {
			return def, true
		}
	}
	return LookupDef{}, false
}

// Figure is one number of a lookup, written by the server.
type Figure struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
	Value string `json:"value"`
}

// Table is a short list of rows, for example the oldest open documents.
type Table struct {
	Title   string     `json:"title"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// LookupResult is what the live source returns for one question.
type LookupResult struct {
	Found    bool
	Figures  []Figure
	Tables   []Table
	AsOf     time.Time // when the shop's system was read
	Cached   bool
	Warnings []string
}

// ErrLookupBusy is returned by a live source that has no room for another question right now (too many at once or too
// many this hour for the shop).
var ErrLookupBusy = errors.New("live lookups are busy")

// LiveSource asks the shop's system. The code is a record the master data copy already knows.
type LiveSource interface {
	Lookup(ctx context.Context, tenantID uuid.UUID, kind LookupKind, code, asOfDate string) (LookupResult, error)
}

// DocumentClass is a family of documents that one report covers, so a document is shown only to a recipient who may read it.
type DocumentClass string

const (
	DocumentSales       DocumentClass = "sales"        // codes 44, 46, 48: sales_goods_services
	DocumentPurchase    DocumentClass = "purchase"     // code 12: purchase_goods_payables
	DocumentDebtReceipt DocumentClass = "debt_receipt" // code 239: ar_debt_receipt
)

// DocumentSource is implemented by a live source that can find a document by its number. Without it the document kind
// answers as if there were no such thing.
type DocumentSource interface {
	LookupDocument(ctx context.Context, tenantID uuid.UUID, number string, classes []DocumentClass) (LookupResult, error)
}

var documentNumberShape = regexp.MustCompile(`^[A-Za-z0-9ก-๙][A-Za-z0-9ก-๙._/-]{2,39}$`)

// validDocumentNumber is a document number as people write it: it starts with a letter or digit and has no "..".
func validDocumentNumber(number string) bool {
	return documentNumberShape.MatchString(number) && !strings.Contains(number, "..")
}

func documentClasses(permitted []report.Key) []DocumentClass {
	var classes []DocumentClass
	for _, pair := range []struct {
		key   report.Key
		class DocumentClass
	}{{report.SalesGoodsServices, DocumentSales}, {report.PurchaseGoodsPayables, DocumentPurchase}, {report.ARDebtReceipt, DocumentDebtReceipt}} {
		if contains(permitted, pair.key) {
			classes = append(classes, pair.class)
		}
	}
	return classes
}

var ErrLookupUnavailable = errors.New("live lookups are not available")

// ConfigureLookups turns on the live lookup tool. Without it the tool answers as if there were no such thing.
func (service *Service) ConfigureLookups(source LiveSource) *Service {
	service.live = source
	return service
}

// InvalidLookupError says, in Thai the assistant can pass on, what was wrong with the request.
type InvalidLookupError struct{ Message string }

func (err *InvalidLookupError) Error() string { return "lookup request is not valid" }

const (
	MessageLookupBusy     = "ตอนนี้ค้นข้อมูลสดได้ไม่ทัน (ถามติดกันมากเกินไป) ลองใหม่ในอีกสักครู่"
	MessageLookupNotFound = "ไม่พบรายการนี้ในข้อมูลหลักของร้าน ให้ค้นหาด้วย search_master ก่อนเพื่อได้รหัสที่ถูกต้อง"
	MessageLookupLive     = "ข้อมูลนี้อ่านสดจากระบบของร้านตอนที่ถาม (ไม่ใช่รายงานที่เตรียมไว้) ถ้าเพิ่งถามซ้ำภายในไม่กี่นาทีอาจเป็นผลที่จำไว้"
)

type LookupResponse struct {
	Status  string     `json:"status"`
	Message string     `json:"message,omitempty"`
	Kind    LookupKind `json:"kind"`
	Subject *Subject   `json:"subject,omitempty"`
	Figures []Figure   `json:"figures,omitempty"`
	Tables  []Table    `json:"tables,omitempty"`
	AsOf    string     `json:"asOf,omitempty"`
	Cached  bool       `json:"cached,omitempty"`
	Notes   []string   `json:"notes,omitempty"`
}

// Subject is the record the answer is about.
type Subject struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// LiveLookup answers one narrow question about one known customer or item, live. A lookup the recipient has no report
// for, or a kind that does not exist, answers like a missing report. People need a token that may see names.
func (service *Service) LiveLookup(ctx context.Context, principal Principal, rawKind, code string) (LookupResponse, error) {
	started := service.now()
	now := started.UTC()
	if service.live == nil || service.master == nil {
		return LookupResponse{}, ErrLookupUnavailable
	}
	def, known := lookupFor(rawKind)
	if known {
		permitted, err := service.store.PermittedReports(ctx, principal, now)
		if err != nil {
			return LookupResponse{}, fmt.Errorf("list permitted reports: %w", err)
		}
		known = false
		for _, key := range def.Reports {
			if contains(permitted, key) {
				known = true
				break
			}
		}
	}
	if !known {
		service.record(ctx, principal, ToolLookup, "", report.Period{}, OutcomeNoData, started, nil)
		return LookupResponse{}, ErrNoData
	}
	key := string(def.Kind)
	code = strings.TrimSpace(code)
	if def.Kind == LookupDocument {
		return service.liveDocument(ctx, principal, code, started)
	}
	if code == "" || utf8.RuneCountInString(code) > 80 {
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeInvalidLookup, started, nil)
		return LookupResponse{}, &InvalidLookupError{Message: "ต้องระบุรหัส (code) ของลูกค้าหรือสินค้าที่ได้จากการค้นหา search_master"}
	}
	if def.Master != MasterItem && !principal.NamesVisible {
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeUnavailable, started, nil)
		return LookupResponse{Status: "UNAVAILABLE", Kind: def.Kind, Message: "การดูข้อมูลรายลูกค้าต้องเห็นชื่อลูกค้า แต่การเข้าถึงนี้ปิดการแสดงชื่อไว้"}, nil
	}
	record, found, err := service.master.MasterRecord(ctx, principal.TenantID, def.Master, code)
	if err != nil {
		return LookupResponse{}, fmt.Errorf("read master record: %w", err)
	}
	if !found {
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeNoData, started, nil)
		return LookupResponse{Status: "NOT_FOUND", Kind: def.Kind, Message: MessageLookupNotFound}, nil
	}
	asOf := now.In(locationOf(principal)).Format(time.DateOnly)
	result, err := service.live.Lookup(ctx, principal.TenantID, def.Kind, record.Code, asOf)
	switch {
	case errors.Is(err, ErrLookupBusy):
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeUnavailable, started, nil)
		return LookupResponse{Status: "BUSY", Kind: def.Kind, Message: MessageLookupBusy}, nil
	case err != nil:
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeUnavailable, started, nil)
		return LookupResponse{Status: "UNAVAILABLE", Kind: def.Kind, Message: MessageUnavailable}, nil
	}
	response := LookupResponse{Kind: def.Kind, Subject: &Subject{Code: record.Code, Name: record.Name}, Cached: result.Cached}
	if !result.Found {
		response.Status, response.Message = "NOT_FOUND", MessageLookupNotFound
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeNoData, started, nil)
		return response, nil
	}
	response.Status, response.Figures, response.Tables = "READY", result.Figures, result.Tables
	response.AsOf = result.AsOf.In(locationOf(principal)).Format(time.RFC3339)
	response.Notes = append([]string{MessageLookupLive}, result.Warnings...)
	service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeOK, started, nil)
	return response, nil
}

// liveDocument finds one document by its number among the kinds of document the recipient may read. A number that matches
// nothing, or only a kind the recipient has no report for, answers NOT_FOUND, the same words either way.
func (service *Service) liveDocument(ctx context.Context, principal Principal, number string, started time.Time) (LookupResponse, error) {
	now := started.UTC()
	const key = string(LookupDocument)
	source, ok := service.live.(DocumentSource)
	if !ok {
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeNoData, started, nil)
		return LookupResponse{}, ErrNoData
	}
	if !validDocumentNumber(number) {
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeInvalidLookup, started, nil)
		return LookupResponse{}, &InvalidLookupError{Message: "ต้องระบุเลขที่เอกสารตามที่เขียนบนเอกสาร (ตัวอักษร ตัวเลข และเครื่องหมาย - . / ยาว 3 ถึง 40 ตัว)"}
	}
	permitted, err := service.store.PermittedReports(ctx, principal, now)
	if err != nil {
		return LookupResponse{}, fmt.Errorf("list permitted reports: %w", err)
	}
	result, err := source.LookupDocument(ctx, principal.TenantID, number, documentClasses(permitted))
	switch {
	case errors.Is(err, ErrLookupBusy):
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeUnavailable, started, nil)
		return LookupResponse{Status: "BUSY", Kind: LookupDocument, Message: MessageLookupBusy}, nil
	case err != nil:
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeUnavailable, started, nil)
		return LookupResponse{Status: "UNAVAILABLE", Kind: LookupDocument, Message: MessageUnavailable}, nil
	}
	response := LookupResponse{Kind: LookupDocument, Cached: result.Cached, Subject: &Subject{Code: number}}
	if !result.Found {
		response.Status, response.Message = "NOT_FOUND", MessageDocumentNotFound
		service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeNoData, started, nil)
		return response, nil
	}
	response.Status, response.Figures, response.Tables = "READY", result.Figures, hideNames(result.Tables, principal.NamesVisible)
	response.AsOf = result.AsOf.In(locationOf(principal)).Format(time.RFC3339)
	response.Notes = append([]string{MessageLookupLive}, result.Warnings...)
	service.record(ctx, principal, ToolLookup, key, report.Period{}, OutcomeOK, started, nil)
	return response, nil
}

const MessageDocumentNotFound = "ไม่พบเอกสารเลขที่นี้ในชนิดเอกสารที่ดูได้ (ใบขาย ใบเพิ่มหนี้ ใบรับคืน ใบซื้อ ใบรับชำระหนี้) ตรวจเลขที่ให้ตรงกับที่เขียนบนเอกสาร รวมตัวพิมพ์เล็กใหญ่"

// hideNames blanks the party name column when the token may not see names; the party code stays.
func hideNames(tables []Table, visible bool) []Table {
	if visible {
		return tables
	}
	out := make([]Table, len(tables))
	for index, table := range tables {
		rows := make([][]string, len(table.Rows))
		for r, row := range table.Rows {
			rows[r] = append([]string(nil), row...)
			for c, column := range table.Columns {
				if column == DocumentNameColumn && c < len(rows[r]) {
					rows[r][c] = "(ซ่อนชื่อ)"
				}
			}
		}
		out[index] = Table{Title: table.Title, Columns: table.Columns, Rows: rows}
	}
	return out
}

// DocumentNameColumn is the header of the table column that holds a customer's or supplier's name.
const DocumentNameColumn = "ชื่อคู่ค้า"
