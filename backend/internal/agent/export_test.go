package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/viewer"
	"github.com/google/uuid"
)

// fakeExports holds one run of rows and pages through them the way the viewer's own row reader does: at most 100 a
// page, with a cursor that is the ordinal reached.
type fakeExports struct {
	rows       []map[string]string
	own        viewer.DashboardSnapshot
	ownErr     error
	created    []string
	createErr  error
	runStatus  report.RunStatus
	run        report.Run
	listErr    error
	listedWith []int
}

func (exports *fakeExports) OwnDetailRun(context.Context, uuid.UUID, uuid.UUID, report.Key, report.Period) (viewer.DashboardSnapshot, error) {
	return exports.own, exports.ownErr
}

func (exports *fakeExports) Create(_ context.Context, _, _ uuid.UUID, _ report.Key, idempotencyKey string, _ viewer.CreateReportRunInput) (report.Run, error) {
	exports.created = append(exports.created, idempotencyKey)
	if exports.createErr != nil {
		return report.Run{}, exports.createErr
	}
	run := exports.run
	run.Status = exports.runStatus
	return run, nil
}

func (exports *fakeExports) Get(_ context.Context, _, _ uuid.UUID, key report.Key, runID uuid.UUID) (report.Run, error) {
	run := exports.run
	run.ID, run.ReportKey, run.TenantID = runID, key, tenantID
	run.Status, run.RowCount = report.StatusSucceeded, len(exports.rows)
	run.Period = report.Period{DateFrom: "2026-09-01", DateTo: "2026-09-30"}
	return run, nil
}

func (exports *fakeExports) ListRows(_ context.Context, _, _ uuid.UUID, _ report.Key, _ uuid.UUID, cursor string, pageSize int) (viewer.ReportRows, error) {
	if exports.listErr != nil {
		return viewer.ReportRows{}, exports.listErr
	}
	exports.listedWith = append(exports.listedWith, pageSize)
	after := 0
	if cursor != "" {
		after, _ = strconv.Atoi(cursor)
	}
	end := min(after+pageSize, len(exports.rows))
	page := viewer.ReportRows{Rows: exports.rows[after:end], HasMore: end < len(exports.rows)}
	if page.HasMore {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}

// columnIndex finds a column by its key, so the tests do not break when the report page gains a column.
func columnIndex(t *testing.T, columns []ExportColumn, key string) int {
	t.Helper()
	for index, column := range columns {
		if column.Key == key {
			return index
		}
	}
	t.Fatalf("no column %s", key)
	return -1
}

func salesRows(count int) []map[string]string {
	rows := make([]map[string]string, count)
	for index := range rows {
		rows[index] = map[string]string{"doc_date": "2026-09-01", "doc_no": fmt.Sprintf("IV-%05d", index), "cust_name": "บริษัท ตัวอย่าง จำกัด", "item_name": "ปูนซีเมนต์", "qty": "2", "sum_amount": "250.00", "secret_internal": "x"}
	}
	return rows
}

func exportService(exports *fakeExports, permitted ...report.Key) (*Service, *fakeStore) {
	store := &fakeStore{permitted: permitted}
	return newService(store, &fakeSnapshots{}).ConfigureExports(exports), store
}

func freshOwn() viewer.DashboardSnapshot {
	return viewer.DashboardSnapshot{RunID: uuid.New(), FreshnessStatus: viewer.FreshnessFresh}
}

func TestAnExportGivesEveryRowOfTheFreshRunInThePagesAskedFor(t *testing.T) {
	exports := &fakeExports{rows: salesRows(1200), own: freshOwn()}
	service, store := exportService(exports, report.SalesGoodsServices)
	first, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if err != nil || first.Status != "READY" || len(first.Rows) != 500 || first.NextCursor == "" || first.TotalRows != 1200 {
		t.Fatalf("first = %d rows %q %v %v", len(first.Rows), first.NextCursor, first.TotalRows, err)
	}
	if len(exports.created) != 0 {
		t.Errorf("a fresh run of the person's own must not be fetched again: %v", exports.created)
	}
	docNo, customer := columnIndex(t, first.Columns, "doc_no"), columnIndex(t, first.Columns, "cust_name")
	if first.Rows[0][docNo] != "IV-00000" || first.Rows[0][customer] != "บริษัท ตัวอย่าง จำกัด" {
		t.Errorf("rows follow the columns the page shows: %v", first.Rows[0])
	}
	if len(first.Rows[0]) != len(first.Columns) {
		t.Errorf("one cell per column: %v", first.Rows[0])
	}
	total := len(first.Rows)
	cursor := first.NextCursor
	for cursor != "" {
		next, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "", "", cursor)
		if err != nil || next.Status != "READY" {
			t.Fatalf("next = %+v %v", next, err)
		}
		total += len(next.Rows)
		cursor = next.NextCursor
	}
	if total != 1200 {
		t.Errorf("all rows must come out exactly once, got %d", total)
	}
	counted := 0
	for _, call := range store.calls {
		if call.Tool == ToolExport {
			counted++
		}
	}
	if counted != 1 {
		t.Errorf("one export is one counted call, not one per page: %d", counted)
	}
}

