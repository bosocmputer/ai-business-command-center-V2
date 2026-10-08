package alert

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/google/uuid"
)

func rat(text string) *big.Rat { value, _ := new(big.Rat).SetString(text); return value }

type fakeStore struct {
	targets []agent.AlertTarget
	checked map[uuid.UUID]string // rule -> last day marked OK or NO_ACCESS
	status  map[uuid.UUID]string
	events  []agent.AlertEvent
	fired   map[string]bool // ruleID|day
}

func newFakeStore(targets ...agent.AlertTarget) *fakeStore {
	return &fakeStore{targets: targets, checked: map[uuid.UUID]string{}, status: map[uuid.UUID]string{}, fired: map[string]bool{}}
}

func (store *fakeStore) AlertTargets(context.Context, time.Time) ([]agent.AlertTarget, error) {
	out := make([]agent.AlertTarget, 0)
	for _, target := range store.targets {
		target.LastCheckedOn = store.checked[target.RuleID]
		out = append(out, target)
	}
	return out, nil
}
func (store *fakeStore) MarkAlertChecked(_ context.Context, id uuid.UUID, on, status string) error {
	store.status[id] = status
	if status == agent.AlertCheckOK || status == agent.AlertCheckNoAccess {
		store.checked[id] = on
	}
	return nil
}
func (store *fakeStore) RecordAlertFire(_ context.Context, target agent.AlertTarget, on, _, _, message, status string, _ time.Time) (agent.AlertEvent, bool, error) {
	key := target.RuleID.String() + "|" + on
	if store.fired[key] {
		return agent.AlertEvent{}, false, nil
	}
	store.fired[key] = true
	event := agent.AlertEvent{ID: uuid.New(), RuleID: target.RuleID, Rule: target.Rule, FiredOn: on, Message: message, Status: status}
	store.events = append(store.events, event)
	return event, true, nil
}
func (store *fakeStore) UnsentAlerts(context.Context, time.Time) ([]agent.AlertEvent, error) {
	out := make([]agent.AlertEvent, 0)
	for _, event := range store.events {
		if event.Status == agent.AlertEventPending || event.Status == agent.AlertEventFailed {
			out = append(out, event)
		}
	}
	return out, nil
}
func (store *fakeStore) MarkAlertDelivery(_ context.Context, id uuid.UUID, delivered bool, _ string, _ time.Time) error {
	for index := range store.events {
		if store.events[index].ID == id {
			store.events[index].Status = agent.AlertEventFailed
			if delivered {
				store.events[index].Status = agent.AlertEventSent
			}
		}
	}
	return nil
}

type fakeSource struct {
	reports  map[string]agent.ReportResponse
	compare  agent.CompareResponse
	err      error
	requests []string
}

func (source *fakeSource) Report(_ context.Context, _ agent.Principal, key, _, _ string) (agent.ReportResponse, error) {
	source.requests = append(source.requests, key)
	return source.reports[key], source.err
}
func (source *fakeSource) Compare(_ context.Context, _ agent.Principal, _, _, aFrom, _, bFrom, _ string) (agent.CompareResponse, error) {
	source.requests = append(source.requests, "compare "+aFrom+" vs "+bFrom)
	return source.compare, source.err
}

type fakeSender struct {
	sent []agent.AlertEvent
	err  error
}

func (sender *fakeSender) Send(_ context.Context, event agent.AlertEvent) error {
	if sender.err != nil {
		return sender.err
	}
	sender.sent = append(sender.sent, event)
	return nil
}

func ready(key string, collected string, kpis map[string]string) agent.ReportResponse {
	response := agent.ReportResponse{Status: "READY", Freshness: "FRESH", CollectedAt: collected}
	for name, value := range kpis {
		response.KPIs = append(response.KPIs, agent.KPI{Key: name, Value: value})
	}
	return response
}

func target(rule agent.AlertRuleKey, threshold string) agent.AlertTarget {
	return agent.AlertTarget{RuleID: uuid.New(), Rule: rule, Threshold: rat(threshold), Principal: agent.Principal{TokenID: uuid.New(), TenantID: uuid.New(), RecipientID: uuid.New(), Timezone: "Asia/Bangkok"}}
}

var (
	// 09:00 in Bangkok on Thursday 8 Oct 2026, after the 08:15 start. Yesterday, 7 Oct, was a Wednesday.
	nineAM = time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)
	quiet  = slog.New(slog.NewJSONHandler(io.Discard, nil))
)

