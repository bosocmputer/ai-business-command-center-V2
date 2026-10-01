package executionmode

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

type setCall struct {
	key    report.Key
	mode   report.ExecutionMode
	source report.ModeSource
	reason string
}

type fakeStore struct {
	records   []report.ExecutionModeRecord
	busy      bool
	sets      []setCall
	measured  map[report.Key]int
	setErr    error
	listError error
}

func (f *fakeStore) List(context.Context, uuid.UUID) ([]report.ExecutionModeRecord, error) {
	return f.records, f.listError
}
func (f *fakeStore) Set(_ context.Context, _ []byte, _ string, _ uuid.UUID, key report.Key, mode report.ExecutionMode, source report.ModeSource, reason string, now time.Time) (report.ExecutionModeRecord, error) {
	if f.setErr != nil {
		return report.ExecutionModeRecord{}, f.setErr
	}
	f.sets = append(f.sets, setCall{key, mode, source, reason})
	return report.ExecutionModeRecord{ReportKey: key, Mode: mode, Source: source, Reason: reason, ChangedAt: &now}, nil
}
func (f *fakeStore) RecordMeasurement(_ context.Context, _ uuid.UUID, key report.Key, units int, _ time.Duration, _ time.Time) error {
	if f.measured == nil {
		f.measured = map[report.Key]int{}
	}
	f.measured[key] = units
	return nil
}
func (f *fakeStore) TenantBusy(context.Context, uuid.UUID) (bool, error) { return f.busy, nil }

type fakeConnections struct{ err error }

func (f fakeConnections) Open(context.Context, uuid.UUID) (sml.Connection, error) {
	return sml.Connection{}, f.err
}

type queryFunc func(context.Context, sml.Connection, string) ([]map[string]string, error)

func (q queryFunc) Query(ctx context.Context, c sml.Connection, sql string) ([]map[string]string, error) {
	return q(ctx, c, sql)
}

func manifest(units int) []map[string]string {
	rows := make([]map[string]string, units)
	for index := range rows {
		rows[index] = map[string]string{"unit_key": strconv.Itoa(index)}
	}
	return rows
}

var now = func() time.Time { return time.Date(2026, 9, 30, 13, 0, 0, 0, time.UTC) }

func TestListShowsEveryReportAndMarksWhichCanBeChunked(t *testing.T) {
	changed := now()
	rows := 8080
	store := &fakeStore{records: []report.ExecutionModeRecord{{ReportKey: report.StockBalance, Mode: report.ModeChunked, Source: report.ModeSourceAutoSwitched, Reason: "SML_RESULT_INVALID/XML_MALFORMED", LastRows: &rows, ChangedAt: &changed}}}
	items, err := NewService(store, nil, nil, now).List(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != len(report.Keys()) {
		t.Fatalf("%d items, want one per report (%d)", len(items), len(report.Keys()))
	}
	byKey := map[report.Key]Item{}
	for _, item := range items {
		byKey[item.ReportKey] = item
	}
	stock := byKey[report.StockBalance]
	if stock.Mode != report.ModeChunked || stock.Source != report.ModeSourceAutoSwitched || !stock.Chunkable || stock.LastRows == nil || *stock.LastRows != 8080 {
		t.Fatalf("stock = %+v", stock)
	}
	if stock.ChunkThreshold == nil || *stock.ChunkThreshold != report.ChunkUnitThreshold(report.StockBalance) {
		t.Fatalf("a chunkable report must show its threshold: %+v", stock)
	}
	sales := byKey[report.SalesGoodsServices]
	if sales.ChunkThreshold != nil {
		t.Fatalf("a report that cannot be chunked must have no threshold: %+v", sales)
	}
	if sales.Mode != report.ModeDirect || sales.Source != report.ModeSourceDefault || sales.Chunkable {
		t.Fatalf("a report with no stored row must read DIRECT/DEFAULT and not chunkable: %+v", sales)
	}
	if sales.Label == "" {
		t.Fatal("rows need a Thai label for the admin page")
	}
}

func TestSetRefusesModesTheWorkerCannotHonour(t *testing.T) {
	store := &fakeStore{}
	service := NewService(store, nil, nil, now)
	for name, call := range map[string]func() error{
		"chunked for a report with no chunk plan": func() error {
			_, err := service.Set(context.Background(), nil, "r", uuid.New(), report.SalesGoodsServices, report.ModeChunked, "")
			return err
		},
		"unknown report": func() error {
			_, err := service.Set(context.Background(), nil, "r", uuid.New(), report.Key("nope"), report.ModeDirect, "")
			return err
		},
		"invalid mode": func() error {
			_, err := service.Set(context.Background(), nil, "r", uuid.New(), report.StockBalance, report.ExecutionMode("FAST"), "")
			return err
		},
		"reason too long": func() error {
			_, err := service.Set(context.Background(), nil, "r", uuid.New(), report.StockBalance, report.ModeChunked, strings.Repeat("ก", 201))
			return err
		},
	} {
		if err := call(); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: error = %v, want ErrUnsupported", name, err)
		}
	}
	if len(store.sets) != 0 {
		t.Fatalf("store was written despite validation errors: %v", store.sets)
	}

	item, err := service.Set(context.Background(), []byte("admin"), "req", uuid.New(), report.StockBalance, report.ModeChunked, "ร้านใหญ่")
	if err != nil || item.Mode != report.ModeChunked || item.Source != report.ModeSourceManual || !item.Chunkable {
		t.Fatalf("set = %+v, %v", item, err)
	}
	if len(store.sets) != 1 || store.sets[0].source != report.ModeSourceManual {
		t.Fatalf("store calls = %v", store.sets)
	}
}

