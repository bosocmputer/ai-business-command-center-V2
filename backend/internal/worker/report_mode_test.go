package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/sml"
	"github.com/google/uuid"
)

type fakeModeStore struct {
	mode           report.ExecutionMode
	getErr         error
	switchReasons  []string
	switchErr      error
	directSuccess  []int
	measurements   []int
	timeoutStreak  int
	timeoutCalls   int
	timeoutErr     error
	lastDurationMS int64
}

func (store *fakeModeStore) Get(_ context.Context, _ uuid.UUID, key report.Key) (report.ExecutionModeRecord, error) {
	if store.getErr != nil {
		return report.ExecutionModeRecord{}, store.getErr
	}
	return report.ExecutionModeRecord{ReportKey: key, Mode: store.mode}, nil
}
func (store *fakeModeStore) SwitchToChunked(_ context.Context, _ uuid.UUID, _ report.Key, reason string, _ time.Time) (bool, error) {
	if store.switchErr != nil {
		return false, store.switchErr
	}
	store.switchReasons = append(store.switchReasons, reason)
	store.mode = report.ModeChunked
	return true, nil
}
func (store *fakeModeStore) RecordDirectSuccess(_ context.Context, _ uuid.UUID, _ report.Key, rows int, duration time.Duration, _ time.Time) error {
	store.directSuccess = append(store.directSuccess, rows)
	store.lastDurationMS = duration.Milliseconds()
	return nil
}
func (store *fakeModeStore) RecordMeasurement(_ context.Context, _ uuid.UUID, _ report.Key, units int, _ time.Duration, _ time.Time) error {
	store.measurements = append(store.measurements, units)
	return nil
}
func (store *fakeModeStore) RecordDirectTimeout(context.Context, uuid.UUID, report.Key, time.Time) (int, error) {
	store.timeoutCalls++
	return store.timeoutStreak, store.timeoutErr
}

var modeTestNow = time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)

func modeRun(tenantID uuid.UUID, key report.Key) report.Run {
	return report.Run{
		ID: uuid.New(), TenantID: tenantID, ReportKey: key,
		Source: report.SourceBackground, ResultKind: report.ResultSummary,
		Status: report.StatusClaimed, Attempt: 1,
		Period: report.Period{Preset: report.AsOfRun, DateFrom: "2026-09-30", DateTo: "2026-09-30"},
	}
}

func stockRows() []map[string]string {
	return []map[string]string{{
		"ic_code": "001", "ic_name": "สินค้า", "balance_amount": "100", "amount_in": "10", "amount_out": "5",
		"_metric_item_count": "2", "_metric_balance_amount": "100", "_metric_amount_in": "10", "_metric_amount_out": "5", "_metric_row_count": "2",
	}}
}

func xmlMalformed() error {
	return &sml.SafeError{Code: "SML_RESULT_INVALID", Phase: sml.ResponseStarted, ProtocolEvidence: &sml.ProtocolEvidence{RequestRef: "NXR-ABCDEFGHIJKLMNOP", RequestCount: 1, ResultValidationCode: sml.ResultValidationXMLMalformed}}
}

func newModeWorker(store *fakeRunStore, modes *fakeModeStore, client queryClientFunc, master bool) *ReportWorker {
	return NewReportWorker(store, connectionProviderFunc(func(context.Context, uuid.UUID) (sml.Connection, error) {
		return sml.Connection{}, nil
	}), client, "worker-a", func() time.Time { return modeTestNow }).
		ConfigureHeavyChunks(master, false, nil).
		ConfigureExecutionModes(modes)
}

func TestModeTableDecidesBetweenDirectAndChunked(t *testing.T) {
	for _, test := range []struct {
		name        string
		mode        report.ExecutionMode
		master      bool
		wantChunked bool
	}{
		{"chunked mode uses chunks", report.ModeChunked, true, true},
		{"direct mode does not", report.ModeDirect, true, false},
		{"master switch off overrides chunked", report.ModeChunked, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tenantID := uuid.New()
			store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
			modes := &fakeModeStore{mode: test.mode}
			worker := newModeWorker(store, modes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
				if strings.Contains(sql, "select code as unit_key") {
					return []map[string]string{{"unit_key": "001"}, {"unit_key": "002"}}, nil
				}
				return stockRows(), nil
			}, test.master)
			worker.denySummaryDirect(tenantID, report.StockBalance) // these cases are about the chunked path itself
			if err := worker.ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := len(store.chunkManifests) > 0; got != test.wantChunked {
				t.Fatalf("chunked = %v, want %v", got, test.wantChunked)
			}
			if store.completed == nil {
				t.Fatalf("run did not complete: fail=%q", store.failedCode)
			}
		})
	}
}