func newEvaluator(store *fakeStore, source *fakeSource, sender Sender, dryRun bool, at time.Time) *Evaluator {
	return NewEvaluator(store, source, sender, dryRun, func() time.Time { return at }, quiet)
}

const collected = "2026-10-08T07:31:00+07:00"

func TestAnOverdueAlertSpeaksOnlyAboveTheOwnersThreshold(t *testing.T) {
	above := target(agent.AlertAROverdue, "500000")
	store := newFakeStore(above)
	source := &fakeSource{reports: map[string]agent.ReportResponse{"ar_aging": ready("ar_aging", collected, map[string]string{"overdue_amount": "641200.50"})}}
	sender := &fakeSender{}
	summary := newEvaluator(store, source, sender, false, nineAM).RunOnce(context.Background())
	if summary.Fired != 1 || summary.Sent != 1 || len(sender.sent) != 1 {
		t.Fatalf("summary = %+v, sent = %d", summary, len(sender.sent))
	}
	message := sender.sent[0].Message
	for _, want := range []string{"641,200.50 บาท", "500,000.00 บาท", "8 ต.ค. 2569 เวลา 07:31 น.", "ลูกหนี้รายไหนเลยกำหนดเยอะสุด", "ปิดเตือนยอดลูกหนี้เลยกำหนด"} {
		if !strings.Contains(message, want) {
			t.Errorf("message lacks %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "2026") {
		t.Errorf("years are written in the Buddhist Era:\n%s", message)
	}

	below := target(agent.AlertAROverdue, "900000")
	store = newFakeStore(below)
	sender = &fakeSender{}
	summary = newEvaluator(store, source, sender, false, nineAM).RunOnce(context.Background())
	if summary.Fired != 0 || summary.Checked != 1 || len(sender.sent) != 0 || store.checked[below.RuleID] != "2026-10-08" {
		t.Fatalf("below the threshold: %+v, checked=%v", summary, store.checked)
	}
}

func TestADryRunRecordsWhatItWouldSayAndSendsNothing(t *testing.T) {
	item := target(agent.AlertStockReorder, "1")
	store := newFakeStore(item)
	source := &fakeSource{reports: map[string]agent.ReportResponse{"stock_reorder": ready("stock_reorder", collected, map[string]string{"reorder_item_count": "4"})}}
	sender := &fakeSender{}
	summary := newEvaluator(store, source, sender, true, nineAM).RunOnce(context.Background())
	if summary.Fired != 1 || summary.Sent != 0 || len(sender.sent) != 0 || len(store.events) != 1 || store.events[0].Status != agent.AlertEventDryRun {
		t.Fatalf("summary = %+v, events = %+v", summary, store.events)
	}
	if !strings.Contains(store.events[0].Message, "4 รายการ") {
		t.Errorf("message = %s", store.events[0].Message)
	}
}

func TestAReportThatIsNotFreshYetIsWaitedForNotUsed(t *testing.T) {
	item := target(agent.AlertAROverdue, "1000")
	store := newFakeStore(item)
	stale := ready("ar_aging", collected, map[string]string{"overdue_amount": "999999"})
	stale.Freshness = "STALE"
	source := &fakeSource{reports: map[string]agent.ReportResponse{"ar_aging": stale}}
	sender := &fakeSender{}
	evaluator := newEvaluator(store, source, sender, false, nineAM)
	if summary := evaluator.RunOnce(context.Background()); summary.NotReady != 1 || summary.Fired != 0 || store.status[item.RuleID] != agent.AlertCheckNotReady || store.checked[item.RuleID] != "" {
		t.Fatalf("a stale report must not be used or counted as today's check: %+v %v", summary, store.status)
	}
	source.reports["ar_aging"] = agent.ReportResponse{Status: "PREPARING"}
	if summary := evaluator.RunOnce(context.Background()); summary.NotReady != 1 {
		t.Fatalf("preparing: %+v", summary)
	}
	source.reports["ar_aging"] = ready("ar_aging", collected, map[string]string{"overdue_amount": "999999"})
	if summary := evaluator.RunOnce(context.Background()); summary.Fired != 1 || summary.Sent != 1 {
		t.Fatalf("once the fresh report is there it is used: %+v", summary)
	}
	if summary := evaluator.RunOnce(context.Background()); summary.Checked != 0 || summary.Fired != 0 {
		t.Fatalf("a rule checked today is left alone: %+v", summary)
	}
}

func TestTheCheckWaitsForMorningAndGivesUpAfterTheWindow(t *testing.T) {
	item := target(agent.AlertAROverdue, "1000")
	source := &fakeSource{reports: map[string]agent.ReportResponse{"ar_aging": ready("ar_aging", collected, map[string]string{"overdue_amount": "5000"})}}
	for name, at := range map[string]time.Time{
		"07:59 is before the morning prefetch":     time.Date(2026, 10, 8, 0, 59, 0, 0, time.UTC),
		"08:14 is still before the start":          time.Date(2026, 10, 8, 1, 14, 0, 0, time.UTC),
		"12:16 is past the four hour window":       time.Date(2026, 10, 8, 5, 16, 0, 0, time.UTC),
		"midnight belongs to the previous workday": time.Date(2026, 10, 7, 17, 5, 0, 0, time.UTC),
	} {
		store := newFakeStore(item)
		if summary := newEvaluator(store, source, &fakeSender{}, false, at).RunOnce(context.Background()); summary != (Summary{}) || len(source.requests) != 0 {
			t.Errorf("%s: %+v, requests %v", name, summary, source.requests)
		}
	}
	store := newFakeStore(item)
	if summary := newEvaluator(store, source, &fakeSender{}, false, time.Date(2026, 10, 8, 1, 15, 0, 0, time.UTC)).RunOnce(context.Background()); summary.Fired != 1 {
		t.Fatalf("08:15 exactly is in the window: %+v", summary)
	}
}

func TestAStillTrueRuleStaysQuietForThreeDaysUnlessItGotWorse(t *testing.T) {
	item := target(agent.AlertAROverdue, "500000")
	yesterday := nineAM.Add(-24 * time.Hour)
	item.LastFiredAt, item.LastFiredValue = &yesterday, rat("600000")
	source := &fakeSource{reports: map[string]agent.ReportResponse{"ar_aging": ready("ar_aging", collected, map[string]string{"overdue_amount": "620000"})}}
	store := newFakeStore(item)
	if summary := newEvaluator(store, source, &fakeSender{}, false, nineAM).RunOnce(context.Background()); summary.Fired != 0 || summary.Checked != 1 {
		t.Fatalf("3%% worse the next day stays quiet: %+v", summary)
	}
	source.reports["ar_aging"] = ready("ar_aging", collected, map[string]string{"overdue_amount": "660000"})
	store = newFakeStore(item)
	if summary := newEvaluator(store, source, &fakeSender{}, false, nineAM).RunOnce(context.Background()); summary.Fired != 1 {
		t.Fatalf("10%% worse speaks again: %+v", summary)
	}
}

func TestASecondRunOnTheSameDayNeverSendsTwice(t *testing.T) {
	item := target(agent.AlertAROverdue, "1000")
	store := newFakeStore(item)
	source := &fakeSource{reports: map[string]agent.ReportResponse{"ar_aging": ready("ar_aging", collected, map[string]string{"overdue_amount": "5000"})}}
	sender := &fakeSender{}
	evaluator := newEvaluator(store, source, sender, false, nineAM)
	evaluator.RunOnce(context.Background())
	store.checked = map[uuid.UUID]string{} // as if the worker restarted before the check was marked
	evaluator.RunOnce(context.Background())
	if len(sender.sent) != 1 || len(store.events) != 1 {
		t.Fatalf("sent %d, recorded %d; a repeat the same day must change nothing", len(sender.sent), len(store.events))
	}
}

func TestADeliveryThatFailsIsTriedAgainAndDoesNotLoseTheAlert(t *testing.T) {
	item := target(agent.AlertAROverdue, "1000")
	store := newFakeStore(item)
	source := &fakeSource{reports: map[string]agent.ReportResponse{"ar_aging": ready("ar_aging", collected, map[string]string{"overdue_amount": "5000"})}}
	sender := &fakeSender{err: &SendError{Code: "UNREACHABLE"}}
	evaluator := newEvaluator(store, source, sender, false, nineAM)
	if summary := evaluator.RunOnce(context.Background()); summary.Fired != 1 || summary.Failed != 1 || store.events[0].Status != agent.AlertEventFailed {
		t.Fatalf("first run: %+v %+v", summary, store.events)
	}
	sender.err = nil
	if summary := evaluator.RunOnce(context.Background()); summary.Sent != 1 || store.events[0].Status != agent.AlertEventSent || len(sender.sent) != 1 {
		t.Fatalf("second run: %+v %+v", summary, store.events)
	}
}

func TestARuleThatCannotBeReadIsMarkedAndAnErrorIsRetried(t *testing.T) {
	item := target(agent.AlertAROverdue, "1000")
	store := newFakeStore(item)
	source := &fakeSource{err: agent.ErrNoData}
	evaluator := newEvaluator(store, source, &fakeSender{}, false, nineAM)
	evaluator.RunOnce(context.Background())
	if store.status[item.RuleID] != agent.AlertCheckNoAccess || store.checked[item.RuleID] != "2026-10-08" {
		t.Fatalf("no access: %v %v", store.status, store.checked)
	}
	store = newFakeStore(item)
	source.err = errors.New("database is down")
	evaluator = newEvaluator(store, source, &fakeSender{}, false, nineAM)
	if summary := evaluator.RunOnce(context.Background()); summary.Errors != 1 || store.checked[item.RuleID] != "" || store.status[item.RuleID] != agent.AlertCheckError {
		t.Fatalf("error: %+v %v %v", summary, store.status, store.checked)
	}
}

func TestSalesDropComparesYesterdayWithTheSameWeekdayAWeekEarlier(t *testing.T) {
	item := target(agent.AlertSalesDrop, "30")
	compare := agent.CompareResponse{Status: "READY", A: &agent.CompareSide{Value: "60000.00"}, B: &agent.CompareSide{Value: "100000.00"}, CollectedAt: collected}
	source := &fakeSource{compare: compare}
	store := newFakeStore(item)
	sender := &fakeSender{}
	if summary := newEvaluator(store, source, sender, false, nineAM).RunOnce(context.Background()); summary.Fired != 1 || len(sender.sent) != 1 {
		t.Fatalf("a 40%% drop against a 30%% threshold: %+v", summary)
	}
	if source.requests[0] != "compare 2026-10-07 vs 2026-09-30" {
		t.Errorf("periods = %v", source.requests)
	}
	for _, want := range []string{"วันพุธ ที่ 7 ต.ค. 2569", "60,000.00 บาท", "100,000.00 บาท", "40%", "30%"} {
		if !strings.Contains(sender.sent[0].Message, want) {
			t.Errorf("message lacks %q:\n%s", want, sender.sent[0].Message)
		}
	}

	small := agent.CompareResponse{Status: "READY", A: &agent.CompareSide{Value: "80000.00"}, B: &agent.CompareSide{Value: "100000.00"}}
	source.compare = small
	store = newFakeStore(item)
	if summary := newEvaluator(store, source, &fakeSender{}, false, nineAM).RunOnce(context.Background()); summary.Fired != 0 || summary.Checked != 1 {
		t.Fatalf("a 20%% drop is under 30%%: %+v", summary)
	}
	// The same weekday a week ago was a closed day (zero): nothing to compare, no alert, and no divide by zero.
	source.compare = agent.CompareResponse{Status: "READY", A: &agent.CompareSide{Value: "0.00"}, B: &agent.CompareSide{Value: "0.00"}}
	store = newFakeStore(item)
	if summary := newEvaluator(store, source, &fakeSender{}, false, nineAM).RunOnce(context.Background()); summary.Fired != 0 || summary.Checked != 1 {
		t.Fatalf("zero base: %+v", summary)
	}
	source.compare = agent.CompareResponse{Status: "PREPARING"}
	store = newFakeStore(item)
	if summary := newEvaluator(store, source, &fakeSender{}, false, nineAM).RunOnce(context.Background()); summary.NotReady != 1 {
		t.Fatalf("preparing: %+v", summary)
	}
}

func TestMessagesUseThaiYearsAndThousandsSeparators(t *testing.T) {
	if got := baht(rat("6566875.1")); got != "6,566,875.10" {
		t.Errorf("baht = %s", got)
	}
	if got := baht(rat("-12.5")); got != "-12.50" {
		t.Errorf("negative baht = %s", got)
	}
	if got := whole(rat("1234567")); got != "1,234,567" {
		t.Errorf("whole = %s", got)
	}
	if got := percent(rat("40")); got != "40" {
		t.Errorf("percent = %s", got)
	}
	if got := percent(rat("33.33")); got != "33.3" {
		t.Errorf("percent = %s", got)
	}
	if got := thaiDate(time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)); got != "7 ต.ค. 2569" {
		t.Errorf("thaiDate = %s", got)
	}
	if got := thaiDateTime("2026-10-08T07:31:00+07:00", time.FixedZone("x", 7*3600)); got != "8 ต.ค. 2569 เวลา 07:31 น." {
		t.Errorf("thaiDateTime = %s", got)
	}
	if got := thaiDateTime("garbage", time.UTC); got != "" {
		t.Errorf("an unreadable time must give nothing: %q", got)
	}
}
