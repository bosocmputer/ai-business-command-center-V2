package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/google/uuid"
)

// Source is where the daily check reads numbers. It is the assistant's own service, so a rule sees exactly what the
// owner would see if they asked, with the same permission check, snapshots and fetch budget.
type Source interface {
	Report(ctx context.Context, principal agent.Principal, reportKey, dateFrom, dateTo string) (agent.ReportResponse, error)
	Compare(ctx context.Context, principal agent.Principal, reportKey, metric, aFrom, aTo, bFrom, bTo string) (agent.CompareResponse, error)
}

type Store interface {
	AlertTargets(ctx context.Context, now time.Time) ([]agent.AlertTarget, error)
	MarkAlertChecked(ctx context.Context, ruleID uuid.UUID, on, status string) error
	RecordAlertFire(ctx context.Context, target agent.AlertTarget, on, value, threshold, message, status string, now time.Time) (agent.AlertEvent, bool, error)
	UnsentAlerts(ctx context.Context, now time.Time) ([]agent.AlertEvent, error)
	MarkAlertDelivery(ctx context.Context, id uuid.UUID, delivered bool, errorCode string, now time.Time) error
}

type Sender interface {
	Send(ctx context.Context, event agent.AlertEvent) error
}

// Evaluator runs the daily check. With dryRun it records what it would have said and sends nothing.
type Evaluator struct {
	store  Store
	source Source
	sender Sender
	dryRun bool
	now    func() time.Time
	logger *slog.Logger
	// The check starts at StartMinute of the shop's day (after the morning prefetch) and gives up for the day after Window.
	StartMinute int
	Window      time.Duration
}

func NewEvaluator(store Store, source Source, sender Sender, dryRun bool, now func() time.Time, logger *slog.Logger) *Evaluator {
	return &Evaluator{store: store, source: source, sender: sender, dryRun: dryRun, now: now, logger: logger, StartMinute: 8*60 + 15, Window: 4 * time.Hour}
}

// Summary holds counts only.
type Summary struct{ Checked, NotReady, Fired, Sent, Failed, Errors int }

// RunOnce looks at every enabled rule that has not been checked today and delivers what is waiting to be delivered.
func (evaluator *Evaluator) RunOnce(ctx context.Context) Summary {
	var summary Summary
	now := evaluator.now().UTC()
	targets, err := evaluator.store.AlertTargets(ctx, now)
	if err != nil {
		evaluator.logger.Error("alert targets failed", "safeErrorCode", "ALERT_TARGETS_FAILED")
		summary.Errors++
		return summary
	}
	for _, target := range targets {
		if ctx.Err() != nil {
			return summary
		}
		location, locErr := time.LoadLocation(target.Principal.Timezone)
		if locErr != nil {
			location = time.FixedZone("Asia/Bangkok", 7*60*60)
		}
		local := now.In(location)
		today := local.Format(time.DateOnly)
		minutes := local.Hour()*60 + local.Minute()
		if target.LastCheckedOn == today || minutes < evaluator.StartMinute || time.Duration(minutes-evaluator.StartMinute)*time.Minute > evaluator.Window {
			continue
		}
		def, known := agent.AlertRuleFor(string(target.Rule))
		if !known {
			continue
		}
		result, evalErr := evaluator.evaluate(ctx, target, def, local, location)
		switch {
		case errors.Is(evalErr, agent.ErrNoData):
			_ = evaluator.store.MarkAlertChecked(ctx, target.RuleID, today, agent.AlertCheckNoAccess)
			continue
		case evalErr != nil:
			summary.Errors++
			evaluator.logger.Warn("alert check failed", "rule", string(target.Rule), "safeErrorCode", "ALERT_CHECK_FAILED")
			_ = evaluator.store.MarkAlertChecked(ctx, target.RuleID, today, agent.AlertCheckError)
			continue
		case !result.ready:
			summary.NotReady++
			_ = evaluator.store.MarkAlertChecked(ctx, target.RuleID, today, agent.AlertCheckNotReady)
			continue
		}
		// Record first, mark the day checked second: a crash between the two repeats the check, and the unique
		// (rule, day) record makes the repeat harmless, while the other order could lose an alert for the day.
		if result.fired && agent.ShouldFireAgain(def, target, result.value, now) {
			status := agent.AlertEventPending
			if evaluator.dryRun {
				status = agent.AlertEventDryRun
			}
			_, created, err := evaluator.store.RecordAlertFire(ctx, target, today, result.value.FloatString(2), target.Threshold.FloatString(2), result.message, status, now)
			if err != nil {
				summary.Errors++
				evaluator.logger.Error("alert record failed", "rule", string(target.Rule), "safeErrorCode", "ALERT_RECORD_FAILED")
				continue
			}
			if created {
				summary.Fired++
			}
		}
		if err := evaluator.store.MarkAlertChecked(ctx, target.RuleID, today, agent.AlertCheckOK); err != nil {
			summary.Errors++
			continue
		}
		summary.Checked++
	}
	if !evaluator.dryRun && evaluator.sender != nil {
		evaluator.deliver(ctx, now, &summary)
	}
	if summary != (Summary{}) {
		evaluator.logger.Info("alert check completed", "event", "agent_alert_check", "dryRun", evaluator.dryRun,
			"checked", summary.Checked, "notReady", summary.NotReady, "fired", summary.Fired, "sent", summary.Sent, "failed", summary.Failed, "errors", summary.Errors)
	}
	return summary
}

