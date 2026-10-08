package agent

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

// An alert rule is something the owner asks to be told about in their own words. Each rule watches one number in
// one report against one threshold the owner chooses. The catalog is fixed in code: the assistant picks from it and
// cannot invent a rule, a report or a recipient.
type AlertRuleKey string

const (
	AlertAROverdue    AlertRuleKey = "ar_overdue"
	AlertAROverYear   AlertRuleKey = "ar_over_year"
	AlertStockReorder AlertRuleKey = "stock_reorder"
	AlertSalesDrop    AlertRuleKey = "sales_drop"
	AlertReceiptsDrop AlertRuleKey = "receipts_drop"
	AlertMarginDrop   AlertRuleKey = "margin_drop"
	// AlertMorningDigest is not a threshold: it is a switch for a short daily summary of the other three figures.
	AlertMorningDigest AlertRuleKey = "morning_digest"
)

type AlertUnit string

const (
	AlertUnitBaht    AlertUnit = "THB"
	AlertUnitCount   AlertUnit = "COUNT"
	AlertUnitPercent AlertUnit = "PERCENT"
	AlertUnitSwitch  AlertUnit = "SWITCH"
	// AlertUnitPoints is a number of percentage points (a margin going from 25% to 20% fell by 5 points).
	AlertUnitPoints AlertUnit = "POINTS"
)

type AlertRuleDef struct {
	Key         AlertRuleKey
	Label       string // what the owner calls it
	Description string // what is watched and when, in plain Thai
	Report      report.Key
	Unit        AlertUnit
	Min, Max    int64
	// Cooldown is how long a rule that is still true stays quiet after it fired, unless it got 10% worse. A rule about one
	// day (yesterday's sales) has none: each day stands alone.
	Cooldown time.Duration
	// Switch rules have no threshold: they are on or off, and the stored threshold is the fixed placeholder "1".
	// They may be set when the recipient can read any one of AnyOf (the others are left out of the message).
	Switch bool
	AnyOf  []report.Key
}

const switchPlaceholder = "1"

// available says whether the recipient may use the rule: the rule's one report, or any of AnyOf for a switch.
func (def AlertRuleDef) available(permitted []report.Key) bool {
	if len(def.AnyOf) == 0 {
		return contains(permitted, def.Report)
	}
	for _, key := range def.AnyOf {
		if contains(permitted, key) {
			return true
		}
	}
	return false
}

const alertCooldown = 3 * 24 * time.Hour

