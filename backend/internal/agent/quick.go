package agent

import (
	"context"
	"errors"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

// The three figures an owner taps for in the LINE menu. They are read for a person and a shop, with the same permission check, snapshot
// rules and freshness waits as the assistant's get_report, and handed back as a dashboard for a card. No model is involved.

type QuickKind string

const (
	QuickYesterday QuickKind = "yesterday" // sales of yesterday
	QuickToday     QuickKind = "today"     // sales of today, read again from the shop's system now
	QuickReorder   QuickKind = "reorder"   // items at or below their reorder point
)

const (
	QuickReady       = "READY"
	QuickPreparing   = "PREPARING"
	QuickUnavailable = "UNAVAILABLE"
	QuickNotAllowed  = "NOT_ALLOWED"

	MessageQuickPreparing   = "กำลังดึงตัวเลขจากระบบของร้าน ใช้เวลาสักครู่ กดปุ่มอีกครั้งในอีกประมาณ 1 นาทีครับ"
	MessageQuickUnavailable = "ตอนนี้ดึงตัวเลขจากระบบของร้านไม่ได้ ลองกดอีกครั้งในอีกสักครู่ หรือแจ้งผู้ดูแลระบบถ้ายังเป็นอยู่"
	MessageQuickNotAllowed  = "คุณยังไม่ได้รับสิทธิ์ดูรายงานนี้ กรุณาติดต่อผู้ดูแลระบบของร้าน"
)

type QuickResult struct {
	Status     string
	Message    string
	Key        report.Key
	Period     report.Period
	Dashboard  report.Dashboard
	FinishedAt time.Time
	Stale      bool
	// RunID is the stored run the figures come from, so the card's button can open exactly those figures.
	RunID uuid.UUID
}

func quickReport(kind QuickKind) (report.Key, bool) {
	switch kind {
	case QuickYesterday, QuickToday:
		return report.SalesGoodsServices, true
	case QuickReorder:
		return report.StockReorder, true
	}
	return "", false
}

// QuickReport reads one of the menu's figures for a person who may ask about the shop. The caller has already checked that the person
// belongs to the shop and that the shop's assistant is on; the report permission is checked here, like for every other read.
func (service *Service) QuickReport(ctx context.Context, tenantID, recipientID uuid.UUID, kind QuickKind) (QuickResult, error) {
	started := service.now()
	now := started.UTC()
	key, ok := quickReport(kind)
	if !ok {
		return QuickResult{}, ErrInvalidPeriod
	}
	principal := Principal{TenantID: tenantID, RecipientID: recipientID, HolderRecipientID: recipientID, Timezone: "Asia/Bangkok"}
	definition, err := service.authorize(ctx, principal, string(key), now)
	if errors.Is(err, ErrNoData) {
		return QuickResult{Status: QuickNotAllowed, Message: MessageQuickNotAllowed, Key: key}, nil
	}
	if err != nil {
		return QuickResult{}, err
	}
	location := locationOf(principal)
	day := now.In(location).Format(time.DateOnly)
	dateFrom, dateTo := "", ""
	switch kind {
	case QuickYesterday:
		dateFrom = now.In(location).AddDate(0, 0, -1).Format(time.DateOnly)
		dateTo = dateFrom
	case QuickToday:
		dateFrom, dateTo = day, day
		ctx = WithRefresh(ctx)
	}
	period, err := resolvePeriod(definition, location, now, dateFrom, dateTo)
	if err != nil {
		return QuickResult{}, err
	}
	snapshot := service.snapshot(ctx, principal, definition, period, now)
	result := QuickResult{Key: key, Period: period}
	switch snapshot.state {
	case stateReady:
		result.Status, result.Stale = QuickReady, snapshot.stale
		result.Dashboard = snapshot.snapshot.Dashboard
		result.RunID = snapshot.snapshot.RunID
		result.FinishedAt = now
		if finished := snapshot.snapshot.SourceFinishedAt; finished != nil {
			result.FinishedAt = *finished
		}
		if kind == QuickToday {
			// Today is not over: comparing it with a whole yesterday would always look like a drop.
			for index := range result.Dashboard.KPIs {
				result.Dashboard.KPIs[index].Comparison = report.MetricComparison{Availability: report.ComparisonUnavailable}
			}
			result.Dashboard.ComparisonPeriod = report.Period{}
		}
	case statePreparing:
		result.Status, result.Message = QuickPreparing, MessageQuickPreparing
	default:
		result.Status, result.Message = QuickUnavailable, MessageQuickUnavailable
	}
	return result, nil
}
