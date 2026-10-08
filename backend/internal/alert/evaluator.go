package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
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
		return evaluator.stateRuleWithNote(ctx, target, def, "overdue_amount", location, func(value *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: ยอดลูกหนี้เลยกำหนดตอนนี้ %s บาท สูงกว่าที่คุณตั้งเตือนไว้ (%s บาท)", baht(value), baht(target.Threshold))
		}, "ลูกหนี้รายไหนเลยกำหนดเยอะสุด", func(response agent.ReportResponse) string {
			if amount := noDueDateAmount(response); amount != nil {
				return fmt.Sprintf("\n(ยอดนี้ไม่รวมเอกสารที่ไม่มีวันครบกำหนด %s บาท เพราะไม่มีวันครบกำหนดในระบบ จึงไม่นับว่าเลยกำหนด)", baht(amount))
			}
			return ""
		})
	case agent.AlertAROverYear:
		return evaluator.stateRule(ctx, target, def, "over_year_amount", location, func(value *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: ยอดลูกหนี้ค้างเกิน 1 ปีตอนนี้ %s บาท สูงกว่าที่คุณตั้งเตือนไว้ (%s บาท)", baht(value), baht(target.Threshold))
		}, "ลูกหนี้รายไหนค้างนานที่สุด")
	case agent.AlertStockReorder:
		return evaluator.stateRule(ctx, target, def, "reorder_item_count", location, func(value *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: มีสินค้าถึงจุดสั่งซื้อ %s รายการ (คุณตั้งเตือนไว้ตั้งแต่ %s รายการ)", whole(value), whole(target.Threshold))
		}, "สินค้าอะไรใกล้หมดสต็อกบ้าง")
	case agent.AlertSalesDrop, agent.AlertReceiptsDrop, agent.AlertMarginDrop:
		return evaluator.weekdayDrop(ctx, target, def, local, location, dropSpecs[def.Key])
	case agent.AlertMorningDigest:
		return evaluator.digest(ctx, target, local, location)
	}
	return evaluation{}, fmt.Errorf("alert rule %q has no check", def.Key)
}

// stateRule watches a figure that is true or false right now (receivables, stock). A report that is still being
// fetched, or that was served from a snapshot older than today's fetch, is not ready: the check waits for the fresh one.
func (evaluator *Evaluator) stateRule(ctx context.Context, target agent.AlertTarget, def agent.AlertRuleDef, metric string, location *time.Location, headline func(*big.Rat) string, followUp string) (evaluation, error) {
	return evaluator.stateRuleWithNote(ctx, target, def, metric, location, headline, followUp, nil)
}

// stateRuleWithNote is stateRule plus a sentence the rule adds under its headline from the same report.
func (evaluator *Evaluator) stateRuleWithNote(ctx context.Context, target agent.AlertTarget, def agent.AlertRuleDef, metric string, location *time.Location, headline func(*big.Rat) string, followUp string, note func(agent.ReportResponse) string) (evaluation, error) {
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
		result.message = headline(value)
		if note != nil {
			result.message += note(response)
		}
		result.message += asOf(response.CollectedAt, location) + closing(def, followUp)
	}
	return result, nil
}

// weekdayDrop compares yesterday with the same weekday a week earlier. Two closed days are stable once fetched. A day
// the shop was probably closed (the guard figure, such as the number of documents, is zero) never raises an alert, and
// neither does a week-before base of zero. The value compared is the fall as a percent of the week-before figure, or in
// percentage points for a rate such as the gross margin.
type dropSpec struct {
	metric, guard string
	points        bool
	headline      func(weekday, date string, yesterday, before, value *big.Rat, threshold *big.Rat) string
	followUp      string
}