var alertCatalog = []AlertRuleDef{
	{Key: AlertAROverdue, Label: "ยอดลูกหนี้เลยกำหนด", Description: "เตือนเมื่อยอดลูกหนี้ที่เลยกำหนดชำระรวมกันเกินจำนวนเงินที่ตั้ง (บาท) ตรวจทุกเช้า", Report: report.ARAging, Unit: AlertUnitBaht, Min: 1, Max: 10_000_000_000, Cooldown: alertCooldown},
	{Key: AlertAROverYear, Label: "ยอดลูกหนี้ค้างเกิน 1 ปี", Description: "เตือนเมื่อยอดลูกหนี้ที่ออกใบมาเกิน 1 ปีรวมกันเกินจำนวนเงินที่ตั้ง (บาท) ตรวจทุกเช้า", Report: report.ARAging, Unit: AlertUnitBaht, Min: 1, Max: 10_000_000_000, Cooldown: alertCooldown},
	{Key: AlertStockReorder, Label: "สินค้าถึงจุดสั่งซื้อ", Description: "เตือนเมื่อมีสินค้าถึงจุดสั่งซื้อตั้งแต่จำนวนรายการที่ตั้ง ตรวจทุกเช้า", Report: report.StockReorder, Unit: AlertUnitCount, Min: 1, Max: 100_000, Cooldown: alertCooldown},
	{Key: AlertSalesDrop, Label: "ยอดขายเมื่อวานตก", Description: "เตือนเมื่อยอดขายเมื่อวานต่ำกว่าวันเดียวกันของสัปดาห์ก่อนตั้งแต่เปอร์เซ็นต์ที่ตั้ง ตรวจทุกเช้า", Report: report.SalesGoodsServices, Unit: AlertUnitPercent, Min: 1, Max: 99},
	{Key: AlertReceiptsDrop, Label: "เงินเข้าเมื่อวานตก", Description: "เตือนเมื่อยอดรับเงินเมื่อวานต่ำกว่าวันเดียวกันของสัปดาห์ก่อนตั้งแต่เปอร์เซ็นต์ที่ตั้ง (ไม่เตือนถ้าเมื่อวานไม่มีเอกสารรับเงินเลย เพราะร้านอาจปิด) ตรวจทุกเช้า", Report: report.CashBankReceipts, Unit: AlertUnitPercent, Min: 1, Max: 99},
	{Key: AlertMarginDrop, Label: "อัตรากำไรขั้นต้นเมื่อวานลด", Description: "เตือนเมื่ออัตรากำไรขั้นต้นเมื่อวานต่ำกว่าวันเดียวกันของสัปดาห์ก่อนตั้งแต่จำนวนจุดเปอร์เซ็นต์ที่ตั้ง (เช่น จาก 25% เหลือ 20% คือลด 5 จุด) ไม่เตือนถ้าเมื่อวานไม่มียอดขาย ตรวจทุกเช้า", Report: report.GrossProfitByProduct, Unit: AlertUnitPoints, Min: 1, Max: 50},
	{Key: AlertMorningDigest, Label: "สรุปเช้า", Description: "ส่งสรุปสั้นๆ ทุกเช้า: ยอดขายเมื่อวานเทียบวันเดียวกันของสัปดาห์ก่อน ยอดลูกหนี้เลยกำหนด และจำนวนสินค้าถึงจุดสั่งซื้อ (เฉพาะรายงานที่มีสิทธิ์ดู) ไม่ต้องระบุเกณฑ์", Unit: AlertUnitSwitch, Min: 1, Max: 1, Switch: true,
		AnyOf: []report.Key{report.SalesGoodsServices, report.ARAging, report.StockReorder}},
}

func AlertCatalog() []AlertRuleDef { return append([]AlertRuleDef(nil), alertCatalog...) }

func AlertRuleFor(raw string) (AlertRuleDef, bool) {
	raw = strings.TrimSpace(raw)
	for _, def := range alertCatalog {
		if string(def.Key) == raw {
			return def, true
		}
	}
	return AlertRuleDef{}, false
}

// InvalidAlertError says, in Thai the assistant can pass on, what a threshold may be.
type InvalidAlertError struct{ Message string }

func (err *InvalidAlertError) Error() string { return "alert threshold is not valid" }

// ParseAlertThreshold turns what the owner said into the stored threshold: digits, an optional decimal point for baht,
// commas and spaces ignored, inside the rule's range. It returns the number and its plain text form.
func ParseAlertThreshold(def AlertRuleDef, raw string) (*big.Rat, string, error) {
	invalid := &InvalidAlertError{Message: fmt.Sprintf("เกณฑ์ของ “%s” ต้องเป็น%s ระหว่าง %s ถึง %s", def.Label, unitPhrase(def.Unit), groupDigits(def.Min), groupDigits(def.Max))}
	cleaned := stripUnitWords(strings.TrimSpace(raw), def.Unit)
	cleaned = strings.NewReplacer(",", "", " ", "", "_", "").Replace(cleaned)
	if cleaned == "" || strings.ContainsAny(cleaned, "eE+-") {
		return nil, "", invalid
	}
	value, ok := new(big.Rat).SetString(cleaned)
	if !ok {
		return nil, "", invalid
	}
	if def.Unit != AlertUnitBaht && !value.IsInt() {
		return nil, "", invalid
	}
	if value.Cmp(new(big.Rat).SetInt64(def.Min)) < 0 || value.Cmp(new(big.Rat).SetInt64(def.Max)) > 0 {
		return nil, "", invalid
	}
	text := value.FloatString(2)
	if def.Unit != AlertUnitBaht {
		text = value.FloatString(0)
	}
	if strings.Contains(text, ".") { // only zeros after a decimal point are noise: "40" must stay "40"
		text = strings.TrimSuffix(strings.TrimRight(text, "0"), ".")
	}
	return value, text, nil
}