func TestModeReadFailureFallsBackToDirect(t *testing.T) {
	tenantID := uuid.New()
	store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	modes := &fakeModeStore{mode: report.ModeChunked, getErr: errors.New("db down")}
	worker := newModeWorker(store, modes, func(context.Context, sml.Connection, string) ([]map[string]string, error) { return stockRows(), nil }, true)
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.chunkManifests) != 0 || store.completed == nil {
		t.Fatalf("manifests=%d completed=%v; a mode lookup failure must behave like the old default", len(store.chunkManifests), store.completed)
	}
}

func TestSizeSignalSwitchesToChunkedAndRequeuesTheSameRun(t *testing.T) {
	tenantID := uuid.New()
	store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	modes := &fakeModeStore{mode: report.ModeDirect}
	worker := newModeWorker(store, modes, func(context.Context, sml.Connection, string) ([]map[string]string, error) { return nil, xmlMalformed() }, true)

	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(modes.switchReasons) != 1 || modes.switchReasons[0] != "SML_RESULT_INVALID/XML_MALFORMED" {
		t.Fatalf("switch reasons = %v", modes.switchReasons)
	}
	if store.retriedCode != "SML_RESULT_INVALID" || store.failCalls != 0 || store.remoteFailCalls != 0 {
		t.Fatalf("retry=%q fail=%d remoteFail=%d; the run must be requeued, not failed", store.retriedCode, store.failCalls, store.remoteFailCalls)
	}
	if worker.modeFor(context.Background(), tenantID, report.StockBalance) != report.ModeChunked {
		t.Fatal("the worker must use the new mode straight away, not after the cache expires")
	}
}

func TestSizeSignalInChunkedModeIsNotSwitchedOrRetried(t *testing.T) {
	tenantID := uuid.New()
	store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	modes := &fakeModeStore{mode: report.ModeChunked}
	worker := newModeWorker(store, modes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
		if strings.Contains(sql, "select code as unit_key") {
			return []map[string]string{{"unit_key": "001"}}, nil
		}
		return nil, xmlMalformed()
	}, true)
	worker.denySummaryDirect(tenantID, report.StockBalance)
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(modes.switchReasons) != 0 || store.retriedCode != "" || store.failCalls != 1 || store.failedCode != "SML_RESULT_INVALID" {
		t.Fatalf("switches=%v retry=%q fail=%d/%q; a failure after chunking must be reported, never looped", modes.switchReasons, store.retriedCode, store.failCalls, store.failedCode)
	}
}

func TestOnlyChunkableReportsAndSizeSignalsSwitch(t *testing.T) {
	for _, test := range []struct {
		name string
		key  report.Key
		err  error
	}{
		{"report that cannot be chunked", report.SalesGoodsServices, xmlMalformed()},
		{"shop unreachable", report.StockBalance, &sml.SafeError{Code: "SML_UNREACHABLE", Phase: sml.RequestSentResultUnknown}},
		{"credentials rejected", report.StockBalance, &sml.SafeError{Code: "SML_AUTH_FAILED", Phase: sml.ResponseStarted}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tenantID := uuid.New()
			store := &fakeRunStore{run: modeRun(tenantID, test.key)}
			modes := &fakeModeStore{mode: report.ModeDirect}
			worker := newModeWorker(store, modes, func(context.Context, sml.Connection, string) ([]map[string]string, error) { return nil, test.err }, true)
			if err := worker.ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(modes.switchReasons) != 0 {
				t.Fatalf("switched on %s: %v", test.name, modes.switchReasons)
			}
		})
	}
}