func (evaluator *Evaluator) weekdayDrop(ctx context.Context, target agent.AlertTarget, def agent.AlertRuleDef, local time.Time, location *time.Location, spec dropSpec) (evaluation, error) {
	yesterday := local.AddDate(0, 0, -1)
	before := yesterday.AddDate(0, 0, -7)
	a, b := yesterday.Format(time.DateOnly), before.Format(time.DateOnly)
	response, err := evaluator.source.Compare(ctx, target.Principal, string(def.Report), spec.metric, a, a, b, b)
	if err != nil {
		return evaluation{}, err
	}
	if response.Status != "READY" || response.A == nil || response.B == nil {
		return evaluation{}, nil
	}
	yesterdayValue, okA := new(big.Rat).SetString(response.A.Value)
	weekBefore, okB := new(big.Rat).SetString(response.B.Value)
	if !okA || !okB {
		return evaluation{}, fmt.Errorf("compare returned a figure that is not a number")
	}
	none := evaluation{ready: true, value: new(big.Rat)}
	if weekBefore.Sign() <= 0 { // nothing to compare with (a closed day, a holiday): no alert
		return none, nil
	}
	if spec.guard != "" {
		guard, guardErr := evaluator.source.Compare(ctx, target.Principal, string(def.Report), spec.guard, a, a, b, b)
		if guardErr != nil {
			return evaluation{}, guardErr
		}
		if guard.Status != "READY" || guard.A == nil {
			return evaluation{}, nil
		}
		if opened, ok := new(big.Rat).SetString(guard.A.Value); !ok || opened.Sign() <= 0 { // yesterday the shop did nothing: it was probably closed
			return none, nil
		}
	}
	var drop *big.Rat
	if spec.points {
		drop = new(big.Rat).Sub(weekBefore, yesterdayValue)
	} else {
		drop = new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).Sub(weekBefore, yesterdayValue), big.NewRat(100, 1)), weekBefore)
	}
	result := evaluation{ready: true, value: drop, fired: drop.Cmp(target.Threshold) >= 0}
	if result.fired {
		result.message = spec.headline(thaiWeekday(yesterday), thaiDate(yesterday), yesterdayValue, weekBefore, drop, target.Threshold) +
			asOf(response.CollectedAt, location) + closing(def, spec.followUp)
	}
	return result, nil
}

var dropSpecs = map[agent.AlertRuleKey]dropSpec{
	agent.AlertSalesDrop: {metric: "total_amount", guard: "document_count", followUp: "ยอดขายเมื่อวานเป็นอย่างไรบ้าง",
		headline: func(weekday, date string, yesterday, before, drop, threshold *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: ยอดขายเมื่อวาน (วัน%s ที่ %s) %s บาท ต่ำกว่าวัน%sสัปดาห์ก่อน (%s บาท) อยู่ %s%% (คุณตั้งเตือนไว้ที่ %s%%)",
				weekday, date, baht(yesterday), weekday, baht(before), percent(drop), whole(threshold))
		}},
	agent.AlertReceiptsDrop: {metric: "total_amount", guard: "document_count", followUp: "เงินเข้าเมื่อวานเป็นอย่างไรบ้าง",
		headline: func(weekday, date string, yesterday, before, drop, threshold *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: เงินเข้าเมื่อวาน (วัน%s ที่ %s) %s บาท ต่ำกว่าวัน%sสัปดาห์ก่อน (%s บาท) อยู่ %s%% (คุณตั้งเตือนไว้ที่ %s%%)",
				weekday, date, baht(yesterday), weekday, baht(before), percent(drop), whole(threshold))
		}},
	agent.AlertMarginDrop: {metric: "gross_margin_percent", guard: "net_amount", points: true, followUp: "กำไรขั้นต้นเมื่อวานเป็นอย่างไรบ้าง",
		headline: func(weekday, date string, yesterday, before, drop, threshold *big.Rat) string {
			return fmt.Sprintf("⚠️ แจ้งเตือน: อัตรากำไรขั้นต้นเมื่อวาน (วัน%s ที่ %s) อยู่ที่ %s%% ต่ำกว่าวัน%sสัปดาห์ก่อน (%s%%) อยู่ %s จุด (คุณตั้งเตือนไว้ที่ %s จุด)",
				weekday, date, percent(yesterday), weekday, percent(before), percent(drop), whole(threshold))
		}},
}