// stripUnitWords drops the unit a person naturally says after or before a number ("40%", "500,000 บาท", "3 รายการ", "5 จุด"),
// so the assistant does not have to be trusted to send bare digits. Only the unit that belongs to the rule is dropped:
// "5 บาท" for a margin rule is a misunderstanding and is refused like any other text that is not a number.
func stripUnitWords(text string, unit AlertUnit) string {
	var words []string
	switch unit {
	case AlertUnitBaht:
		words = []string{"บาท", "฿"}
	case AlertUnitCount:
		words = []string{"รายการ"}
	case AlertUnitPercent:
		words = []string{"เปอร์เซ็นต์", "เปอร์เซนต์", "%"}
	case AlertUnitPoints:
		words = []string{"จุดเปอร์เซ็นต์", "จุด", "%"}
	}
	for _, word := range words { // at most one unit before the number...
		if trimmed := strings.TrimPrefix(text, word); trimmed != text {
			text = strings.TrimSpace(trimmed)
			break
		}
	}
	for _, word := range words { // ...and at most one after it
		if trimmed := strings.TrimSuffix(text, word); trimmed != text {
			text = strings.TrimSpace(trimmed)
			break
		}
	}
	return text
}

func unitPhrase(unit AlertUnit) string {
	switch unit {
	case AlertUnitBaht:
		return "จำนวนเงินเป็นบาท"
	case AlertUnitCount:
		return "จำนวนรายการ (เลขจำนวนเต็ม)"
	case AlertUnitPoints:
		return "จำนวนจุดเปอร์เซ็นต์ (เลขจำนวนเต็ม)"
	default:
		return "เปอร์เซ็นต์ (เลขจำนวนเต็ม)"
	}
}

func groupDigits(value int64) string {
	text := fmt.Sprintf("%d", value)
	for index := len(text) - 3; index > 0; index -= 3 {
		text = text[:index] + "," + text[index:]
	}
	return text
}

// StoredAlert is one rule as kept for a recipient.
type StoredAlert struct {
	Rule        AlertRuleKey
	Threshold   string
	Enabled     bool
	LastFiredAt *time.Time
	LastStatus  string
}

// AlertStore keeps a recipient's rules. Reads and writes are always for the principal's own tenant and recipient.
type AlertStore interface {
	Alerts(ctx context.Context, principal Principal) ([]StoredAlert, error)
	UpsertAlert(ctx context.Context, principal Principal, rule AlertRuleKey, threshold string, enabled bool, requestID string, now time.Time) (StoredAlert, error)
}

type AlertView struct {
	Rule        AlertRuleKey `json:"rule"`
	Label       string       `json:"label"`
	Description string       `json:"description"`
	Unit        AlertUnit    `json:"unit"`
	Available   bool         `json:"available"`
	Threshold   string       `json:"threshold,omitempty"`
	Enabled     bool         `json:"enabled"`
	LastFiredAt string       `json:"lastFiredAt,omitempty"`
	Message     string       `json:"message,omitempty"`
}

type AlertsResponse struct {
	Status string      `json:"status"`
	Alerts []AlertView `json:"alerts"`
	Notes  []string    `json:"notes,omitempty"`
}

// AlertSetRequest: Threshold is empty when only switching a rule off or on. Enabled defaults to on.
type AlertSetRequest struct {
	Threshold string `json:"threshold"`
	Enabled   *bool  `json:"enabled"`
}

