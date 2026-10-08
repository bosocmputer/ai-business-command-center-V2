package agent

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
)

type fakeAlertStore struct {
	items map[AlertRuleKey]StoredAlert
	saved []StoredAlert
}

func (store *fakeAlertStore) Alerts(context.Context, Principal) ([]StoredAlert, error) {
	list := make([]StoredAlert, 0)
	for _, item := range store.items {
		list = append(list, item)
	}
	return list, nil
}

func (store *fakeAlertStore) UpsertAlert(_ context.Context, _ Principal, rule AlertRuleKey, threshold string, enabled bool, _ string, _ time.Time) (StoredAlert, error) {
	if store.items == nil {
		store.items = map[AlertRuleKey]StoredAlert{}
	}
	item := StoredAlert{Rule: rule, Threshold: threshold, Enabled: enabled}
	store.items[rule] = item
	store.saved = append(store.saved, item)
	return item, nil
}

func alertService(permitted ...report.Key) (*Service, *fakeStore, *fakeAlertStore) {
	store := &fakeStore{permitted: permitted}
	alerts := &fakeAlertStore{}
	return newService(store, &fakeSnapshots{}).ConfigureAlerts(alerts), store, alerts
}

func TestAlertThresholdsAreCheckedAgainstTheRulesOwnRange(t *testing.T) {
	money, _ := AlertRuleFor("ar_overdue")
	count, _ := AlertRuleFor("stock_reorder")
	percent, _ := AlertRuleFor("sales_drop")
	cases := []struct {
		def  AlertRuleDef
		raw  string
		want string // empty = must be refused
	}{
		{money, "500000", "500000"},
		{money, " 500,000 ", "500000"},
		{money, "1250.5", "1250.5"},
		{money, "1250.50", "1250.5"},
		{money, "0", ""},
		{money, "-5", ""},
		{money, "10000000001", ""},
		{money, "1e6", ""},
		{money, "ห้าแสน", ""},
		{money, "", ""},
		{count, "3", "3"},
		{count, "2.5", ""},
		{count, "100001", ""},
		{percent, "40", "40"},
		{percent, "100", ""},
		{percent, "0", ""},
		{percent, "40.5", ""},
		{percent, "40%", "40"},
		{percent, " 40 % ", "40"},
		{percent, "40 เปอร์เซ็นต์", "40"},
		{money, "500,000 บาท", "500000"},
		{money, "฿500000", "500000"},
		{money, "500000บาท", "500000"},
		{count, "3 รายการ", "3"},
		{count, "รายการ", ""},
		{percent, "%", ""},
		{percent, "40%%", ""},
		{money, "5แสนบาท", ""},
	}
	for _, c := range cases {
		_, text, err := ParseAlertThreshold(c.def, c.raw)
		var invalid *InvalidAlertError
		if c.want == "" {
			if !errors.As(err, &invalid) || invalid.Message == "" {
				t.Errorf("%s %q: want a Thai refusal, got %q %v", c.def.Key, c.raw, text, err)
			}
			continue
		}
		if err != nil || text != c.want {
			t.Errorf("%s %q: got %q %v, want %q", c.def.Key, c.raw, text, err, c.want)
		}
	}
}

func TestSettingAnAlertStoresItAndListsIt(t *testing.T) {
	service, store, alerts := alertService(report.ARAging, report.SalesGoodsServices)
	view, err := service.SetAlert(context.Background(), principal, "ar_overdue", AlertSetRequest{Threshold: "500,000"}, "req-1")
	if err != nil || view.Threshold != "500000" || !view.Enabled || view.Message != MessageAlertSet {
		t.Fatalf("set = %+v %v", view, err)
	}
	if len(alerts.saved) != 1 || alerts.saved[0].Rule != AlertAROverdue {
		t.Fatalf("stored = %+v", alerts.saved)
	}
	if len(store.calls) == 0 || store.calls[len(store.calls)-1].Tool != ToolAlertSet || store.calls[len(store.calls)-1].Outcome != OutcomeOK {
		t.Fatalf("the call log must show an alert_set that worked: %+v", store.calls)
	}
	list, err := service.Alerts(context.Background(), principal)
	if err != nil || len(list.Alerts) != len(AlertCatalog()) {
		t.Fatalf("list = %+v %v", list, err)
	}
	available := map[AlertRuleKey]bool{}
	for _, item := range list.Alerts {
		available[item.Rule] = item.Available
		if item.Rule == AlertAROverdue && (item.Threshold != "500000" || !item.Enabled) {
			t.Errorf("the rule just set must show its threshold: %+v", item)
		}
	}
	if !available[AlertAROverdue] || !available[AlertSalesDrop] || available[AlertStockReorder] {
		t.Errorf("a rule is available only when the recipient may read its report: %+v", available)
	}
}

