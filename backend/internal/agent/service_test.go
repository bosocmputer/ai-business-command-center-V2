package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

type testHasher struct{}

func (testHasher) HashToken(token string) []byte {
	sum := sha256.Sum256([]byte("k" + token))
	return sum[:]
}

type fakeStore struct {
	principals map[string]Principal
	permitted  []report.Key
	calls      []Call
	callsUsed  int
	preparing  int
	delivered  *Delivered
	issuedHash []byte
	issueErr   error
}

func (store *fakeStore) Authenticate(_ context.Context, hash []byte, _ time.Time) (Principal, error) {
	if principal, ok := store.principals[string(hash)]; ok {
		return principal, nil
	}
	return Principal{}, ErrUnauthorized
}
func (store *fakeStore) TouchToken(context.Context, uuid.UUID, time.Time) error { return nil }
func (store *fakeStore) CallsSince(context.Context, uuid.UUID, time.Time) (int, error) {
	return store.callsUsed, nil
}
func (store *fakeStore) PreparingSince(context.Context, uuid.UUID, time.Time) (int, error) {
	return store.preparing, nil
}
func (store *fakeStore) RecordCall(_ context.Context, call Call, _ time.Time) error {
	store.calls = append(store.calls, call)
	return nil
}
func (store *fakeStore) PermittedReports(context.Context, Principal, time.Time) ([]report.Key, error) {
	return store.permitted, nil
}
func (store *fakeStore) LatestDelivery(context.Context, Principal, report.Key) (Delivered, error) {
	if store.delivered == nil {
		return Delivered{}, ErrNoData
	}
	return *store.delivered, nil
}
func (store *fakeStore) IssueToken(_ context.Context, _ []byte, _ string, _, _ uuid.UUID, namesVisible bool, hash []byte, expiresAt, now time.Time) (TokenInfo, error) {
	store.issuedHash = hash
	return TokenInfo{Status: "ACTIVE", NamesVisible: namesVisible, CreatedAt: &now, ExpiresAt: &expiresAt}, store.issueErr
}
func (store *fakeStore) RevokeToken(context.Context, []byte, string, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (store *fakeStore) TokenInfo(context.Context, uuid.UUID, uuid.UUID, time.Time) (TokenInfo, error) {
	return TokenInfo{Status: "NONE"}, nil
}

type fakeSnapshots struct {
	exact        map[string]viewer.DashboardSnapshot // key: reportKey|from|to
	revalidation viewer.ReportRevalidation
	revalidated  int
}

func snapshotKey(key report.Key, period report.Period) string {
	return string(key) + "|" + period.DateFrom + "|" + period.DateTo
}

func (snapshots *fakeSnapshots) GetExactSnapshotForPeriod(_ context.Context, _ uuid.UUID, key report.Key, period report.Period, _ time.Time) (viewer.DashboardSnapshot, error) {
	if snapshot, ok := snapshots.exact[snapshotKey(key, period)]; ok {
		return snapshot, nil
	}
	return viewer.DashboardSnapshot{}, report.ErrRunNotFound
}
func (snapshots *fakeSnapshots) RevalidateSnapshot(context.Context, uuid.UUID, report.Key, report.Period, time.Time) (viewer.ReportRevalidation, error) {
	snapshots.revalidated++
	return snapshots.revalidation, nil
}

var (
	now       = time.Date(2026, 10, 1, 5, 0, 0, 0, time.UTC) // 12:00 in Bangkok
	tenantID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	principal = Principal{TokenID: uuid.New(), TenantID: tenantID, RecipientID: uuid.New(), ShopName: "ร้านทดสอบ", Timezone: "Asia/Bangkok"}
)

func newService(store *fakeStore, snapshots *fakeSnapshots) *Service {
	if store.principals == nil {
		store.principals = map[string]Principal{string(testHasher{}.HashToken("abcc_valid")): principal}
	}
	return NewService(store, snapshots, testHasher{}, bytes.NewReader(bytes.Repeat([]byte{7}, 64)), Alias(testHasher{}), func() time.Time { return now }, Config{})
}

func finished(minutesAgo int) *time.Time {
	value := now.Add(-time.Duration(minutesAgo) * time.Minute)
	return &value
}

func salesDashboard(period report.Period, total string) report.Dashboard {
	return report.Dashboard{
		ReportKey: report.SalesGoodsServices, Period: period, ComparisonPeriod: report.Period{DateFrom: "2026-08-02", DateTo: "2026-08-31"},
		KPIs: []report.DashboardMetric{
			{Key: "total_amount", Label: "ยอดขายรวม", Value: total, Unit: report.UnitTHB, Comparison: report.MetricComparison{Availability: report.ComparisonAvailable, PreviousValue: "100.00", Delta: "50.00", Percent: "50.0", Direction: report.DirectionUp}},
			{Key: "document_count", Label: "เอกสาร", Value: "12", Unit: report.UnitCount},
		},
		Visualizations: []report.DashboardVisualization{{Key: "top_products", Title: "สินค้า", Intent: report.IntentRanking, Unit: report.UnitTHB, Categories: []string{"ปูนซีเมนต์"}, Series: []report.VisualizationSeries{{Key: "value", Label: "สินค้า", Values: []string{"90.00"}}}}},
		Quality:        report.DashboardQuality{Status: "OK", Warnings: []string{}},
	}
}

func fresh(dashboard report.Dashboard) viewer.DashboardSnapshot {
	return viewer.DashboardSnapshot{RunID: uuid.New(), Dashboard: dashboard, SourceFinishedAt: finished(5), FreshnessStatus: viewer.FreshnessFresh}
}

func TestAuthenticateGivesTheSameRefusalForEveryReason(t *testing.T) {
	service := newService(&fakeStore{}, &fakeSnapshots{})
	for _, token := range []string{"", "   ", "abcc_unknown", strings.Repeat("x", 500)} {
		if _, err := service.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("token %.10q: err = %v, want ErrUnauthorized", token, err)
		}
	}
	got, err := service.Authenticate(context.Background(), " abcc_valid ")
	if err != nil || got.TenantID != tenantID {
		t.Fatalf("a valid token must work, got %+v %v", got, err)
	}
}

