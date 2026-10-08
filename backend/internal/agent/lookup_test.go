package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
)

type fakeLive struct {
	result LookupResult
	err    error
	asked  []string
}

func (live *fakeLive) Lookup(_ context.Context, _ uuid.UUID, kind LookupKind, code, asOf string) (LookupResult, error) {
	live.asked = append(live.asked, string(kind)+"|"+code+"|"+asOf)
	return live.result, live.err
}

func lookupService(live *fakeLive, permitted ...report.Key) (*Service, *fakeStore) {
	master := &fakeMaster{result: MasterResult{Matches: []MasterMatch{{Code: "C001", Name: "บริษัท ตัวอย่าง จำกัด"}, {Code: "A01", Name: "ปูนซีเมนต์", Unit: "ถุง"}}, SyncedAt: synced(5)}}
	store := &fakeStore{permitted: permitted}
	return newService(store, &fakeSnapshots{}).ConfigureMaster(master).ConfigureLookups(live), store
}

func readyResult() LookupResult {
	return LookupResult{Found: true, AsOf: now, Figures: []Figure{{Key: "total_balance", Label: "ยอดค้างรวม", Unit: "THB", Value: "1500.50"}}}
}

func TestALiveLookupNamesTheRecordAndSaysItWasReadLive(t *testing.T) {
	live := &fakeLive{result: readyResult()}
	service, store := lookupService(live, report.ARAging)
	got, err := service.LiveLookup(context.Background(), namedPrincipal, " customer_balance ", " C001 ")
	if err != nil || got.Status != "READY" || got.Subject == nil || got.Subject.Name != "บริษัท ตัวอย่าง จำกัด" || len(got.Figures) != 1 || got.AsOf == "" {
		t.Fatalf("got = %+v %v", got, err)
	}
	if len(live.asked) != 1 || live.asked[0] != "customer_balance|C001|2026-10-01" {
		t.Errorf("asked = %v", live.asked)
	}
	if len(got.Notes) == 0 || !strings.Contains(got.Notes[0], "อ่านสด") {
		t.Errorf("notes = %v", got.Notes)
	}
	last := store.calls[len(store.calls)-1]
	if last.Tool != ToolLookup || last.ReportKey != "customer_balance" || last.Outcome != OutcomeOK {
		t.Errorf("the call log keeps the kind only: %+v", last)
	}
}

func TestALiveLookupNeedsAKnownRecordAndTheRightPermissions(t *testing.T) {
	live := &fakeLive{result: readyResult()}
	service, _ := lookupService(live, report.ARAging, report.StockBalance)
	notFound, err := service.LiveLookup(context.Background(), namedPrincipal, "customer_balance", "C999")
	if err != nil || notFound.Status != "NOT_FOUND" || len(live.asked) != 0 {
		t.Fatalf("a code the master copy does not know must never reach the shop's system: %+v %v asked=%v", notFound, err, live.asked)
	}
	masked, err := service.LiveLookup(context.Background(), principal, "customer_balance", "C001") // names hidden
	if err != nil || masked.Status != "UNAVAILABLE" || len(live.asked) != 0 {
		t.Fatalf("masked = %+v %v", masked, err)
	}
	stock, err := service.LiveLookup(context.Background(), principal, "item_stock", "A01") // an item needs no names
	if err != nil || stock.Status != "READY" || len(live.asked) != 1 {
		t.Fatalf("stock = %+v %v", stock, err)
	}
	// No report about sales: the sales lookup, a kind that does not exist and an empty kind all look like a missing report.
	for _, kind := range []string{"customer_recent_sales", "price", ""} {
		if _, err := service.LiveLookup(context.Background(), namedPrincipal, kind, "C001"); !errors.Is(err, ErrNoData) {
			t.Errorf("kind %q: err = %v", kind, err)
		}
	}
}

func TestALiveLookupRefusesAnEmptyCodeAndReportsBusyAndFailure(t *testing.T) {
	live := &fakeLive{result: readyResult()}
	service, store := lookupService(live, report.ARAging)
	for _, code := range []string{"", "   ", strings.Repeat("x", 81)} {
		_, err := service.LiveLookup(context.Background(), namedPrincipal, "customer_balance", code)
		var invalid *InvalidLookupError
		if !errors.As(err, &invalid) || invalid.Message == "" {
			t.Errorf("code %.5q: err = %v", code, err)
		}
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeInvalidLookup || len(live.asked) != 0 {
		t.Errorf("outcome = %s asked = %v", store.calls[len(store.calls)-1].Outcome, live.asked)
	}
	live.err = ErrLookupBusy
	busy, err := service.LiveLookup(context.Background(), namedPrincipal, "customer_balance", "C001")
	if err != nil || busy.Status != "BUSY" || busy.Message != MessageLookupBusy {
		t.Fatalf("busy = %+v %v", busy, err)
	}
	live.err = errors.New("sml: select ... secret detail")
	failed, err := service.LiveLookup(context.Background(), namedPrincipal, "customer_balance", "C001")
	if err != nil || failed.Status != "UNAVAILABLE" || strings.Contains(failed.Message, "secret") {
		t.Fatalf("failed = %+v %v", failed, err)
	}
	live.err, live.result = nil, LookupResult{Found: false}
	gone, _ := service.LiveLookup(context.Background(), namedPrincipal, "customer_balance", "C001")
	if gone.Status != "NOT_FOUND" || len(gone.Figures) != 0 {
		t.Fatalf("gone = %+v", gone)
	}
}

func TestLiveLookupsAreAbsentUntilConfigured(t *testing.T) {
	bare := newService(&fakeStore{permitted: []report.Key{report.ARAging}}, &fakeSnapshots{})
	if _, err := bare.LiveLookup(context.Background(), namedPrincipal, "customer_balance", "C001"); !errors.Is(err, ErrLookupUnavailable) {
		t.Fatalf("err = %v", err)
	}
	_ = time.Second
}