func unitsByReport(stock, ar int) queryFunc {
	return func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
		if strings.Contains(sql, "from ic_inventory") {
			return manifest(stock), nil
		}
		return manifest(ar), nil
	}
}

func TestMeasureAppliesPerReportThresholdsAndNeverOverrulesAnAdmin(t *testing.T) {
	stockLine, arLine := report.ChunkUnitThreshold(report.StockBalance), report.ChunkUnitThreshold(report.ARCustomerMovement)
	store := &fakeStore{records: []report.ExecutionModeRecord{
		{ReportKey: report.ARCustomerMovement, Mode: report.ModeDirect, Source: report.ModeSourceManual},
	}}
	results, err := NewService(store, fakeConnections{}, unitsByReport(stockLine, arLine+1000), now).Measure(context.Background(), []byte("admin"), "req", uuid.New())
	if err != nil || len(results) != 2 {
		t.Fatalf("results=%+v err=%v; want the two chunkable reports", results, err)
	}
	byKey := map[report.Key]Measurement{}
	for _, result := range results {
		byKey[result.ReportKey] = result
	}
	stock := byKey[report.StockBalance]
	if stock.Units == nil || *stock.Units != stockLine || stock.Threshold != stockLine || stock.RecommendedMode != report.ModeChunked || !stock.Applied {
		t.Fatalf("stock at its own line must be recommended and applied: %+v", stock)
	}
	ar := byKey[report.ARCustomerMovement]
	if ar.Threshold != arLine || ar.RecommendedMode != report.ModeChunked || ar.Applied {
		t.Fatalf("an admin's manual DIRECT must only receive a recommendation: %+v", ar)
	}
	if len(store.sets) != 1 || store.sets[0].key != report.StockBalance || store.sets[0].source != report.ModeSourceMeasured || store.sets[0].mode != report.ModeChunked {
		t.Fatalf("store sets = %+v", store.sets)
	}
	if store.measured[report.StockBalance] != stockLine || store.measured[report.ARCustomerMovement] != arLine+1000 {
		t.Fatalf("recorded sizes = %v", store.measured)
	}
}