func TestTimeoutSwitchesOnlyAfterTwoInARowWhileTheShopStillAnswers(t *testing.T) {
	timeout := &sml.SafeError{Code: "SML_TIMEOUT", Retryable: true, Phase: sml.RequestSentResultUnknown}
	for _, test := range []struct {
		name        string
		streak      int
		probeOK     bool
		wantSwitch  bool
		wantTimeout int
	}{
		{"first timeout only counts", 1, true, false, 1},
		{"second timeout with a live shop switches", 2, true, true, 1},
		{"second timeout with a dead shop does not", 2, false, false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			tenantID := uuid.New()
			store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
			modes := &fakeModeStore{mode: report.ModeDirect, timeoutStreak: test.streak}
			worker := newModeWorker(store, modes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
				if strings.Contains(sql, "select 1 as ok") {
					if test.probeOK {
						return []map[string]string{{"ok": "1"}}, nil
					}
					return nil, timeout
				}
				return nil, timeout
			}, true)
			if err := worker.ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			if (len(modes.switchReasons) == 1) != test.wantSwitch || modes.timeoutCalls != test.wantTimeout {
				t.Fatalf("switches=%v timeoutCalls=%d", modes.switchReasons, modes.timeoutCalls)
			}
			// The timed-out run itself still fails through the normal path with
			// its tenant cooldown; only later runs benefit from the new mode.
			if store.remoteFailCalls != 1 || store.retriedCode != "" {
				t.Fatalf("remoteFail=%d retry=%q", store.remoteFailCalls, store.retriedCode)
			}
			if test.wantSwitch && !strings.HasPrefix(modes.switchReasons[0], "SML_TIMEOUT") {
				t.Fatalf("reason = %q", modes.switchReasons[0])
			}
		})
	}
}

func TestSuccessfulRunsRecordObservedSize(t *testing.T) {
	tenantID := uuid.New()

	directStore := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	directModes := &fakeModeStore{mode: report.ModeDirect}
	direct := newModeWorker(directStore, directModes, func(context.Context, sml.Connection, string) ([]map[string]string, error) {
		return append(stockRows(), stockRows()...), nil
	}, true)
	if err := direct.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(directModes.directSuccess) != 1 || directModes.directSuccess[0] != 2 || len(directModes.measurements) != 0 {
		t.Fatalf("direct success rows=%v measurements=%v", directModes.directSuccess, directModes.measurements)
	}

	chunkStore := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	chunkModes := &fakeModeStore{mode: report.ModeChunked}
	chunked := newModeWorker(chunkStore, chunkModes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
		if strings.Contains(sql, "select code as unit_key") {
			return []map[string]string{{"unit_key": "001"}, {"unit_key": "002"}, {"unit_key": "003"}}, nil
		}
		return stockRows(), nil
	}, true)
	chunked.denySummaryDirect(tenantID, report.StockBalance)
	if err := chunked.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(chunkModes.measurements) != 1 || chunkModes.measurements[0] != 3 || len(chunkModes.directSuccess) != 0 {
		t.Fatalf("chunked measurements=%v directSuccess=%v; units must be the manifest size", chunkModes.measurements, chunkModes.directSuccess)
	}
}

func TestLegacyAllowlistStillWorksWithoutAModeStore(t *testing.T) {
	tenantID := uuid.New()
	run := modeRun(tenantID, report.StockBalance)
	definition, _ := report.DefinitionFor(report.StockBalance)
	worker := NewReportWorker(nil, nil, nil, "worker-a", time.Now).ConfigureHeavyChunks(true, false, []string{tenantID.String() + "/stock_balance"})
	if !worker.chunkDecision(context.Background(), run, definition, report.ResultSummary) {
		t.Fatal("allowlisted report must still be chunked when no mode store is configured")
	}
	other := modeRun(uuid.New(), report.StockBalance)
	if worker.chunkDecision(context.Background(), other, definition, report.ResultSummary) {
		t.Fatal("a tenant outside the allowlist must not be chunked")
	}
}

