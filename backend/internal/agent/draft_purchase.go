package agent

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/thaifmt"
)

const (
	MessageNothingToOrder     = "ตอนนี้ไม่มีสินค้าถึงจุดสั่งซื้อ จึงไม่มีรายการให้ร่างสั่งซื้อ"
	MessagePurchaseNoSupplier = "รายงานไม่มีข้อมูลผู้จำหน่ายของสินค้า ร่างนี้จึงเขียนถึง “ผู้จำหน่าย” กว้าง ๆ เจ้าของเลือกผู้จำหน่ายและแก้ชื่อเอง"
	MessagePurchaseQuantity   = "จำนวนในร่างคือจำนวนที่ขาดจากจุดสั่งซื้อ (จุดสั่งซื้อ − คงเหลือ) ไม่ใช่จำนวนที่แนะนำให้สั่ง เจ้าของปรับจำนวนก่อนส่ง"
	MessagePurchaseOnOrder    = "รายการที่มียอดค้างรับจากใบสั่งซื้อเดิม อาจไม่ต้องสั่งเพิ่ม ให้ตรวจก่อนส่ง"
)

// PurchaseItem is one item below its reorder point, as shown in a purchase draft.
type PurchaseItem struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	Unit         string `json:"unit,omitempty"`
	ShortageQty  string `json:"shortageQty"`
	BalanceQty   string `json:"balanceQty"`
	ReorderPoint string `json:"reorderPoint"`
	OnOrderQty   string `json:"onOrderQty"`
}

type PurchaseDraftResponse struct {
	Status            string         `json:"status"`
	Message           string         `json:"message,omitempty"`
	ItemsBelowPoint   int            `json:"itemsBelowPoint,omitempty"`
	ItemsListed       int            `json:"itemsListed,omitempty"`
	Items             []PurchaseItem `json:"items,omitempty"`
	Draft             string         `json:"draft,omitempty"`
	CollectedAt       string         `json:"collectedAt,omitempty"`
	Freshness         string         `json:"freshness,omitempty"`
	Notes             []string       `json:"notes,omitempty"`
	RetryAfterSeconds int            `json:"retryAfterSeconds,omitempty"`
}

// DraftPurchaseOrder writes a purchase list from the reorder report: the items below their reorder point, the most short
// first, up to twenty. AI-BCC writes every figure; nothing is sent, nothing is written to SML. A recipient without the
// reorder report gets the uniform "no data" answer.
func (service *Service) DraftPurchaseOrder(ctx context.Context, principal Principal) (PurchaseDraftResponse, error) {
	started := service.now()
	now := started.UTC()
	definition, err := service.authorize(ctx, principal, string(report.StockReorder), now)
	if err != nil {
		service.record(ctx, principal, ToolDraft, "", report.Period{}, OutcomeNoData, started, nil)
		return PurchaseDraftResponse{}, err
	}
	key := string(definition.Key)
	period, err := resolvePeriod(definition, locationOf(principal), now, "", "")
	if err != nil {
		service.record(ctx, principal, ToolDraft, key, report.Period{}, OutcomeInvalidPeriod, started, nil)
		return PurchaseDraftResponse{}, err
	}
	snapshot := service.snapshot(ctx, principal, definition, period, now)
	if snapshot.state != stateReady {
		service.record(ctx, principal, ToolDraft, key, period, outcomeOf(snapshot.state), started, snapshot.runID())
		return PurchaseDraftResponse{Status: snapshot.state.status(), Message: snapshot.message(), RetryAfterSeconds: snapshot.retryAfter}, nil
	}
	dashboard := snapshot.snapshot.Dashboard
	response := PurchaseDraftResponse{Freshness: "FRESH"}
	if snapshot.stale {
		response.Freshness = "STALE"
	}
	if finished := snapshot.snapshot.SourceFinishedAt; finished != nil {
		response.CollectedAt = finished.In(locationOf(principal)).Format(time.RFC3339)
	}
	for _, metric := range dashboard.KPIs {
		if metric.Key == "reorder_item_count" {
			if count, ok := new(big.Rat).SetString(metric.Value); ok && count.IsInt() {
				response.ItemsBelowPoint = int(count.Num().Int64())
			}
		}
	}
	items := purchaseItems(dashboard)
	response.Items, response.ItemsListed = items, len(items)
	if len(items) == 0 {
		response.Status, response.Message = "NOTHING_TO_ORDER", MessageNothingToOrder
		if response.ItemsBelowPoint > 0 { // the report says there are items but this snapshot carries none of them (an older snapshot)
			response.Status, response.Message = "UNAVAILABLE", MessageUnavailable
		}
		service.record(ctx, principal, ToolDraft, key, period, OutcomeOK, started, snapshot.runID())
		return response, nil
	}
	response.Status = "READY"
	response.ItemsBelowPoint = max(response.ItemsBelowPoint, len(items))
	response.Draft = purchaseDraft(principal.ShopName, items, locationOf(principal), now)
	response.Notes = []string{MessageDraftNote, MessagePurchaseNoSupplier, MessagePurchaseQuantity}
	for _, item := range items {
		if onOrder, ok := new(big.Rat).SetString(item.OnOrderQty); ok && onOrder.Sign() > 0 {
			response.Notes = append(response.Notes, MessagePurchaseOnOrder)
			break
		}
	}
	if response.ItemsBelowPoint > len(items) {
		response.Notes = append(response.Notes, fmt.Sprintf("มีสินค้าถึงจุดสั่งซื้อ %d รายการ ร่างนี้แสดง %d รายการที่ขาดมากสุด", response.ItemsBelowPoint, len(items)))
	}
	if snapshot.stale {
		response.Notes = append(response.Notes, MessageDraftStale)
	}
	service.record(ctx, principal, ToolDraft, key, period, OutcomeOK, started, snapshot.runID())
	return response, nil
}