func TestAuthenticateStopsACallerThatAsksTooOften(t *testing.T) {
	store := &fakeStore{callsUsed: 60}
	_, err := newService(store, &fakeSnapshots{}).Authenticate(context.Background(), "abcc_valid")
	var limited *RateLimitedError
	if !errors.As(err, &limited) || limited.RetryAfter <= 0 {
		t.Fatalf("err = %v, want a rate limit with a wait time", err)
	}
	if len(store.calls) != 1 || store.calls[0].Outcome != OutcomeRateLimited {
		t.Fatalf("the refusal should be logged once as RATE_LIMITED, got %+v", store.calls)
	}
}

func TestAReportThatIsForbiddenAnswersLikeOneThatDoesNotExist(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	service := newService(store, &fakeSnapshots{})
	for _, key := range []string{"ar_aging", "no_such_report", "", "../../x"} {
		if _, err := service.Report(context.Background(), principal, key, "", ""); !errors.Is(err, ErrNoData) {
			t.Errorf("report %q: err = %v, want ErrNoData", key, err)
		}
	}
	if _, err := service.Compare(context.Background(), principal, "ar_aging", "total_amount", "2026-09-01", "2026-09-30", "2026-08-01", "2026-08-31"); !errors.Is(err, ErrNoData) {
		t.Errorf("compare on a forbidden report: err = %v", err)
	}
	if _, err := service.LatestDelivery(context.Background(), principal, "ar_aging"); !errors.Is(err, ErrNoData) {
		t.Errorf("latest delivery on a forbidden report: err = %v", err)
	}
}