func (evaluator *Evaluator) deliver(ctx context.Context, now time.Time, summary *Summary) {
	events, err := evaluator.store.UnsentAlerts(ctx, now)
	if err != nil {
		summary.Errors++
		evaluator.logger.Error("alert unsent list failed", "safeErrorCode", "ALERT_UNSENT_FAILED")
		return
	}
	for _, event := range events {
		if ctx.Err() != nil {
			return
		}
		sendErr := evaluator.sender.Send(ctx, event)
		code := ""
		var sendError *SendError
		if errors.As(sendErr, &sendError) {
			code = sendError.Code
		} else if sendErr != nil {
			code = "SEND_FAILED"
		}
		if markErr := evaluator.store.MarkAlertDelivery(ctx, event.ID, sendErr == nil, code, evaluator.now().UTC()); markErr != nil {
			summary.Errors++
		}
		if sendErr == nil {
			summary.Sent++
		} else {
			summary.Failed++
			evaluator.logger.Warn("alert delivery failed", "rule", string(event.Rule), "safeErrorCode", code)
		}
	}
}

type evaluation struct {
	ready   bool
	fired   bool
	value   *big.Rat
	message string
}

func (evaluator *Evaluator) evaluate(ctx context.Context, target agent.AlertTarget, def agent.AlertRuleDef, local time.Time, location *time.Location) (evaluation, error) {
	switch def.Key {
	case agent.AlertAROverdue:
		return evaluator.stateRule(ctx, target, def, "overdue_amount", location, func(value *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: ยอดลูกหนี้เลยกำหนดตอนนี้ %s บาท สูงกว่าที่คุณตั้งเตือนไว้ (%s บาท)", baht(value), baht(target.Threshold))
		}, "ลูกหนี้รายไหนเลยกำหนดเยอะสุด")
	case agent.AlertAROverYear:
		return evaluator.stateRule(ctx, target, def, "over_year_amount", location, func(value *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: ยอดลูกหนี้ค้างเกิน 1 ปีตอนนี้ %s บาท สูงกว่าที่คุณตั้งเตือนไว้ (%s บาท)", baht(value), baht(target.Threshold))
		}, "ลูกหนี้รายไหนค้างนานที่สุด")
	case agent.AlertStockReorder:
		return evaluator.stateRule(ctx, target, def, "reorder_item_count", location, func(value *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: มีสินค้าถึงจุดสั่งซื้อ %s รายการ (คุณตั้งเตือนไว้ตั้งแต่ %s รายการ)", whole(value), whole(target.Threshold))
		}, "สินค้าอะไรใกล้หมดสต็อกบ้าง")
	case agent.AlertSalesDrop:
		return evaluator.salesDrop(ctx, target, def, local, location)
	}
	return evaluation{}, fmt.Errorf("alert rule %q has no check", def.Key)
}