// purchaseItems reads the assistant-only reorder list the reorder report builds (see report.buildReorderItems).
func purchaseItems(dashboard report.Dashboard) []PurchaseItem {
	for _, visualization := range dashboard.Visualizations {
		if visualization.Key != "agent_reorder_items" || len(visualization.Series) < 4 {
			continue
		}
		short, balance, point, onOrder := visualization.Series[0], visualization.Series[1], visualization.Series[2], visualization.Series[3]
		var list []PurchaseItem
		for index, code := range visualization.Categories {
			if index >= len(short.Values) || index >= len(short.PointLabels) || index >= len(balance.Values) || index >= len(balance.PointLabels) || index >= len(point.Values) || index >= len(onOrder.Values) {
				continue
			}
			shortage, ok := new(big.Rat).SetString(short.Values[index])
			if !ok || shortage.Sign() <= 0 || strings.TrimSpace(code) == "" {
				continue
			}
			list = append(list, PurchaseItem{Code: strings.TrimSpace(code), Name: short.PointLabels[index], Unit: balance.PointLabels[index],
				ShortageQty: short.Values[index], BalanceQty: balance.Values[index], ReorderPoint: point.Values[index], OnOrderQty: onOrder.Values[index]})
		}
		return list
	}
	return nil
}

func qtyText(text string) string {
	if value, ok := new(big.Rat).SetString(text); ok {
		return thaifmt.Qty(value)
	}
	return text
}

func purchaseDraft(shop string, items []PurchaseItem, location *time.Location, now time.Time) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "ขอสั่งซื้อสินค้า จาก %s\nวันที่ %s\n\nเรียน ผู้จำหน่าย\n\nรบกวนเสนอราคาและกำหนดส่งสินค้าตามรายการนี้\n\n", shop, thaifmt.Date(now.In(location)))
	for index, item := range items {
		unit := ""
		if item.Unit != "" {
			unit = " " + item.Unit
		}
		fmt.Fprintf(&builder, "%d. %s (รหัส %s) จำนวน %s%s", index+1, item.Name, item.Code, qtyText(item.ShortageQty), unit)
		if onOrder, ok := new(big.Rat).SetString(item.OnOrderQty); ok && onOrder.Sign() > 0 {
			fmt.Fprintf(&builder, " (มียอดค้างรับจากใบสั่งซื้อเดิม %s%s)", qtyText(item.OnOrderQty), unit)
		}
		builder.WriteString("\n")
	}
	fmt.Fprintf(&builder, "\nขอบคุณครับ/ค่ะ\n%s", shop)
	return builder.String()
}