func TestContextListsOnlyPermittedActiveReportsAndSaysNamesAreMasked(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.ARAging, report.SalesGoodsServices}, callsUsed: 4}
	response, err := newService(store, &fakeSnapshots{}).Context(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	if response.Shop != "ร้านทดสอบ" || response.Today != "2026-10-01" || response.Limits.CallsRemaining != 56 {
		t.Fatalf("context = %+v", response)
	}
	keys := []report.Key{}
	for _, info := range response.Reports {
		keys = append(keys, info.Key)
	}
	if len(keys) != 2 || keys[0] != report.SalesGoodsServices || keys[1] != report.ARAging {
		t.Fatalf("reports = %v, want only the two permitted, in catalog order", keys)
	}
	if len(response.Notes) != 1 || response.Notes[0] != MessageMasked {
		t.Fatalf("a token that may not see names must say names are masked: %v", response.Notes)
	}
}

func TestReportDefaultsAndRefusesPeriodsThatCannotBeRight(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices, report.ARAging}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{
		"sales_goods_services|2026-10-01|2026-10-01": fresh(salesDashboard(report.Period{DateFrom: "2026-10-01", DateTo: "2026-10-01"}, "10.00")),
	}}
	service := newService(store, snapshots)
	if _, err := service.Report(context.Background(), principal, "sales_goods_services", "", ""); err != nil {
		t.Fatalf("default period: %v", err)
	}
	if got := store.calls[len(store.calls)-1]; got.PeriodFrom != "2026-10-01" || got.PeriodTo != "2026-10-01" {
		t.Fatalf("the default period is the month so far, 2026-10-01 to today: %+v", got)
	}
	for name, dates := range map[string][2]string{
		"future end": {"2026-09-01", "2026-10-02"}, "reversed": {"2026-09-30", "2026-09-01"}, "too long": {"2025-01-01", "2026-09-30"},
		"only a start": {"2026-09-01", ""}, "not a date": {"yesterday", "today"},
	} {
		if _, err := service.Report(context.Background(), principal, "sales_goods_services", dates[0], dates[1]); !errors.Is(err, ErrInvalidPeriod) {
			t.Errorf("%s: err = %v, want ErrInvalidPeriod", name, err)
		}
	}
	// An as-of report takes one date, and defaults to today.
	if _, err := service.Report(context.Background(), principal, "ar_aging", "", ""); err != nil {
		t.Fatalf("as-of default: %v", err)
	}
	if got := store.calls[len(store.calls)-1]; got.PeriodFrom != "2026-10-01" || got.PeriodTo != "2026-10-01" {
		t.Fatalf("as-of period = %+v", got)
	}
}

func TestReportReturnsTheStoredNumbersWithPeriodAndCollectedTime(t *testing.T) {
	period := report.Period{DateFrom: "2026-09-01", DateTo: "2026-09-30"}
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{snapshotKey(report.SalesGoodsServices, period): fresh(salesDashboard(period, "150.00"))}}
	response, err := newService(store, snapshots).Report(context.Background(), principal, "sales_goods_services", "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "READY" || response.Freshness != "FRESH" || response.Period.DateFrom != "2026-09-01" || response.CollectedAt == "" {
		t.Fatalf("response = %+v", response)
	}
	if response.KPIs[0].Value != "150.00" || response.KPIs[0].Comparison == nil || response.KPIs[0].Comparison.Percent != "50.0" {
		t.Fatalf("kpis = %+v", response.KPIs)
	}
	if !strings.HasSuffix(response.CollectedAt, "+07:00") {
		t.Fatalf("collectedAt must be in the shop's time zone, got %s", response.CollectedAt)
	}
	if snapshots.revalidated != 0 {
		t.Fatal("a fresh snapshot must be served without any fetch")
	}
	if got := store.calls[0]; got.Outcome != OutcomeOK || got.SnapshotRunID == nil {
		t.Fatalf("the call log should hold the snapshot run: %+v", got)
	}
}

func TestMissingSnapshotStartsAFetchAndSaysPreparing(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	snapshots := &fakeSnapshots{revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationMissingRefreshing, RetryAfter: 90}}
	response, err := newService(store, snapshots).Report(context.Background(), principal, "sales_goods_services", "2026-09-01", "2026-09-30")
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "PREPARING" || response.RetryAfterSeconds != 90 || response.Message != MessagePreparing || len(response.KPIs) != 0 {
		t.Fatalf("response = %+v", response)
	}
	if snapshots.revalidated != 1 || store.calls[0].Outcome != OutcomePreparing {
		t.Fatalf("one fetch should have been requested and logged as PREPARING: %d, %+v", snapshots.revalidated, store.calls)
	}
}