// stateRule watches a figure that is true or false right now (receivables, stock). A report that is still being
// fetched, or that was served from a snapshot older than today's fetch, is not ready: the check waits for the fresh one.
func (evaluator *Evaluator) stateRule(ctx context.Context, target agent.AlertTarget, def agent.AlertRuleDef, metric string, location *time.Location, headline func(*big.Rat) string, followUp string) (evaluation, error) {
	response, err := evaluator.source.Report(ctx, target.Principal, string(def.Report), "", "")
	if err != nil {
		return evaluation{}, err
	}
	if response.Status != "READY" || response.Freshness != "FRESH" {
		return evaluation{}, nil
	}
	value, ok := kpi(response, metric)
	if !ok {
		return evaluation{}, fmt.Errorf("report %s has no %s", def.Report, metric)
	}
	result := evaluation{ready: true, value: value, fired: value.Cmp(target.Threshold) >= 0}
	if result.fired {
		result.message = headline(value) + asOf(response.CollectedAt, location) + closing(def, followUp)
	}
	return result, nil
}

// salesDrop compares yesterday with the same weekday a week earlier. Two closed days are stable once fetched.
func (evaluator *Evaluator) salesDrop(ctx context.Context, target agent.AlertTarget, def agent.AlertRuleDef, local time.Time, location *time.Location) (evaluation, error) {
	yesterday := local.AddDate(0, 0, -1)
	before := yesterday.AddDate(0, 0, -7)
	a, b := yesterday.Format(time.DateOnly), before.Format(time.DateOnly)
	response, err := evaluator.source.Compare(ctx, target.Principal, string(def.Report), "total_amount", a, a, b, b)
	if err != nil {
		return evaluation{}, err
	}
	if response.Status != "READY" || response.A == nil || response.B == nil {
		return evaluation{}, nil
	}
	yesterdayTotal, okA := new(big.Rat).SetString(response.A.Value)
	weekBefore, okB := new(big.Rat).SetString(response.B.Value)
	if !okA || !okB {
		return evaluation{}, fmt.Errorf("compare returned a figure that is not a number")
	}
	if weekBefore.Sign() <= 0 { // nothing to compare with (a closed day, a holiday): no alert
		return evaluation{ready: true, value: new(big.Rat)}, nil
	}
	drop := new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).Sub(weekBefore, yesterdayTotal), big.NewRat(100, 1)), weekBefore)
	result := evaluation{ready: true, value: drop, fired: drop.Cmp(target.Threshold) >= 0}
	if result.fired {
		weekday := thaiWeekday(yesterday)
		result.message = fmt.Sprintf("⚠️ แจ้งเตือน: ยอดขายเมื่อวาน (วัน%s ที่ %s) %s บาท ต่ำกว่าวัน%sสัปดาห์ก่อน (%s บาท) อยู่ %s%% (คุณตั้งเตือนไว้ที่ %s%%)",
			weekday, thaiDate(yesterday), baht(yesterdayTotal), weekday, baht(weekBefore), percent(drop), whole(target.Threshold)) +
			asOf(response.CollectedAt, location) + closing(def, "ยอดขายเมื่อวานเป็นอย่างไรบ้าง")
	}
	return result, nil
}

func closing(def agent.AlertRuleDef, followUp string) string {
	return fmt.Sprintf("\n\nถามต่อได้เลย เช่น “%s”\nอยากปิดการเตือนนี้ พิมพ์ “ปิดเตือน%s”", followUp, def.Label)
}

func kpi(response agent.ReportResponse, key string) (*big.Rat, bool) {
	for _, item := range response.KPIs {
		if item.Key == key {
			value, ok := new(big.Rat).SetString(item.Value)
			return value, ok
		}
	}
	return nil, false
}
