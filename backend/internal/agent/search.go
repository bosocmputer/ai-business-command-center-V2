package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

// The assistant can look a customer, supplier or item up by a name, a code or a phone number the owner says. The copy it
// searches is made by the worker from SML once a day and holds codes, names, phone numbers and units: no business figures.

type MasterKind string

const (
	MasterCustomer MasterKind = "CUSTOMER"
	MasterSupplier MasterKind = "SUPPLIER"
	MasterItem     MasterKind = "ITEM"
)

type MasterMatch struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Phone        string `json:"phone,omitempty"`
	Unit         string `json:"unit,omitempty"`
	SupplierCode string `json:"supplierCode,omitempty"`
}

type MasterResult struct {
	Matches  []MasterMatch
	Total    int
	SyncedAt *time.Time // nil: this shop has no copy of the kind yet
}

type MasterStore interface {
	SearchMaster(ctx context.Context, tenantID uuid.UUID, kind MasterKind, words []string, limit int) (MasterResult, error)
}

var ErrMasterUnavailable = errors.New("master data search is not available")

// ConfigureMaster turns on the search tool. Without it the tool answers as if there were no such thing.
func (service *Service) ConfigureMaster(store MasterStore) *Service {
	service.master = store
	return service
}

// searchReports are the reports that make a kind of record the recipient's business. Seeing none of them, a recipient
// gets the uniform "no data" answer for the kind.
var searchReports = map[MasterKind][]report.Key{
	MasterCustomer: {report.SalesGoodsServices, report.CustomerRFM, report.PurchaseFrequency, report.GrossProfitByARCustomer, report.ARCustomerMovement, report.ARDebtReceipt, report.ARAging},
	MasterSupplier: {report.PurchaseGoodsPayables, report.CashBankPayments},
	MasterItem:     {report.SalesGoodsServices, report.GrossProfitByProduct, report.StockBalance, report.StockReorder, report.PurchaseGoodsPayables},
}

const (
	searchLimit       = 10
	searchStaleAfter  = 3 * 24 * time.Hour
	MessageNotSynced  = "ยังไม่มีสำเนาข้อมูลหลักของร้านนี้ (ระบบคัดลอกจาก SML วันละครั้งตอนเช้า) ลองใหม่ภายหลัง"
	MessageSearchMore = "พบมากกว่าที่แสดง ให้เจ้าของระบุชื่อเพิ่มเพื่อให้แคบลง"
	MessageSearchNone = "ไม่พบรายการที่ตรงกับคำค้น"
)

// InvalidSearchError says, in Thai the assistant can pass on, what was wrong with the request.
type InvalidSearchError struct{ Message string }

func (err *InvalidSearchError) Error() string { return "search request is not valid" }

type SearchResponse struct {
	Status     string        `json:"status"`
	Message    string        `json:"message,omitempty"`
	Kind       MasterKind    `json:"kind,omitempty"`
	Matches    []MasterMatch `json:"matches,omitempty"`
	TotalFound int           `json:"totalFound"`
	SyncedAt   string        `json:"syncedAt,omitempty"`
	Notes      []string      `json:"notes,omitempty"`
}

// SearchMaster looks one kind of record up. People (customers, suppliers) need a token that may see names, and every kind
// needs the permission to read at least one report that concerns it; otherwise the answer is the uniform "no data".
func (service *Service) SearchMaster(ctx context.Context, principal Principal, rawKind, query string) (SearchResponse, error) {
	started := service.now()
	now := started.UTC()
	if service.master == nil {
		return SearchResponse{}, ErrMasterUnavailable
	}
	kind := MasterKind(strings.ToUpper(strings.TrimSpace(rawKind)))
	reports, known := searchReports[kind]
	if known {
		permitted, err := service.store.PermittedReports(ctx, principal, now)
		if err != nil {
			return SearchResponse{}, fmt.Errorf("list permitted reports: %w", err)
		}
		known = false
		for _, key := range reports {
			if contains(permitted, key) {
				known = true
				break
			}
		}
	}
	if !known {
		service.record(ctx, principal, ToolSearch, "", report.Period{}, OutcomeNoData, started, nil)
		return SearchResponse{}, ErrNoData
	}
	words := strings.Fields(strings.ToLower(query))
	total := 0
	for _, word := range words {
		total += utf8.RuneCountInString(word)
	}
	if total < 2 || len(words) > 5 || utf8.RuneCountInString(strings.Join(words, " ")) > 100 {
		service.record(ctx, principal, ToolSearch, strings.ToLower(string(kind)), report.Period{}, OutcomeInvalidSearch, started, nil)
		return SearchResponse{}, &InvalidSearchError{Message: "ต้องระบุคำค้นอย่างน้อย 2 ตัวอักษร (ชื่อ รหัส หรือเบอร์โทร) และไม่เกิน 5 คำ"}
	}
	if kind != MasterItem && !principal.NamesVisible {
		service.record(ctx, principal, ToolSearch, strings.ToLower(string(kind)), report.Period{}, OutcomeUnavailable, started, nil)
		return SearchResponse{Status: "UNAVAILABLE", Kind: kind, Message: "การค้นหาชื่อลูกค้าและผู้จำหน่ายต้องเห็นชื่อ แต่การเข้าถึงนี้ปิดการแสดงชื่อไว้"}, nil
	}
	result, err := service.master.SearchMaster(ctx, principal.TenantID, kind, words, searchLimit)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("search master data: %w", err)
	}
	response := SearchResponse{Kind: kind}
	if result.SyncedAt == nil {
		response.Status, response.Message = "NOT_SYNCED", MessageNotSynced
		service.record(ctx, principal, ToolSearch, strings.ToLower(string(kind)), report.Period{}, OutcomeUnavailable, started, nil)
		return response, nil
	}
	response.Status, response.Matches, response.TotalFound = "READY", result.Matches, result.Total
	response.SyncedAt = result.SyncedAt.In(locationOf(principal)).Format(time.RFC3339)
	switch {
	case result.Total == 0:
		response.Message = MessageSearchNone
	case result.Total > len(result.Matches):
		response.Notes = append(response.Notes, MessageSearchMore)
	}
	if now.Sub(*result.SyncedAt) > searchStaleAfter {
		response.Notes = append(response.Notes, "สำเนาข้อมูลหลักไม่ได้อัปเดตมาหลายวัน ข้อมูลอาจไม่เป็นปัจจุบัน")
	}
	service.record(ctx, principal, ToolSearch, strings.ToLower(string(kind)), report.Period{}, OutcomeOK, started, nil)
	return response, nil
}