func TestAnExportStopsAtTheRowCapAndSaysSo(t *testing.T) {
	exports := &fakeExports{rows: salesRows(exportMaxRows + 700), own: freshOwn()}
	service, _ := exportService(exports, report.SalesGoodsServices)
	got, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	total, cursor, truncated := len(got.Rows), got.NextCursor, got.Truncated
	for err == nil && cursor != "" {
		got, err = service.Export(context.Background(), namedPrincipal, "sales_goods_services", "", "", cursor)
		total += len(got.Rows)
		cursor, truncated = got.NextCursor, truncated || got.Truncated
	}
	if err != nil || total != exportMaxRows || !truncated || got.TotalRows != exportMaxRows+700 {
		t.Fatalf("total = %d truncated = %v err = %v", total, truncated, err)
	}
	if !strings.Contains(strings.Join(got.Notes, " "), "20,000") {
		t.Errorf("the cap must be said: %v", got.Notes)
	}
}

func TestAnExportHidesNamesWhenTheTokenMustNotSeeThem(t *testing.T) {
	exports := &fakeExports{rows: salesRows(3), own: freshOwn()}
	service, _ := exportService(exports, report.SalesGoodsServices)
	got, err := service.Export(context.Background(), principal, "sales_goods_services", "2026-09-01", "2026-09-30", "") // names hidden
	if err != nil || len(got.Rows) != 3 {
		t.Fatalf("got = %+v %v", got, err)
	}
	customer := columnIndex(t, got.Columns, "cust_name")
	for _, row := range got.Rows {
		if strings.Contains(strings.Join(row, "|"), "ตัวอย่าง") || !strings.HasPrefix(row[customer], "ลูกค้า-") {
			t.Errorf("a customer name leaked into the file: %v", row)
		}
	}
	if !strings.Contains(strings.Join(got.Notes, " "), MessageMasked) {
		t.Errorf("notes = %v", got.Notes)
	}
	named, _ := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if named.Rows[0][customer] != "บริษัท ตัวอย่าง จำกัด" {
		t.Errorf("a token that may see names keeps them: %v", named.Rows[0])
	}
}

