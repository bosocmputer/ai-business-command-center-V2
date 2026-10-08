package master

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRowsKeepCodesOnceTrimNamesAndFlagInactiveRecords(t *testing.T) {
	customers := Rows(KindCustomer, []map[string]string{
		{"code": " C001 ", "name": "  บริษัท   ตัวอย่าง  จำกัด ", "phone": "081-234-5678", "status": "0"},
		{"code": "C001", "name": "ซ้ำ", "phone": "", "status": "0"},
		{"code": "", "name": "ไม่มีรหัส"},
		{"code": "C002", "name": "เลิกใช้", "phone": "02-111-2222", "status": "1"},
	})
	if len(customers) != 2 || customers[0].Code != "C001" || customers[0].Name != "บริษัท ตัวอย่าง จำกัด" || customers[0].Phone != "081-234-5678" || !customers[0].Active || customers[1].Active {
		t.Fatalf("customers = %+v", customers)
	}
	items := Rows(KindItem, []map[string]string{{"code": "A01", "name": strings.Repeat("ก", 400), "unit": "ถุง", "supplier_code": "S9", "phone": "ignored", "status": "0"}})
	if len(items) != 1 || len([]rune(items[0].Name)) != 300 || items[0].Unit != "ถุง" || items[0].SupplierCode != "S9" || items[0].Phone != "" {
		t.Fatalf("items = %+v", items)
	}
}

func TestEveryQueryIsAFixedReadOnlySelect(t *testing.T) {
	for _, kind := range Kinds {
		statement := strings.ToLower(Query(kind))
		if !strings.HasPrefix(statement, "select ") || strings.Contains(statement, ";") || strings.Contains(statement, "$") {
			t.Errorf("%s: %s", kind, statement)
		}
		for _, forbidden := range []string{"insert ", "update ", "delete ", "drop ", "alter ", "truncate "} {
			if strings.Contains(statement, forbidden) {
				t.Errorf("%s contains %q", kind, forbidden)
			}
		}
		// No business figures are copied: no money, quantity or balance columns.
		for _, forbidden := range []string{"amount", "balance", "cost", "price", "credit"} {
			if strings.Contains(statement, forbidden) {
				t.Errorf("%s copies %q", kind, forbidden)
			}
		}
	}
}

type fakeStore struct {
	targets  []Target
	replaced map[Kind]int
	failed   map[Kind]string
	err      error
}

func (store *fakeStore) Targets(context.Context, time.Time) ([]Target, error) {
	return store.targets, store.err
}
func (store *fakeStore) Replace(_ context.Context, _ uuid.UUID, kind Kind, records []Record, _ time.Time) error {
	if len(records) == 0 {
		return ErrEmpty
	}
	store.replaced[kind] = len(records)
	return nil
}
func (store *fakeStore) MarkFailed(_ context.Context, _ uuid.UUID, kind Kind, code string, _ time.Time) error {
	store.failed[kind] = code
	return nil
}

type fakeSource struct {
	rows map[Kind][]Record
	err  map[Kind]error
}

func (source fakeSource) Fetch(_ context.Context, _ uuid.UUID, kind Kind) ([]Record, error) {
	return source.rows[kind], source.err[kind]
}

var quiet = slog.New(slog.NewJSONHandler(io.Discard, nil))

func newSyncer(store *fakeStore, source fakeSource, at time.Time) *Syncer {
	return NewSyncer(store, source, quiet, func() time.Time { return at })
}

func target() Target {
	return Target{TenantID: uuid.New(), Timezone: "Asia/Bangkok", Due: []Kind{KindCustomer, KindSupplier, KindItem}}
}

func TestTheCopyRunsInTheMorningWindowAndRecordsEachKind(t *testing.T) {
	one := []Record{{Code: "X", Name: "x", Active: true}}
	source := fakeSource{rows: map[Kind][]Record{KindCustomer: one, KindSupplier: one, KindItem: one}, err: map[Kind]error{}}
	for name, at := range map[string]time.Time{
		"06:29 is before the start":  time.Date(2026, 10, 8, 23, 29, 0, 0, time.UTC),
		"10:31 is past the window":   time.Date(2026, 10, 9, 3, 31, 0, 0, time.UTC),
		"midnight belongs to before": time.Date(2026, 10, 8, 17, 0, 0, 0, time.UTC),
	} {
		store := &fakeStore{targets: []Target{target()}, replaced: map[Kind]int{}, failed: map[Kind]string{}}
		if summary := newSyncer(store, source, at).RunOnce(context.Background()); summary != (Summary{}) {
			t.Errorf("%s: %+v", name, summary)
		}
	}
	store := &fakeStore{targets: []Target{target()}, replaced: map[Kind]int{}, failed: map[Kind]string{}}
	if summary := newSyncer(store, source, time.Date(2026, 10, 8, 23, 30, 0, 0, time.UTC)).RunOnce(context.Background()); summary.Copied != 3 || summary.Failed != 0 || summary.Rows != 3 {
		t.Fatalf("06:30 exactly is in the window: %+v", summary)
	}
}

func TestAFailedKindIsRecordedWithAShortCodeAndTheOthersStillRun(t *testing.T) {
	one := []Record{{Code: "X", Name: "x", Active: true}}
	source := fakeSource{rows: map[Kind][]Record{KindCustomer: one, KindItem: one}, err: map[Kind]error{KindSupplier: errors.New("select code from ap_supplier: secret detail")}}
	store := &fakeStore{targets: []Target{target()}, replaced: map[Kind]int{}, failed: map[Kind]string{}}
	summary := newSyncer(store, source, time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)).RunOnce(context.Background())
	if summary.Copied != 2 || summary.Failed != 1 || store.failed[KindSupplier] != "MASTER_COPY_FAILED" {
		t.Fatalf("summary = %+v, failed = %v", summary, store.failed)
	}
	if strings.Contains(store.failed[KindSupplier], "secret") {
		t.Errorf("only a short code is kept, never the error text: %q", store.failed[KindSupplier])
	}
	// An empty answer never wipes a copy: the store refuses it and the failure says why.
	empty := fakeSource{rows: map[Kind][]Record{}, err: map[Kind]error{}}
	store = &fakeStore{targets: []Target{{TenantID: uuid.New(), Timezone: "Asia/Bangkok", Due: []Kind{KindItem}}}, replaced: map[Kind]int{}, failed: map[Kind]string{}}
	newSyncer(store, empty, time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)).RunOnce(context.Background())
	if store.failed[KindItem] != "EMPTY_RESULT" {
		t.Errorf("failed = %v", store.failed)
	}
}