const (
	MessageAlertSet      = "ตั้งการเตือนแล้ว ระบบตรวจทุกเช้าและจะแจ้งเมื่อเงื่อนไขเป็นจริง"
	MessageAlertOff      = "ปิดการเตือนนี้แล้ว"
	MessageAlertsNote    = "การเตือนส่งเป็นข้อความในแชตนี้ ตรวจวันละครั้งตอนเช้า และไม่เตือนซ้ำเรื่องเดิมภายใน 3 วันถ้าตัวเลขไม่แย่ลงเกิน 10%"
	MessageAlertNeedsSet = "ต้องระบุเกณฑ์ก่อนจึงจะเปิดการเตือนนี้ได้"
	MessageDigestOn      = "เปิดสรุปเช้าแล้ว ระบบจะส่งสรุปในแชตนี้ทุกเช้า"
	MessageDigestOff     = "ปิดสรุปเช้าแล้ว"
)

var ErrAlertsUnavailable = errors.New("alerts are not available")

// ConfigureAlerts turns on the two alert tools. Without it they answer as if there were no such thing.
func (service *Service) ConfigureAlerts(store AlertStore) *Service {
	service.alerts = store
	return service
}

func (service *Service) Alerts(ctx context.Context, principal Principal) (AlertsResponse, error) {
	started := service.now()
	if service.alerts == nil {
		return AlertsResponse{}, ErrAlertsUnavailable
	}
	now := started.UTC()
	stored, err := service.alerts.Alerts(ctx, principal)
	if err != nil {
		return AlertsResponse{}, fmt.Errorf("list alerts: %w", err)
	}
	permitted, err := service.store.PermittedReports(ctx, principal, now)
	if err != nil {
		return AlertsResponse{}, fmt.Errorf("list permitted reports: %w", err)
	}
	service.record(ctx, principal, ToolAlerts, "", report.Period{}, OutcomeOK, started, nil)
	response := AlertsResponse{Status: "READY", Notes: []string{MessageAlertsNote}}
	for _, def := range alertCatalog {
		view := AlertView{Rule: def.Key, Label: def.Label, Description: def.Description, Unit: def.Unit, Available: def.available(permitted)}
		for _, item := range stored {
			if item.Rule == def.Key {
				view.Threshold, view.Enabled = item.Threshold, item.Enabled
				if def.Switch {
					view.Threshold = ""
				}
				if item.LastFiredAt != nil {
					view.LastFiredAt = item.LastFiredAt.In(locationOf(principal)).Format(time.RFC3339)
				}
			}
		}
		response.Alerts = append(response.Alerts, view)
	}
	return response, nil
}