// A report fetched in chunks for its detail rows still tries its bounded summary as one query first.
func TestChunkedModeSummaryTriesOneQueryFirstAndRecordsNoMeasurement(t *testing.T) {
	tenantID := uuid.New()
	store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	modes := &fakeModeStore{mode: report.ModeChunked}
	worker := newModeWorker(store, modes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
		if strings.Contains(sql, "select code as unit_key") {
			t.Error("the manifest must not be read when the summary runs as one query")
		}
		return stockRows(), nil
	}, true)
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.completed == nil || len(store.chunkManifests) != 0 {
		t.Fatalf("completed=%v manifests=%d fail=%q", store.completed != nil, len(store.chunkManifests), store.failedCode)
	}
	if len(modes.measurements) != 0 || len(modes.directSuccess) != 0 || len(modes.switchReasons) != 0 {
		t.Fatalf("a one-query summary says nothing about the detail size: measurements=%v directSuccess=%v switches=%v", modes.measurements, modes.directSuccess, modes.switchReasons)
	}
}

func TestChunkedModeSummaryFallsBackToChunksAfterASizeSignalAndRemembersIt(t *testing.T) {
	tenantID := uuid.New()
	store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	modes := &fakeModeStore{mode: report.ModeChunked}
	worker := newModeWorker(store, modes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
		if strings.Contains(sql, "select code as unit_key") {
			return []map[string]string{{"unit_key": "001"}}, nil
		}
		return nil, xmlMalformed()
	}, true)
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.retriedCode != "SML_RESULT_INVALID" || store.failCalls != 0 || len(modes.switchReasons) != 0 {
		t.Fatalf("retry=%q fail=%d switches=%v; the run must be requeued without touching the mode", store.retriedCode, store.failCalls, modes.switchReasons)
	}
	if worker.summaryDirectAllowed(tenantID, report.StockBalance) {
		t.Fatal("the shop must not be asked for a one-query summary again straight away")
	}
	if !worker.summaryDirectAllowed(uuid.New(), report.StockBalance) {
		t.Fatal("another shop must not be affected")
	}
	// Once denied, the next run goes through the chunks.
	store.retriedCode = ""
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.chunkManifests) == 0 {
		t.Fatal("after a denial the summary must be fetched in chunks")
	}
}

func TestChunkedModeSummaryTimeoutWaitsBeforeTheChunkedRetry(t *testing.T) {
	tenantID := uuid.New()
	store := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	modes := &fakeModeStore{mode: report.ModeChunked}
	worker := newModeWorker(store, modes, func(context.Context, sml.Connection, string) ([]map[string]string, error) {
		return nil, &sml.SafeError{Code: "SML_TIMEOUT", Retryable: true, Phase: sml.RequestSentResultUnknown}
	}, true)
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.retriedCode != "SML_TIMEOUT" || store.failCalls != 0 || store.remoteFailCalls != 0 {
		t.Fatalf("retry=%q fail=%d remoteFail=%d", store.retriedCode, store.failCalls, store.remoteFailCalls)
	}
	if wait := store.retriedNotBefore.Sub(modeTestNow); wait < summaryDirectTimeoutRetryWait {
		t.Fatalf("retry after %s, the remote query may still be running", wait)
	}
	if worker.summaryDirectAllowed(tenantID, report.StockBalance) {
		t.Fatal("a timeout must send the report back to chunks")
	}
}

func TestChunkedModeDetailRunsStayChunkedAndOtherFailuresAreNotRetriedAsChunks(t *testing.T) {
	tenantID := uuid.New()
	detail := modeRun(tenantID, report.StockBalance)
	detail.ResultKind = report.ResultDetail
	store := &fakeRunStore{run: detail}
	modes := &fakeModeStore{mode: report.ModeChunked}
	worker := newModeWorker(store, modes, func(_ context.Context, _ sml.Connection, sql string) ([]map[string]string, error) {
		if strings.Contains(sql, "select code as unit_key") {
			return []map[string]string{{"unit_key": "001"}}, nil
		}
		return stockRows(), nil
	}, true)
	if err := worker.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.chunkManifests) == 0 {
		t.Fatal("detail rows are not bounded and must keep using chunks")
	}

	other := &fakeRunStore{run: modeRun(tenantID, report.StockBalance)}
	failing := newModeWorker(other, &fakeModeStore{mode: report.ModeChunked}, func(context.Context, sml.Connection, string) ([]map[string]string, error) {
		return nil, &sml.SafeError{Code: "SML_AUTH_FAILED", Phase: sml.BeforeRequestSent}
	}, true)
	if err := failing.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !failing.summaryDirectAllowed(tenantID, report.StockBalance) {
		t.Fatal("an unrelated failure must not deny the one-query summary")
	}
}