func TestStaleSnapshotIsServedAndMarkedWhileItRefreshes(t *testing.T) {
	period := report.Period{DateFrom: "2026-09-01", DateTo: "2026-09-30"}
	stale := fresh(salesDashboard(period, "150.00"))
	stale.FreshnessStatus = viewer.FreshnessStale
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	snapshots := &fakeSnapshots{
		exact:        map[string]viewer.DashboardSnapshot{snapshotKey(report.SalesGoodsServices, period): stale},
		revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationStaleRefreshing, Snapshot: &stale},
	}
	response, err := newService(store, snapshots).Report(context.Background(), principal, "sales_goods_services", "2026-09-01", "2026-09-30")
	if err != nil || response.Status != "READY" || response.Freshness != "STALE" || response.KPIs[0].Value != "150.00" {
		t.Fatalf("response = %+v, %v", response, err)
	}
	if !strings.Contains(strings.Join(response.Warnings, "|"), MessageStale) {
		t.Fatalf("a stale answer must say so: %v", response.Warnings)
	}
}

func TestTheHourlyFetchBudgetIsRespected(t *testing.T) {
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}, preparing: 10}
	snapshots := &fakeSnapshots{revalidation: viewer.ReportRevalidation{Disposition: viewer.RevalidationMissingRefreshing}}
	response, err := newService(store, snapshots).Report(context.Background(), principal, "sales_goods_services", "2026-09-01", "2026-09-30")
	if err != nil || response.Status != "UNAVAILABLE" || response.Message != MessageBudgetUsed {
		t.Fatalf("response = %+v, %v", response, err)
	}
	if snapshots.revalidated != 0 {
		t.Fatal("once the budget is used no further fetch may be started")
	}
}

func TestPeopleNamesAreMaskedUnlessTheTokenMaySeeThem(t *testing.T) {
	period := report.Period{DateFrom: "2026-10-01", DateTo: "2026-10-01"}
	dashboard := report.Dashboard{ReportKey: report.ARAging, Period: period, Visualizations: []report.DashboardVisualization{
		{Key: "top_debtors", Title: "ลูกหนี้", Intent: report.IntentRanking, Unit: report.UnitTHB, Categories: []string{"บริษัท ก จำกัด", "คุณ ข"}, Series: []report.VisualizationSeries{{Key: "value", Label: "x", Values: []string{"2.00", "1.00"}}}},
		{Key: "top_products", Title: "สินค้า", Intent: report.IntentRanking, Unit: report.UnitTHB, Categories: []string{"ปูนซีเมนต์"}, Series: []report.VisualizationSeries{{Key: "value", Label: "x", Values: []string{"1.00"}}}},
	}}
	store := &fakeStore{permitted: []report.Key{report.ARAging}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{snapshotKey(report.ARAging, period): fresh(dashboard)}}
	service := newService(store, snapshots)

	masked, err := service.Report(context.Background(), principal, "ar_aging", "", "")
	if err != nil {
		t.Fatal(err)
	}
	debtors := masked.Visualizations[0].Categories
	if strings.Contains(strings.Join(debtors, "|"), "บริษัท ก") || !strings.HasPrefix(debtors[0], "ลูกค้า-") || debtors[0] == debtors[1] {
		t.Fatalf("debtor names must become distinct aliases: %v", debtors)
	}
	if masked.Visualizations[1].Categories[0] != "ปูนซีเมนต์" {
		t.Fatalf("product names are not personal data and stay: %v", masked.Visualizations[1].Categories)
	}
	again, _ := service.Report(context.Background(), principal, "ar_aging", "", "")
	if again.Visualizations[0].Categories[0] != debtors[0] {
		t.Fatal("the same name must keep the same alias, so a conversation can refer back to it")
	}
	if !strings.Contains(strings.Join(masked.Warnings, "|"), MessageMasked) {
		t.Fatalf("a masked answer must say names are masked: %v", masked.Warnings)
	}

	visible := principal
	visible.NamesVisible = true
	shown, _ := service.Report(context.Background(), visible, "ar_aging", "", "")
	if shown.Visualizations[0].Categories[0] != "บริษัท ก จำกัด" || strings.Contains(strings.Join(shown.Warnings, "|"), MessageMasked) {
		t.Fatalf("names must show for a token that may see them: %+v", shown)
	}
}