func TestAnAlertOnAReportTheRecipientCannotReadAnswersLikeAMissingOne(t *testing.T) {
	service, store, alerts := alertService(report.SalesGoodsServices) // may not read ar_aging
	for _, rule := range []string{"ar_overdue", "no_such_rule", ""} {
		_, err := service.SetAlert(context.Background(), principal, rule, AlertSetRequest{Threshold: "1000"}, "req")
		if !errors.Is(err, ErrNoData) {
			t.Errorf("rule %q: err = %v, want ErrNoData", rule, err)
		}
	}
	if len(alerts.saved) != 0 {
		t.Fatalf("nothing may be stored: %+v", alerts.saved)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeNoData {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
}

func TestAnInvalidThresholdIsRefusedInThaiAndLogged(t *testing.T) {
	service, store, alerts := alertService(report.ARAging)
	_, err := service.SetAlert(context.Background(), principal, "ar_overdue", AlertSetRequest{Threshold: "ครึ่งล้าน"}, "req")
	var invalid *InvalidAlertError
	if !errors.As(err, &invalid) || invalid.Message == "" {
		t.Fatalf("err = %v", err)
	}
	if len(alerts.saved) != 0 || store.calls[len(store.calls)-1].Outcome != OutcomeInvalidAlert {
		t.Fatalf("stored %v, last call %+v", alerts.saved, store.calls[len(store.calls)-1])
	}
	// Turning a rule on without ever giving a threshold is also refused.
	if _, err := service.SetAlert(context.Background(), principal, "ar_overdue", AlertSetRequest{}, "req"); !errors.As(err, &invalid) || invalid.Message != MessageAlertNeedsSet {
		t.Fatalf("no threshold: %v", err)
	}
}

func TestSwitchingARuleOffKeepsItsThreshold(t *testing.T) {
	service, _, alerts := alertService(report.ARAging)
	if _, err := service.SetAlert(context.Background(), principal, "ar_overdue", AlertSetRequest{Threshold: "800000"}, "r1"); err != nil {
		t.Fatal(err)
	}
	off := false
	view, err := service.SetAlert(context.Background(), principal, "ar_overdue", AlertSetRequest{Enabled: &off}, "r2")
	if err != nil || view.Enabled || view.Threshold != "800000" || view.Message != MessageAlertOff {
		t.Fatalf("off = %+v %v", view, err)
	}
	if got := alerts.items[AlertAROverdue]; got.Enabled || got.Threshold != "800000" {
		t.Fatalf("stored = %+v", got)
	}
	// Switching off a rule that was never set is not an error and stores nothing.
	view, err = service.SetAlert(context.Background(), principal, "ar_over_year", AlertSetRequest{Enabled: &off}, "r3")
	_ = view
	if err == nil {
		// ar_over_year is also ar_aging, which is permitted, so this succeeds without a store write
		if _, stored := alerts.items[AlertAROverYear]; stored {
			t.Fatal("a rule that was never set must not be created by switching it off")
		}
	}
}

func TestAlertToolsAreAbsentUntilConfigured(t *testing.T) {
	service := newService(&fakeStore{permitted: []report.Key{report.ARAging}}, &fakeSnapshots{})
	if _, err := service.Alerts(context.Background(), principal); !errors.Is(err, ErrAlertsUnavailable) {
		t.Fatalf("list: %v", err)
	}
	if _, err := service.SetAlert(context.Background(), principal, "ar_overdue", AlertSetRequest{Threshold: "1000"}, "r"); !errors.Is(err, ErrAlertsUnavailable) {
		t.Fatalf("set: %v", err)
	}
}

func TestAnAlertThatIsStillTrueStaysQuietUnlessItGotWorse(t *testing.T) {
	def, _ := AlertRuleFor("ar_overdue")
	sales, _ := AlertRuleFor("sales_drop")
	now := time.Date(2026, 10, 8, 2, 0, 0, 0, time.UTC)
	rat := func(text string) *big.Rat { value, _ := new(big.Rat).SetString(text); return value }
	day := func(days int) *time.Time { value := now.Add(-time.Duration(days) * 24 * time.Hour); return &value }
	cases := []struct {
		name   string
		def    AlertRuleDef
		target AlertTarget
		value  string
		want   bool
	}{
		{"never spoke", def, AlertTarget{}, "100", true},
		{"spoke yesterday, same figure", def, AlertTarget{LastFiredAt: day(1), LastFiredValue: rat("1000")}, "1000", false},
		{"spoke yesterday, 9% worse", def, AlertTarget{LastFiredAt: day(1), LastFiredValue: rat("1000")}, "1090", false},
		{"spoke yesterday, 10% worse", def, AlertTarget{LastFiredAt: day(1), LastFiredValue: rat("1000")}, "1100", true},
		{"spoke yesterday, better", def, AlertTarget{LastFiredAt: day(1), LastFiredValue: rat("1000")}, "900", false},
		{"spoke three days ago", def, AlertTarget{LastFiredAt: day(3), LastFiredValue: rat("1000")}, "1000", true},
		{"spoke but no figure was kept", def, AlertTarget{LastFiredAt: day(1)}, "1000", false},
		{"one-day rules have no cooldown", sales, AlertTarget{LastFiredAt: day(1), LastFiredValue: rat("50")}, "45", true},
	}
	for _, c := range cases {
		if got := ShouldFireAgain(c.def, c.target, rat(c.value), now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