func TestEachReportHasItsOwnLine(t *testing.T) {
	stockLine, arLine := report.ChunkUnitThreshold(report.StockBalance), report.ChunkUnitThreshold(report.ARCustomerMovement)
	if stockLine == arLine {
		t.Fatalf("both reports share the line %d; the point of per-report thresholds is that they differ", stockLine)
	}
	// Just under each line stays DIRECT, and a size that is over the stock line
	// but under the receivable line is DIRECT only for receivables.
	results, err := NewService(&fakeStore{}, fakeConnections{}, unitsByReport(stockLine-1, arLine-1), now).Measure(context.Background(), nil, "req", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.RecommendedMode != report.ModeDirect {
			t.Fatalf("%s recommended %s just under its line", result.ReportKey, result.RecommendedMode)
		}
	}
	results, _ = NewService(&fakeStore{}, fakeConnections{}, unitsByReport(stockLine+1, stockLine+1), now).Measure(context.Background(), nil, "req", uuid.New())
	for _, result := range results {
		want := report.ModeChunked
		if result.ReportKey == report.ARCustomerMovement && stockLine+1 < arLine {
			want = report.ModeDirect
		}
		if result.RecommendedMode != want {
			t.Fatalf("%s at %d units recommended %s, want %s", result.ReportKey, stockLine+1, result.RecommendedMode, want)
		}
	}
}

func TestAMeasurementReplacesAnEarlierMeasurementButNotAWorkerSwitch(t *testing.T) {
	store := &fakeStore{records: []report.ExecutionModeRecord{
		{ReportKey: report.StockBalance, Mode: report.ModeChunked, Source: report.ModeSourceMeasured},
		{ReportKey: report.ARCustomerMovement, Mode: report.ModeChunked, Source: report.ModeSourceAutoSwitched},
	}}
	results, err := NewService(store, fakeConnections{}, unitsByReport(100, 100), now).Measure(context.Background(), nil, "req", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		wantApplied := result.ReportKey == report.StockBalance
		if result.Applied != wantApplied {
			t.Fatalf("%s applied=%v, want %v: only an earlier measurement may be replaced", result.ReportKey, result.Applied, wantApplied)
		}
	}
	if len(store.sets) != 1 || store.sets[0].key != report.StockBalance || store.sets[0].mode != report.ModeDirect {
		t.Fatalf("store sets = %+v; the shrunken shop must go back to DIRECT and the auto-switched report must stay", store.sets)
	}
}

func TestMeasureRefusesWhileAReportRunsAndReportsPerReportFailures(t *testing.T) {
	busy := &fakeStore{busy: true}
	if _, err := NewService(busy, fakeConnections{}, nil, now).Measure(context.Background(), nil, "r", uuid.New()); !errors.Is(err, ErrTenantBusy) {
		t.Fatalf("busy error = %v", err)
	}

	notConfigured := NewService(&fakeStore{}, fakeConnections{err: sml.ErrConnectionNotConfigured}, nil, now)
	if _, err := notConfigured.Measure(context.Background(), nil, "r", uuid.New()); !errors.Is(err, sml.ErrConnectionNotConfigured) {
		t.Fatalf("connection error = %v", err)
	}

	store := &fakeStore{}
	calls := 0
	client := queryFunc(func(context.Context, sml.Connection, string) ([]map[string]string, error) {
		calls++
		if calls == 1 {
			return nil, &sml.SafeError{Code: "SML_TIMEOUT", Retryable: true}
		}
		return manifest(10), nil
	})
	results, err := NewService(store, fakeConnections{}, client, now).Measure(context.Background(), nil, "r", uuid.New())
	if err != nil || len(results) != 2 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	if results[0].SafeErrorCode != "SML_TIMEOUT" || results[0].Units != nil || results[0].Applied {
		t.Fatalf("a failed measurement must carry its safe code and change nothing: %+v", results[0])
	}
	if results[1].Units == nil || *results[1].Units != 10 {
		t.Fatalf("one failure must not stop the other report: %+v", results[1])
	}
	if len(store.sets) != 1 {
		t.Fatalf("only the measured report may be written: %v", store.sets)
	}
}