func TestCompareIsComputedOnTheServerAndSaysWhatItDoesNotTell(t *testing.T) {
	// Today is 2026-10-01 in Bangkok, so A is unfinished. B is the same length just before it.
	a := report.Period{DateFrom: "2026-09-29", DateTo: "2026-10-01"}
	b := report.Period{DateFrom: "2026-09-26", DateTo: "2026-09-28"}
	short := report.Period{DateFrom: "2026-09-27", DateTo: "2026-09-28"}
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{
		snapshotKey(report.SalesGoodsServices, a):     fresh(salesDashboard(a, "444990.00")),
		snapshotKey(report.SalesGoodsServices, b):     fresh(salesDashboard(b, "185120.00")),
		snapshotKey(report.SalesGoodsServices, short): fresh(salesDashboard(short, "100000.00")),
	}}
	service := newService(store, snapshots)
	response, err := service.Compare(context.Background(), principal, "sales_goods_services", "total_amount", "2026-09-29", "2026-10-01", "2026-09-26", "2026-09-28")
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != "READY" || response.Delta != "259870.00" || response.Percent != "140.4" || response.A.Days != 3 || response.B.Days != 3 {
		t.Fatalf("response = %+v", response)
	}
	if !strings.Contains(strings.Join(response.Warnings, "|"), "ช่วง A ยังไม่จบ") {
		t.Fatalf("a period that includes today must be flagged as unfinished: %v", response.Warnings)
	}
	if strings.Contains(strings.Join(response.Warnings, "|"), "จำนวนวันไม่เท่ากัน") {
		t.Fatalf("equal lengths must not be flagged: %v", response.Warnings)
	}

	uneven, err := service.Compare(context.Background(), principal, "sales_goods_services", "total_amount", "2026-09-29", "2026-10-01", "2026-09-27", "2026-09-28")
	if err != nil || uneven.Status != "READY" || !strings.Contains(strings.Join(uneven.Warnings, "|"), "จำนวนวันไม่เท่ากัน") {
		t.Fatalf("periods of different length must be called out: %+v %v", uneven, err)
	}
	if uneven.A.Days != 3 || uneven.B.Days != 2 {
		t.Fatalf("day counts must be reported: %+v", uneven)
	}
	if got := compareWarnings(a, report.Period{DateFrom: "2026-09-30", DateTo: "2026-10-01"}, time.UTC, now); !strings.Contains(strings.Join(got, "|"), "ทับซ้อน") {
		t.Fatalf("overlapping periods must be called out: %v", got)
	}
}

