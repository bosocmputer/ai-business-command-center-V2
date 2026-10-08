package agent

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

const (
	tokenPrefix       = "abcc_"
	tokenBytes        = 32
	maximumTokenChars = 200
)

type Config struct {
	CallsPerHour     int
	RefreshesPerHour int
	TokenTTL         time.Duration
}

func (config Config) withDefaults() Config {
	if config.CallsPerHour <= 0 {
		config.CallsPerHour = 60
	}
	if config.RefreshesPerHour <= 0 {
		config.RefreshesPerHour = 10
	}
	if config.TokenTTL <= 0 {
		config.TokenTTL = 90 * 24 * time.Hour
	}
	return config
}

type Service struct {
	store     Store
	snapshots Snapshots
	hasher    TokenHasher
	entropy   io.Reader
	alias     func(tenantID uuid.UUID, name string) string
	alerts    AlertStore
	now       func() time.Time
	config    Config
}

func NewService(store Store, snapshots Snapshots, hasher TokenHasher, entropy io.Reader, alias func(uuid.UUID, string) string, now func() time.Time, config Config) *Service {
	return &Service{store: store, snapshots: snapshots, hasher: hasher, entropy: entropy, alias: alias, now: now, config: config.withDefaults()}
}

// Authenticate turns a bearer token into the person and shop it stands for, or ErrUnauthorized. Every reason
// for refusal answers the same, so a caller cannot tell a revoked token from an expired one or a switched-off
// assistant. It also enforces the hourly call limit.
func (service *Service) Authenticate(ctx context.Context, rawToken string) (Principal, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || len(rawToken) > maximumTokenChars {
		return Principal{}, ErrUnauthorized
	}
	now := service.now().UTC()
	principal, err := service.store.Authenticate(ctx, service.hasher.HashToken(rawToken), now)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return Principal{}, ErrUnauthorized
		}
		return Principal{}, err
	}
	used, err := service.store.CallsSince(ctx, principal.TokenID, now.Add(-time.Hour))
	if err != nil {
		return Principal{}, err
	}
	if used >= service.config.CallsPerHour {
		_ = service.store.RecordCall(ctx, Call{TokenID: principal.TokenID, TenantID: principal.TenantID, RecipientID: principal.RecipientID, Tool: ToolContext, Outcome: OutcomeRateLimited}, now)
		return principal, &RateLimitedError{RetryAfter: time.Minute}
	}
	_ = service.store.TouchToken(ctx, principal.TokenID, now)
	return principal, nil
}

// ---- response shapes -------------------------------------------------------

type ReportInfo struct {
	Key             report.Key           `json:"key"`
	Label           string               `json:"label"`
	Category        string               `json:"category"`
	CategoryLabel   string               `json:"categoryLabel"`
	PeriodMode      report.ParameterKind `json:"periodMode"`
	SupportsCompare bool                 `json:"supportsCompare"`
}

type Limits struct {
	CallsPerHour   int `json:"callsPerHour"`
	CallsRemaining int `json:"callsRemaining"`
}

type ContextResponse struct {
	Shop     string       `json:"shop"`
	Timezone string       `json:"timezone"`
	Today    string       `json:"today"`
	Reports  []ReportInfo `json:"reports"`
	Limits   Limits       `json:"limits"`
	Notes    []string     `json:"notes,omitempty"`
}

type PeriodView struct {
	DateFrom string `json:"dateFrom"`
	DateTo   string `json:"dateTo"`
	Mode     string `json:"mode,omitempty"`
}

type KPIComparison struct {
	PreviousValue string `json:"previousValue"`
	Delta         string `json:"delta"`
	Percent       string `json:"percent,omitempty"`
	Direction     string `json:"direction,omitempty"`
}

type KPI struct {
	Key        string         `json:"key"`
	Label      string         `json:"label"`
	Unit       string         `json:"unit"`
	Value      string         `json:"value"`
	Comparison *KPIComparison `json:"comparison,omitempty"`
}

type Series struct {
	Key    string   `json:"key"`
	Label  string   `json:"label"`
	Values []string `json:"values"`
}