// digest writes the morning summary. Every line comes from a report the recipient may read; a report they may not read is
// left out, one that is still being fetched makes the whole digest wait for the next check, and when nothing may be read
// the rule answers like a missing one. A digest is not a threshold, so it always speaks once a day.
func (evaluator *Evaluator) digest(ctx context.Context, target agent.AlertTarget, local time.Time, location *time.Location) (evaluation, error) {
	yesterday := local.AddDate(0, 0, -1)
	before := yesterday.AddDate(0, 0, -7)
	a, b := yesterday.Format(time.DateOnly), before.Format(time.DateOnly)
	weekday := thaiWeekday(yesterday)
	lines := make([]string, 0, 3)
	stamps := make([]string, 0, 3)
	followUp := "" // a suggested question, taken from a line that is in the message, so a hidden report is never hinted at
	waiting := false

	compare, err := evaluator.source.Compare(ctx, target.Principal, string(report.SalesGoodsServices), "total_amount", a, a, b, b)
	switch {
	case errors.Is(err, agent.ErrNoData):
	case err != nil:
		return evaluation{}, err
	case compare.Status != "READY" || compare.A == nil || compare.B == nil:
		waiting = true
	default:
		yesterdayTotal, okA := new(big.Rat).SetString(compare.A.Value)
		weekBefore, okB := new(big.Rat).SetString(compare.B.Value)
		if !okA || !okB {
			return evaluation{}, fmt.Errorf("compare returned a figure that is not a number")
		}
		line := fmt.Sprintf("• ยอดขายเมื่อวาน (วัน%s ที่ %s): %s บาท", weekday, thaiDate(yesterday), baht(yesterdayTotal))
		if yesterdayTotal.Sign() == 0 {
			line = fmt.Sprintf("• ยอดขายเมื่อวาน (วัน%s ที่ %s): ไม่มียอดขายเลย (ร้านอาจปิดทำการ)", weekday, thaiDate(yesterday))
		} else if weekBefore.Sign() > 0 {
			change := new(big.Rat).Quo(new(big.Rat).Mul(new(big.Rat).Sub(yesterdayTotal, weekBefore), big.NewRat(100, 1)), weekBefore)
			direction := "สูงกว่า"
			if change.Sign() < 0 {
				direction, change = "ต่ำกว่า", new(big.Rat).Neg(change)
			}
			line += fmt.Sprintf(" %sวัน%sสัปดาห์ก่อน (%s บาท) อยู่ %s%%", direction, weekday, baht(weekBefore), percent(change))
		} else {
			line += fmt.Sprintf(" (วัน%sสัปดาห์ก่อนไม่มียอดขาย จึงไม่เทียบ)", weekday)
		}
		lines = append(lines, line)
		stamps = append(stamps, compare.CollectedAt)
		followUp = "ยอดขายเมื่อวานเป็นอย่างไรบ้าง"
	}

	for _, spec := range []struct {
		key, metric, followUp string
		line                  func(*big.Rat, agent.ReportResponse) string
	}{
		{string(report.ARAging), "overdue_amount", "ลูกหนี้รายไหนเลยกำหนดเยอะสุด", func(value *big.Rat, response agent.ReportResponse) string {
			line := fmt.Sprintf("• ลูกหนี้เลยกำหนด: %s บาท", baht(value))
			if amount := noDueDateAmount(response); amount != nil {
				line += fmt.Sprintf("\n  (ไม่รวมเอกสารที่ไม่มีวันครบกำหนด %s บาท เพราะไม่มีวันครบกำหนดในระบบ)", baht(amount))
			}
			return line
		}},
		{string(report.StockReorder), "reorder_item_count", "สินค้าอะไรใกล้หมดสต็อกบ้าง", func(value *big.Rat, _ agent.ReportResponse) string {
			return fmt.Sprintf("• สินค้าถึงจุดสั่งซื้อ: %s รายการ", whole(value))
		}},
	} {
		response, reportErr := evaluator.source.Report(ctx, target.Principal, spec.key, "", "")
		switch {
		case errors.Is(reportErr, agent.ErrNoData):
			continue
		case reportErr != nil:
			return evaluation{}, reportErr
		case response.Status != "READY" || response.Freshness != "FRESH":
			waiting = true
			continue
		}
		value, ok := kpi(response, spec.metric)
		if !ok {
			return evaluation{}, fmt.Errorf("report %s has no %s", spec.key, spec.metric)
		}
		lines = append(lines, spec.line(value, response))
		stamps = append(stamps, response.CollectedAt)
		if followUp == "" {
			followUp = spec.followUp
		}
	}
	if waiting {
		return evaluation{}, nil
	}
	if len(lines) == 0 {
		return evaluation{}, agent.ErrNoData
	}
	message := fmt.Sprintf("☀️ สรุปเช้าวัน%s ที่ %s\n\n%s", thaiWeekday(local), thaiDate(local), strings.Join(lines, "\n"))
	if stamp := oldest(stamps); stamp != "" {
		message += asOf(stamp, location)
	}
	message += "\n\nถามต่อได้เลย เช่น “" + followUp + "”\nอยากปิดสรุปเช้า พิมพ์ “ปิดสรุปเช้า”"
	return evaluation{ready: true, fired: true, value: big.NewRat(int64(len(lines)), 1), message: message}, nil
}

// oldest returns the earliest of the times the figures were read, so the message never claims to be fresher than its stalest line.
func oldest(stamps []string) string {
	var best time.Time
	text := ""
	for _, stamp := range stamps {
		moment, err := time.Parse(time.RFC3339, stamp)
		if err != nil {
			continue
		}
		if text == "" || moment.Before(best) {
			best, text = moment, stamp
		}
	}
	return text
}

func closing(def agent.AlertRuleDef, followUp string) string {
	return fmt.Sprintf("\n\nถามต่อได้เลย เช่น “%s”\nอยากปิดการเตือนนี้ พิมพ์ “ปิดเตือน%s”", followUp, def.Label)
}

// noDueDateAmount is the receivables that have no due date in the shop's system, when there are any. They are never
// counted as overdue, so a message about overdue amounts says so.
func noDueDateAmount(response agent.ReportResponse) *big.Rat {
	if amount, ok := kpi(response, "no_due_date_amount"); ok && amount.Sign() > 0 {
		return amount
	}
	return nil
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