func TestCompareHandlesMissingMetricUnsupportedReportAndZeroBase(t *testing.T) {
	a := report.Period{DateFrom: "2026-09-01", DateTo: "2026-09-30"}
	b := report.Period{DateFrom: "2026-08-01", DateTo: "2026-08-31"}
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices, report.CustomerRFM}}
	snapshots := &fakeSnapshots{exact: map[string]viewer.DashboardSnapshot{
		snapshotKey(report.SalesGoodsServices, a): fresh(salesDashboard(a, "10.00")),
		snapshotKey(report.SalesGoodsServices, b): fresh(salesDashboard(b, "0.00")),
	}}
	service := newService(store, snapshots)
	zero, _ := service.Compare(context.Background(), principal, "sales_goods_services", "total_amount", "2026-09-01", "2026-09-30", "2026-08-01", "2026-08-31")
	if zero.Status != "READY" || zero.Percent != "" || !strings.Contains(strings.Join(zero.Warnings, "|"), "เป็นศูนย์") {
		t.Fatalf("a zero base has no percentage and must say why: %+v", zero)
	}
	missing, _ := service.Compare(context.Background(), principal, "sales_goods_services", "no_such_metric", "2026-09-01", "2026-09-30", "2026-08-01", "2026-08-31")
	if missing.Status != "UNAVAILABLE" || !strings.Contains(missing.Message, "total_amount") {
		t.Fatalf("an unknown metric must list the ones that exist: %+v", missing)
	}
	unsupported, _ := service.Compare(context.Background(), principal, "customer_rfm", "customer_count", "2026-09-01", "2026-09-30", "2026-08-01", "2026-08-31")
	if unsupported.Status != "UNAVAILABLE" {
		t.Fatalf("a report that cannot be compared must say so: %+v", unsupported)
	}
	if _, err := service.Compare(context.Background(), principal, "sales_goods_services", "total_amount", "", "", "2026-08-01", "2026-08-31"); !errors.Is(err, ErrInvalidPeriod) {
		t.Fatalf("both periods must be given: %v", err)
	}
}

func TestLatestDeliveryReturnsWhatTheCardSaid(t *testing.T) {
	period := report.Period{DateFrom: "2026-09-30", DateTo: "2026-09-30"}
	delivered := Delivered{Dashboard: salesDashboard(period, "77.00"), DeliveredAt: now.Add(-3 * time.Hour), CollectedAt: finished(200), RunID: uuid.New()}
	store := &fakeStore{permitted: []report.Key{report.SalesGoodsServices}, delivered: &delivered}
	response, err := newService(store, &fakeSnapshots{}).LatestDelivery(context.Background(), principal, "sales_goods_services")
	if err != nil || response.Freshness != "DELIVERED" || response.DeliveredAt == "" || response.KPIs[0].Value != "77.00" {
		t.Fatalf("response = %+v, %v", response, err)
	}
	store.delivered = nil
	if _, err := newService(store, &fakeSnapshots{}).LatestDelivery(context.Background(), principal, "sales_goods_services"); !errors.Is(err, ErrNoData) {
		t.Fatalf("no delivery yet must answer like no data: %v", err)
	}
}

func TestIssuedTokenIsStoredOnlyAsAHashAndHasATimeLimit(t *testing.T) {
	store := &fakeStore{}
	service := newService(store, &fakeSnapshots{})
	issued, err := service.IssueToken(context.Background(), []byte("admin"), "req", tenantID, uuid.New(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(issued.Token, "abcc_") || len(issued.Token) < 40 {
		t.Fatalf("token = %q", issued.Token)
	}
	if string(store.issuedHash) != string(testHasher{}.HashToken(issued.Token)) || strings.Contains(string(store.issuedHash), "abcc_") {
		t.Fatal("only the hash of the token may be stored")
	}
	if got := issued.Info.ExpiresAt.Sub(now); got != 90*24*time.Hour {
		t.Fatalf("a token lasts 90 days, got %v", got)
	}
	store.issueErr = ErrAIChatDisabled
	if _, err := service.IssueToken(context.Background(), nil, "req", tenantID, uuid.New(), false); !errors.Is(err, ErrAIChatDisabled) {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomerReportsDefaultToTheLast180DaysNotTheMonthSoFar(t *testing.T) {
	location := locationOf(principal)
	for _, key := range []report.Key{report.CustomerRFM, report.PurchaseFrequency} {
		definition, _ := report.DefinitionFor(key)
		period, err := resolvePeriod(definition, location, now, "", "")
		if err != nil || period.DateFrom != "2026-04-05" || period.DateTo != "2026-10-01" {
			t.Errorf("%s default = %+v, %v; want 2026-04-05 to 2026-10-01 (180 days)", key, period, err)
		}
	}
	sales, _ := report.DefinitionFor(report.SalesGoodsServices)
	if period, _ := resolvePeriod(sales, location, now, "", ""); period.DateFrom != "2026-10-01" {
		t.Errorf("sales keeps the month so far: %+v", period)
	}
}