type Visualization struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	Intent     string   `json:"intent"`
	Unit       string   `json:"unit"`
	Categories []string `json:"categories"`
	Series     []Series `json:"series"`
}

// ReportResponse is READY with numbers, PREPARING while a fetch is under way, or UNAVAILABLE. The period is
// always the one actually used, and collectedAt is when SML was read, never "now".
type ReportResponse struct {
	ReportKey         report.Key      `json:"reportKey"`
	Label             string          `json:"label"`
	Status            string          `json:"status"`
	Message           string          `json:"message,omitempty"`
	Period            *PeriodView     `json:"period,omitempty"`
	ComparisonPeriod  *PeriodView     `json:"comparisonPeriod,omitempty"`
	CollectedAt       string          `json:"collectedAt,omitempty"`
	Freshness         string          `json:"freshness,omitempty"`
	DeliveredAt       string          `json:"deliveredAt,omitempty"`
	KPIs              []KPI           `json:"kpis,omitempty"`
	Visualizations    []Visualization `json:"visualizations,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
	RetryAfterSeconds int             `json:"retryAfterSeconds,omitempty"`
}

type CompareSide struct {
	Period PeriodView `json:"period"`
	Days   int        `json:"days"`
	Value  string     `json:"value"`
}

type CompareResponse struct {
	ReportKey         report.Key   `json:"reportKey"`
	Label             string       `json:"label"`
	Status            string       `json:"status"`
	Message           string       `json:"message,omitempty"`
	Metric            string       `json:"metric,omitempty"`
	MetricLabel       string       `json:"metricLabel,omitempty"`
	Unit              string       `json:"unit,omitempty"`
	A                 *CompareSide `json:"a,omitempty"`
	B                 *CompareSide `json:"b,omitempty"`
	Delta             string       `json:"delta,omitempty"`
	Percent           string       `json:"percent,omitempty"`
	Warnings          []string     `json:"warnings,omitempty"`
	CollectedAt       string       `json:"collectedAt,omitempty"`
	RetryAfterSeconds int          `json:"retryAfterSeconds,omitempty"`
}

// ---- tools -----------------------------------------------------------------

func (service *Service) Context(ctx context.Context, principal Principal) (ContextResponse, error) {
	started := service.now()
	now := started.UTC()
	location := locationOf(principal)
	permitted, err := service.store.PermittedReports(ctx, principal, now)
	if err != nil {
		return ContextResponse{}, err
	}
	reports := make([]ReportInfo, 0, len(permitted))
	for _, definition := range report.Definitions() {
		if definition.Status == report.StatusActive && slices.Contains(permitted, definition.Key) {
			reports = append(reports, ReportInfo{
				Key: definition.Key, Label: definition.LabelTH, Category: definition.Category, CategoryLabel: definition.CategoryLabelTH,
				PeriodMode: definition.ParameterKind, SupportsCompare: compareSupported(definition),
			})
		}
	}
	used, _ := service.store.CallsSince(ctx, principal.TokenID, now.Add(-time.Hour))
	response := ContextResponse{
		Shop: principal.ShopName, Timezone: location.String(), Today: now.In(location).Format(time.DateOnly), Reports: reports,
		Limits: Limits{CallsPerHour: service.config.CallsPerHour, CallsRemaining: max(0, service.config.CallsPerHour-used)},
	}
	if !principal.NamesVisible {
		response.Notes = append(response.Notes, MessageMasked)
	}
	service.record(ctx, principal, ToolContext, "", report.Period{}, OutcomeOK, started, nil)
	return response, nil
}

func (service *Service) Report(ctx context.Context, principal Principal, rawKey, dateFrom, dateTo string) (ReportResponse, error) {
	started := service.now()
	now := started.UTC()
	definition, err := service.authorize(ctx, principal, rawKey, now)
	if err != nil {
		service.record(ctx, principal, ToolGetReport, "", report.Period{}, OutcomeNoData, started, nil)
		return ReportResponse{}, err
	}
	period, err := resolvePeriod(definition, locationOf(principal), now, dateFrom, dateTo)
	if err != nil {
		service.record(ctx, principal, ToolGetReport, string(definition.Key), report.Period{}, OutcomeInvalidPeriod, started, nil)
		return ReportResponse{}, err
	}
	snapshot := service.snapshot(ctx, principal, definition, period, now)
	response := service.reportResponse(principal, definition, period, snapshot)
	service.record(ctx, principal, ToolGetReport, string(definition.Key), period, outcomeOf(snapshot.state), started, snapshot.runID())
	return response, nil
}

func (service *Service) Compare(ctx context.Context, principal Principal, rawKey, metric, aFrom, aTo, bFrom, bTo string) (CompareResponse, error) {
	started := service.now()
	now := started.UTC()
	definition, err := service.authorize(ctx, principal, rawKey, now)
	if err != nil {
		service.record(ctx, principal, ToolCompare, "", report.Period{}, OutcomeNoData, started, nil)
		return CompareResponse{}, err
	}
	location := locationOf(principal)
	periodA, errA := resolvePeriod(definition, location, now, aFrom, aTo)
	periodB, errB := resolvePeriod(definition, location, now, bFrom, bTo)
	if errA != nil || errB != nil || aFrom == "" && aTo == "" || bFrom == "" && bTo == "" {
		service.record(ctx, principal, ToolCompare, string(definition.Key), report.Period{}, OutcomeInvalidPeriod, started, nil)
		return CompareResponse{}, ErrInvalidPeriod
	}
	response := CompareResponse{ReportKey: definition.Key, Label: definition.LabelTH, Metric: metric}
	if !compareSupported(definition) {
		response.Status, response.Message = "UNAVAILABLE", "รายงานนี้เทียบสองช่วงเวลาไม่ได้"
		service.record(ctx, principal, ToolCompare, string(definition.Key), periodA, OutcomeUnavailable, started, nil)
		return response, nil
	}
	snapshotA := service.snapshot(ctx, principal, definition, periodA, now)
	snapshotB := service.snapshot(ctx, principal, definition, periodB, now)
	for _, snapshot := range []snapshotResult{snapshotA, snapshotB} {
		if snapshot.state != stateReady {
			response.Status, response.Message, response.RetryAfterSeconds = snapshot.state.status(), snapshot.message(), snapshot.retryAfter
			service.record(ctx, principal, ToolCompare, string(definition.Key), periodA, outcomeOf(snapshot.state), started, nil)
			return response, nil
		}
	}
	valueA, labelA, unit, okA := metricOf(snapshotA.snapshot.Dashboard, metric)
	valueB, _, _, okB := metricOf(snapshotB.snapshot.Dashboard, metric)
	if !okA || !okB {
		response.Status = "UNAVAILABLE"
		response.Message = "ไม่มีตัวชี้วัดนี้ในรายงาน ตัวที่เลือกได้: " + strings.Join(metricKeys(snapshotA.snapshot.Dashboard), ", ")
		service.record(ctx, principal, ToolCompare, string(definition.Key), periodA, OutcomeUnavailable, started, nil)
		return response, nil
	}
	response.Status, response.MetricLabel, response.Unit = "READY", labelA, unit
	response.A = &CompareSide{Period: PeriodView{DateFrom: periodA.DateFrom, DateTo: periodA.DateTo}, Days: inclusiveDays(periodA), Value: valueA.text}
	response.B = &CompareSide{Period: PeriodView{DateFrom: periodB.DateFrom, DateTo: periodB.DateTo}, Days: inclusiveDays(periodB), Value: valueB.text}
	delta := new(big.Rat).Sub(valueA.number, valueB.number)
	response.Delta = formatNumber(delta, unit)
	if valueB.number.Sign() != 0 {
		percent := new(big.Rat).Quo(new(big.Rat).Mul(delta, big.NewRat(100, 1)), new(big.Rat).Abs(valueB.number))
		response.Percent = percent.FloatString(1)
	} else {
		response.Warnings = append(response.Warnings, "ช่วง B เป็นศูนย์ จึงคำนวณเปอร์เซ็นต์ไม่ได้")
	}
	response.Warnings = append(response.Warnings, compareWarnings(periodA, periodB, locationOf(principal), now)...)
	if finished := snapshotA.snapshot.SourceFinishedAt; finished != nil {
		response.CollectedAt = finished.In(locationOf(principal)).Format(time.RFC3339)
	}
	service.record(ctx, principal, ToolCompare, string(definition.Key), periodA, OutcomeOK, started, snapshotA.runID())
	return response, nil
}

func (service *Service) LatestDelivery(ctx context.Context, principal Principal, rawKey string) (ReportResponse, error) {
	started := service.now()
	now := started.UTC()
	definition, err := service.authorize(ctx, principal, rawKey, now)
	if err != nil {
		service.record(ctx, principal, ToolLatestDelivery, "", report.Period{}, OutcomeNoData, started, nil)
		return ReportResponse{}, err
	}
	delivered, err := service.store.LatestDelivery(ctx, principal, definition.Key)
	if err != nil {
		if errors.Is(err, ErrNoData) {
			service.record(ctx, principal, ToolLatestDelivery, string(definition.Key), report.Period{}, OutcomeNoData, started, nil)
		}
		return ReportResponse{}, err
	}
	location := locationOf(principal)
	response := ReportResponse{ReportKey: definition.Key, Label: definition.LabelTH, Status: "READY", Freshness: "DELIVERED", DeliveredAt: delivered.DeliveredAt.In(location).Format(time.RFC3339)}
	if delivered.CollectedAt != nil {
		response.CollectedAt = delivered.CollectedAt.In(location).Format(time.RFC3339)
	}
	service.fill(&response, principal, delivered.Dashboard)
	service.record(ctx, principal, ToolLatestDelivery, string(definition.Key), delivered.Dashboard.Period, OutcomeOK, started, &delivered.RunID)
	return response, nil
}

// ---- admin -----------------------------------------------------------------

// IssueToken makes a new token for a recipient and revokes the one they had. The token is returned once.
func (service *Service) IssueToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool) (IssuedToken, error) {
	random := make([]byte, tokenBytes)
	if _, err := io.ReadFull(service.entropy, random); err != nil {
		return IssuedToken{}, errors.New("generate agent token")
	}
	token := tokenPrefix + base64.RawURLEncoding.EncodeToString(random)
	now := service.now().UTC()
	info, err := service.store.IssueToken(ctx, actorHash, requestID, tenantID, recipientID, namesVisible, service.hasher.HashToken(token), now.Add(service.config.TokenTTL), now)
	if err != nil {
		return IssuedToken{}, err
	}
	return IssuedToken{Token: token, Info: info}, nil
}

func (service *Service) RevokeToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID) error {
	return service.store.RevokeToken(ctx, actorHash, requestID, tenantID, recipientID, service.now().UTC())
}

func (service *Service) TokenInfo(ctx context.Context, tenantID, recipientID uuid.UUID) (TokenInfo, error) {
	return service.store.TokenInfo(ctx, tenantID, recipientID, service.now().UTC())
}

// ---- internals -------------------------------------------------------------

// authorize gives the report definition when the key names a report this person may read. An unknown key and a
// forbidden one return the same ErrNoData.
func (service *Service) authorize(ctx context.Context, principal Principal, rawKey string, now time.Time) (report.Definition, error) {
	definition, ok := report.DefinitionFor(report.Key(strings.TrimSpace(rawKey)))
	if !ok || definition.Status != report.StatusActive {
		return report.Definition{}, ErrNoData
	}
	permitted, err := service.store.PermittedReports(ctx, principal, now)
	if err != nil {
		return report.Definition{}, err
	}
	if !slices.Contains(permitted, definition.Key) {
		return report.Definition{}, ErrNoData
	}
	return definition, nil
}

func (service *Service) record(ctx context.Context, principal Principal, tool Tool, key string, period report.Period, outcome Outcome, started time.Time, runID *uuid.UUID) {
	call := Call{
		TokenID: principal.TokenID, TenantID: principal.TenantID, RecipientID: principal.RecipientID, Tool: tool, ReportKey: key,
		PeriodFrom: period.DateFrom, PeriodTo: period.DateTo, Outcome: outcome, Duration: max(0, service.now().Sub(started)), SnapshotRunID: runID,
	}
	// A failure to log must not fail the owner's question; the caller still gets the answer.
	_ = service.store.RecordCall(ctx, call, service.now().UTC())
}

type snapshotState int

const (
	stateReady snapshotState = iota
	statePreparing
	stateUnavailable
)

func (state snapshotState) status() string {
	switch state {
	case stateReady:
		return "READY"
	case statePreparing:
		return "PREPARING"
	default:
		return "UNAVAILABLE"
	}
}

func outcomeOf(state snapshotState) Outcome {
	switch state {
	case stateReady:
		return OutcomeOK
	case statePreparing:
		return OutcomePreparing
	default:
		return OutcomeUnavailable
	}
}

type snapshotResult struct {
	state      snapshotState
	snapshot   viewer.DashboardSnapshot
	stale      bool
	retryAfter int
	// budgetUsed says the shop has had many fetches started this hour, so none more is started.
	budgetUsed bool
}

func (result snapshotResult) message() string {
	switch result.state {
	case statePreparing:
		return MessagePreparing
	case stateUnavailable:
		if result.budgetUsed {
			return MessageBudgetUsed
		}
		return MessageUnavailable
	default:
		return ""
	}
}

func (result snapshotResult) runID() *uuid.UUID {
	if result.state != stateReady || result.snapshot.RunID == uuid.Nil {
		return nil
	}
	id := result.snapshot.RunID
	return &id
}

// snapshot finds the stored numbers for a period and, when they are missing or old, asks the existing
// revalidation path to fetch them in the background, within an hourly budget per shop.
func (service *Service) snapshot(ctx context.Context, principal Principal, definition report.Definition, period report.Period, now time.Time) snapshotResult {
	cached, err := service.snapshots.GetExactSnapshotForPeriod(ctx, principal.TenantID, definition.Key, period, now)
	have := err == nil
	if have && cached.FreshnessStatus == viewer.FreshnessFresh {
		return snapshotResult{state: stateReady, snapshot: cached}
	}
	if err != nil && !errors.Is(err, report.ErrRunNotFound) {
		return snapshotResult{state: stateUnavailable}
	}
	fallback := func(retryAfter int) snapshotResult {
		if have && cached.FreshnessStatus != viewer.FreshnessExpired {
			return snapshotResult{state: stateReady, snapshot: cached, stale: true, retryAfter: retryAfter}
		}
		return snapshotResult{state: stateUnavailable, retryAfter: retryAfter}
	}
	used, err := service.store.PreparingSince(ctx, principal.TenantID, now.Add(-time.Hour))
	if err != nil || used >= service.config.RefreshesPerHour {
		result := fallback(300)
		result.budgetUsed = true
		return result
	}
	revalidation, err := service.snapshots.RevalidateSnapshot(ctx, principal.TenantID, definition.Key, period, now)
	if err != nil {
		return fallback(60)
	}
	retry := max(revalidation.RetryAfter, 60)
	switch revalidation.Disposition {
	case viewer.RevalidationFreshCache:
		if revalidation.Snapshot != nil {
			return snapshotResult{state: stateReady, snapshot: *revalidation.Snapshot}
		}
	case viewer.RevalidationMissingRefreshing, viewer.RevalidationStaleRefreshing, viewer.RevalidationJoined:
		if revalidation.Snapshot != nil && revalidation.Snapshot.FreshnessStatus != viewer.FreshnessExpired {
			return snapshotResult{state: stateReady, snapshot: *revalidation.Snapshot, stale: true, retryAfter: retry}
		}
		return snapshotResult{state: statePreparing, retryAfter: retry}
	}
	return fallback(retry)
}

func (service *Service) reportResponse(principal Principal, definition report.Definition, period report.Period, snapshot snapshotResult) ReportResponse {
	response := ReportResponse{ReportKey: definition.Key, Label: definition.LabelTH, Status: snapshot.state.status(), Message: snapshot.message(), RetryAfterSeconds: snapshot.retryAfter}
	if snapshot.state != stateReady {
		response.Period = &PeriodView{DateFrom: period.DateFrom, DateTo: period.DateTo, Mode: string(definition.ParameterKind)}
		return response
	}
	response.Freshness = "FRESH"
	if snapshot.stale {
		response.Freshness = "STALE"
		response.Warnings = append(response.Warnings, MessageStale)
	}
	if finished := snapshot.snapshot.SourceFinishedAt; finished != nil {
		response.CollectedAt = finished.In(locationOf(principal)).Format(time.RFC3339)
	}
	service.fill(&response, principal, snapshot.snapshot.Dashboard)
	if response.Period != nil {
		response.Period.Mode = string(definition.ParameterKind)
	}
	return response
}

// fill copies the dashboard's figures into the response, replacing customer and supplier names with aliases
// when the token may not see names.
func (service *Service) fill(response *ReportResponse, principal Principal, dashboard report.Dashboard) {
	response.Period = &PeriodView{DateFrom: dashboard.Period.DateFrom, DateTo: dashboard.Period.DateTo}
	if dashboard.ComparisonPeriod.DateFrom != "" {
		response.ComparisonPeriod = &PeriodView{DateFrom: dashboard.ComparisonPeriod.DateFrom, DateTo: dashboard.ComparisonPeriod.DateTo}
	}
	for _, metric := range dashboard.KPIs {
		kpi := KPI{Key: metric.Key, Label: metric.Label, Unit: string(metric.Unit), Value: metric.Value}
		if metric.Comparison.Availability == report.ComparisonAvailable {
			kpi.Comparison = &KPIComparison{PreviousValue: metric.Comparison.PreviousValue, Delta: metric.Comparison.Delta, Percent: metric.Comparison.Percent, Direction: string(metric.Comparison.Direction)}
		}
		response.KPIs = append(response.KPIs, kpi)
	}
	for _, visualization := range dashboard.Visualizations {
		if report.IsAgentVisualization(visualization.Key) {
			continue // made for the assistant's own tools, not for it to read back
		}
		view := Visualization{Key: visualization.Key, Title: visualization.Title, Intent: string(visualization.Intent), Unit: string(visualization.Unit), Categories: slices.Clone(visualization.Categories)}
		if _, named := report.PersonNameVisualizations[visualization.Key]; named && !principal.NamesVisible {
			for index, name := range view.Categories {
				view.Categories[index] = service.alias(principal.TenantID, name)
			}
		}
		for _, series := range visualization.Series {
			view.Series = append(view.Series, Series{Key: series.Key, Label: series.Label, Values: slices.Clone(series.Values)})
		}
		response.Visualizations = append(response.Visualizations, view)
	}
	response.Warnings = append(response.Warnings, dashboard.Quality.Warnings...)
	if !principal.NamesVisible && hasNamedVisualization(dashboard) {
		response.Warnings = append(response.Warnings, MessageMasked)
	}
}

func hasNamedVisualization(dashboard report.Dashboard) bool {
	for _, visualization := range dashboard.Visualizations {
		if _, named := report.PersonNameVisualizations[visualization.Key]; named {
			return true
		}
	}
	return false
}

// Alias gives a name a stable code: the same name in the same shop always gets the same code, and the code cannot
// be turned back into the name without the server's key.
func Alias(hasher TokenHasher) func(uuid.UUID, string) string {
	return func(tenantID uuid.UUID, name string) string {
		sum := hasher.HashToken("agent-alias|" + tenantID.String() + "|" + strings.TrimSpace(name))
		return "ลูกค้า-" + strings.ToUpper(hex.EncodeToString(sum[:2]))
	}
}

func locationOf(principal Principal) *time.Location {
	location, err := time.LoadLocation(principal.Timezone)
	if err != nil {
		return time.FixedZone("Asia/Bangkok", 7*60*60)
	}
	return location
}

// defaultLookbackDays are the reports whose useful default is a stretch of recent days rather than the month so far.
var defaultLookbackDays = map[report.Key]int{report.CustomerRFM: 180, report.PurchaseFrequency: 180}

// resolvePeriod gives the period a report is read for. Date-range reports default to the month so far and as-of
// reports to today. A period is never in the future and never longer than 366 days.
func resolvePeriod(definition report.Definition, location *time.Location, now time.Time, dateFrom, dateTo string) (report.Period, error) {
	dateFrom, dateTo = strings.TrimSpace(dateFrom), strings.TrimSpace(dateTo)
	today := now.In(location).Format(time.DateOnly)
	switch definition.ParameterKind {
	case report.CurrentOnly:
		return report.ResolvePeriod(report.AsOfRun, location, now, nil, nil)
	case report.AsOfDate:
		if dateTo == "" {
			dateTo = dateFrom
		}
		if dateTo == "" {
			dateTo = today
		}
		dateFrom = dateTo
	default:
		if dateFrom == "" && dateTo == "" {
			dateFrom, dateTo = today[:8]+"01", today
			// Customer behaviour needs months, not the days of a new month: segments and buying rhythm are read over 180 days.
			if days, ok := defaultLookbackDays[definition.Key]; ok {
				if end, err := time.Parse(time.DateOnly, today); err == nil {
					dateFrom = end.AddDate(0, 0, -(days - 1)).Format(time.DateOnly)
				}
			}
		}
	}
	if dateFrom == "" || dateTo == "" {
		return report.Period{}, ErrInvalidPeriod
	}
	period, err := report.ResolvePeriod(report.Custom, location, now, &dateFrom, &dateTo)
	if err != nil || period.DateTo > today {
		return report.Period{}, ErrInvalidPeriod
	}
	return period, nil
}

func inclusiveDays(period report.Period) int {
	from, errFrom := time.Parse(time.DateOnly, period.DateFrom)
	to, errTo := time.Parse(time.DateOnly, period.DateTo)
	if errFrom != nil || errTo != nil {
		return 0
	}
	return int(to.Sub(from).Hours()/24) + 1
}

func compareSupported(definition report.Definition) bool {
	switch definition.ParameterKind {
	case report.DateRange:
		return report.ComparisonSupported(definition.Key, report.Period{Preset: report.Custom})
	case report.AsOfDate:
		return report.ComparisonSupported(definition.Key, report.Period{Preset: report.Custom})
	default:
		return false
	}
}

type metricValue struct {
	number *big.Rat
	text   string
}

func metricOf(dashboard report.Dashboard, key string) (metricValue, string, string, bool) {
	for _, metric := range dashboard.KPIs {
		if metric.Key == key {
			number, ok := new(big.Rat).SetString(metric.Value)
			if !ok {
				return metricValue{}, "", "", false
			}
			return metricValue{number: number, text: metric.Value}, metric.Label, string(metric.Unit), true
		}
	}
	return metricValue{}, "", "", false
}

func metricKeys(dashboard report.Dashboard) []string {
	keys := make([]string, 0, len(dashboard.KPIs))
	for _, metric := range dashboard.KPIs {
		keys = append(keys, metric.Key)
	}
	return keys
}

func formatNumber(value *big.Rat, unit string) string {
	if unit == string(report.UnitCount) {
		return value.FloatString(0)
	}
	return value.FloatString(2)
}

// compareWarnings says what a comparison does not tell: different lengths, a period that has not finished, and
// periods that overlap.
func compareWarnings(a, b report.Period, location *time.Location, now time.Time) []string {
	var warnings []string
	daysA, daysB := inclusiveDays(a), inclusiveDays(b)
	if daysA != daysB {
		warnings = append(warnings, fmt.Sprintf("ช่วง A มี %d วัน ช่วง B มี %d วัน จำนวนวันไม่เท่ากัน ผลต่างจึงอาจไม่ยุติธรรม", daysA, daysB))
	}
	today := now.In(location).Format(time.DateOnly)
	if a.DateTo >= today {
		warnings = append(warnings, "ช่วง A ยังไม่จบ ยอดจะเพิ่มขึ้นอีกจนสิ้นช่วง")
	}
	if b.DateTo >= today {
		warnings = append(warnings, "ช่วง B ยังไม่จบ ยอดจะเพิ่มขึ้นอีกจนสิ้นช่วง")
	}
	if a.DateFrom <= b.DateTo && b.DateFrom <= a.DateTo {
		warnings = append(warnings, "สองช่วงทับซ้อนกัน")
	}
	return warnings
}