func TestAnExportStartsOneFetchAndAsksAgainLater(t *testing.T) {
	exports := &fakeExports{rows: salesRows(2), ownErr: report.ErrRunNotFound, runStatus: report.StatusRunning}
	service, store := exportService(exports, report.SalesGoodsServices)
	first, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if err != nil || first.Status != "PREPARING" || first.RetryAfterSeconds <= 0 || len(first.Rows) != 0 || len(exports.created) != 1 {
		t.Fatalf("first = %+v %v created=%v", first, err, exports.created)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomePreparing {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
	exports.runStatus = report.StatusSucceeded
	exports.run.ID = uuid.New()
	second, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if err != nil || second.Status != "READY" || len(second.Rows) != 2 {
		t.Fatalf("second = %+v %v", second, err)
	}
	if len(exports.created) != 2 || exports.created[0] != exports.created[1] {
		t.Errorf("asking again in the same hour must be the same request, not a new fetch: %v", exports.created)
	}
}

func TestAnExportWithoutThePermissionOrOfAnUnknownReportAnswersAlike(t *testing.T) {
	exports := &fakeExports{rows: salesRows(2), own: freshOwn()}
	service, _ := exportService(exports, report.StockBalance)
	for _, key := range []string{"sales_goods_services", "no_such_report", ""} {
		if _, err := service.Export(context.Background(), namedPrincipal, key, "", "", ""); !errors.Is(err, ErrNoData) {
			t.Errorf("key %q: err = %v", key, err)
		}
	}
	if _, err := service.Export(context.Background(), namedPrincipal, "stock_balance", "", "", "not-a-cursor"); !errors.Is(err, ErrInvalidPeriod) {
		t.Errorf("a made-up cursor must be refused: %v", err)
	}
	bare := newService(&fakeStore{permitted: []report.Key{report.SalesGoodsServices}}, &fakeSnapshots{})
	if _, err := bare.Export(context.Background(), namedPrincipal, "sales_goods_services", "", "", ""); !errors.Is(err, ErrExportUnavailable) {
		t.Errorf("bare err = %v", err)
	}
}

func TestAnExportReportsFailureAndExpiredRowsWithoutDetail(t *testing.T) {
	exports := &fakeExports{rows: salesRows(2), ownErr: report.ErrRunNotFound, createErr: errors.New("sml: select secret detail")}
	service, _ := exportService(exports, report.SalesGoodsServices)
	failed, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if err != nil || failed.Status != "UNAVAILABLE" || strings.Contains(failed.Message, "secret") {
		t.Fatalf("failed = %+v %v", failed, err)
	}
	busy := &fakeExports{rows: salesRows(2), ownErr: report.ErrRunNotFound, createErr: report.ErrRunConcurrencyLimit}
	service, _ = exportService(busy, report.SalesGoodsServices)
	if got, _ := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", ""); got.Status != "UNAVAILABLE" || got.RetryAfterSeconds == 0 {
		t.Errorf("busy = %+v", got)
	}
	expired := &fakeExports{rows: salesRows(2), own: freshOwn(), listErr: report.ErrRunRowsExpired}
	service, _ = exportService(expired, report.SalesGoodsServices)
	got, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if err != nil || got.Status != "UNAVAILABLE" || got.Message != MessageExportExpired || len(got.Rows) != 0 {
		t.Errorf("expired = %+v %v", got, err)
	}
}

func TestEveryReportThatHasRowsHasColumnsAndNameColumnsAreInThem(t *testing.T) {
	for _, definition := range report.Definitions() {
		if definition.Status != report.StatusActive {
			continue
		}
		columns, ok := exportColumns[definition.Key]
		if !ok || len(columns) == 0 {
			t.Errorf("%s has no export columns", definition.Key)
			continue
		}
		have := map[string]bool{}
		for _, column := range columns {
			if column.Label == "" || have[column.Key] || (column.Type != columnText && column.Type != columnNumber && column.Type != columnDate) {
				t.Errorf("%s column %+v is wrong or repeated", definition.Key, column)
			}
			have[column.Key] = true
		}
		for _, name := range exportNameColumns[definition.Key] {
			if !have[name] {
				t.Errorf("%s: name column %s is not exported", definition.Key, name)
			}
		}
	}
}

func TestOnlyAmountsOfMoneyAreMarkedToBeAddedUp(t *testing.T) {
	marked := map[string]bool{}
	for key, columns := range exportColumns {
		for _, column := range columns {
			if column.Total && column.Type != columnNumber {
				t.Errorf("%s %s: only a number can be added up", key, column.Key)
			}
			if column.Total {
				marked[column.Key] = true
			}
		}
	}
	for _, key := range []string{"sum_amount", "total_amount", "balance", "amount_sale", "total_net_value", "cash_amount"} {
		if !marked[key] {
			t.Errorf("%s is money and must be added up", key)
		}
	}
	for _, key := range []string{"qty", "price", "average_cost", "days_past_due", "r_score", "balance_qty", "avg_gap_days"} {
		if marked[key] {
			t.Errorf("%s must not be added up", key)
		}
	}
	for key := range moneyColumns {
		found := false
		for _, columns := range exportColumns {
			for _, column := range columns {
				found = found || column.Key == key
			}
		}
		if !found {
			t.Errorf("money column %s belongs to no report", key)
		}
	}
}

func TestACodeTheRowsCarryIsShownInWords(t *testing.T) {
	rows := salesRows(2)
	rows[0]["vat_type"], rows[1]["vat_type"] = "I", "C"
	exports := &fakeExports{rows: rows, own: freshOwn()}
	service, _ := exportService(exports, report.SalesGoodsServices)
	got, err := service.Export(context.Background(), namedPrincipal, "sales_goods_services", "2026-09-01", "2026-09-30", "")
	if err != nil || len(got.Rows) != 2 {
		t.Fatalf("got = %+v %v", got, err)
	}
	vat := columnIndex(t, got.Columns, "vat_type")
	if got.Rows[0][vat] != "VAT รวมใน" || got.Rows[1][vat] != "VAT 0%" {
		t.Errorf("vat types = %q %q", got.Rows[0][vat], got.Rows[1][vat])
	}
}