// SetAlert creates, changes or switches off one rule. A rule on a report the recipient may not read, or one that is
// not in the catalog, answers exactly like a missing report.
func (service *Service) SetAlert(ctx context.Context, principal Principal, rawRule string, request AlertSetRequest, requestID string) (AlertView, error) {
	started := service.now()
	if service.alerts == nil {
		return AlertView{}, ErrAlertsUnavailable
	}
	now := started.UTC()
	def, known := AlertRuleFor(rawRule)
	if known {
		permitted, err := service.store.PermittedReports(ctx, principal, now)
		if err != nil {
			return AlertView{}, fmt.Errorf("list permitted reports: %w", err)
		}
		known = def.available(permitted)
	}
	if !known {
		service.record(ctx, principal, ToolAlertSet, "", report.Period{}, OutcomeNoData, started, nil)
		return AlertView{}, ErrNoData
	}
	enabled := request.Enabled == nil || *request.Enabled
	existing, err := service.alerts.Alerts(ctx, principal)
	if err != nil {
		return AlertView{}, fmt.Errorf("read alert: %w", err)
	}
	threshold := ""
	for _, item := range existing {
		if item.Rule == def.Key {
			threshold = item.Threshold
		}
	}
	if def.Switch {
		threshold = switchPlaceholder // a switch has no threshold; whatever was said is ignored
	} else if strings.TrimSpace(request.Threshold) != "" {
		_, text, parseErr := ParseAlertThreshold(def, request.Threshold)
		if parseErr != nil {
			service.record(ctx, principal, ToolAlertSet, string(def.Key), report.Period{}, OutcomeInvalidAlert, started, nil)
			return AlertView{}, parseErr
		}
		threshold = text
	}
	view := AlertView{Rule: def.Key, Label: def.Label, Description: def.Description, Unit: def.Unit, Available: true}
	if threshold == "" {
		if enabled {
			service.record(ctx, principal, ToolAlertSet, string(def.Key), report.Period{}, OutcomeInvalidAlert, started, nil)
			return AlertView{}, &InvalidAlertError{Message: MessageAlertNeedsSet}
		}
		view.Message = MessageAlertOff // switching off a rule that was never set changes nothing
		service.record(ctx, principal, ToolAlertSet, string(def.Key), report.Period{}, OutcomeOK, started, nil)
		return view, nil
	}
	saved, err := service.alerts.UpsertAlert(ctx, principal, def.Key, threshold, enabled, requestID, now)
	if err != nil {
		return AlertView{}, fmt.Errorf("save alert: %w", err)
	}
	service.record(ctx, principal, ToolAlertSet, string(def.Key), report.Period{}, OutcomeOK, started, nil)
	view.Threshold, view.Enabled = saved.Threshold, saved.Enabled
	view.Message = MessageAlertSet
	if !saved.Enabled {
		view.Message = MessageAlertOff
	}
	if def.Switch {
		view.Threshold = ""
		view.Message = MessageDigestOn
		if !saved.Enabled {
			view.Message = MessageDigestOff
		}
	}
	return view, nil
}

func contains(keys []report.Key, wanted report.Key) bool {
	for _, key := range keys {
		if key == wanted {
			return true
		}
	}
	return false
}

// AlertTarget is one enabled rule together with the person it belongs to, as the worker's daily check sees it.
type AlertTarget struct {
	RuleID         uuid.UUID
	Rule           AlertRuleKey
	Threshold      *big.Rat
	LastCheckedOn  string
	LastFiredAt    *time.Time
	LastFiredValue *big.Rat
	Principal      Principal
}

// ShouldFireAgain says whether a rule that is true now may speak: the first time, after its cooldown, or when the
// figure got at least 10% worse than the one it last spoke about.
func ShouldFireAgain(def AlertRuleDef, target AlertTarget, value *big.Rat, now time.Time) bool {
	if def.Cooldown == 0 || target.LastFiredAt == nil {
		return true
	}
	if now.Sub(*target.LastFiredAt) >= def.Cooldown {
		return true
	}
	if target.LastFiredValue == nil || target.LastFiredValue.Sign() <= 0 {
		return false
	}
	worse := new(big.Rat).Mul(target.LastFiredValue, big.NewRat(11, 10))
	return value.Cmp(worse) >= 0
}

// AlertEvent is a rule that fired, with the message AI-BCC wrote for it.
type AlertEvent struct {
	ID        uuid.UUID
	RuleID    uuid.UUID
	TenantID  uuid.UUID
	Recipient uuid.UUID
	Rule      AlertRuleKey
	FiredOn   string
	Message   string
	Status    string
	Attempts  int
}

const (
	AlertEventPending = "PENDING"
	AlertEventSent    = "SENT"
	AlertEventFailed  = "FAILED"
	AlertEventDryRun  = "DRY_RUN"

	AlertCheckOK       = "OK"
	AlertCheckNotReady = "NOT_READY"
	AlertCheckNoAccess = "NO_ACCESS"
	AlertCheckError    = "ERROR"
)
