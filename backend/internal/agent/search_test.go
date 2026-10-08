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

type fakeMaster struct {
	result   MasterResult
	gotKind  MasterKind
	gotWords []string
	gotLimit int
	calls    int
}

func (master *fakeMaster) SearchMaster(_ context.Context, _ uuid.UUID, kind MasterKind, words []string, limit int) (MasterResult, error) {
	master.calls++
	master.gotKind, master.gotWords, master.gotLimit = kind, words, limit
	return master.result, nil
}

func searchService(master *fakeMaster, permitted ...report.Key) (*Service, *fakeStore) {
	store := &fakeStore{permitted: permitted}
	return newService(store, &fakeSnapshots{}).ConfigureMaster(master), store
}

func synced(minutesAgo int) *time.Time { return finished(minutesAgo) }

func TestSearchFindsARecordByWordsAndReportsHowFreshTheCopyIs(t *testing.T) {
	master := &fakeMaster{result: MasterResult{Matches: []MasterMatch{{Code: "C001", Name: "บริษัท ตัวอย่าง จำกัด", Phone: "081-234-5678"}}, Total: 1, SyncedAt: synced(120)}}
	service, store := searchService(master, report.ARAging)
	got, err := service.SearchMaster(context.Background(), namedPrincipal, " customer ", "  ตัวอย่าง   081 ")
	if err != nil || got.Status != "READY" || got.Kind != MasterCustomer || len(got.Matches) != 1 || got.Matches[0].Phone != "081-234-5678" || got.TotalFound != 1 || got.SyncedAt == "" {
		t.Fatalf("got = %+v %v", got, err)
	}
	if master.gotKind != MasterCustomer || strings.Join(master.gotWords, "|") != "ตัวอย่าง|081" || master.gotLimit != 10 {
		t.Errorf("store asked for %v %v %d", master.gotKind, master.gotWords, master.gotLimit)
	}
	last := store.calls[len(store.calls)-1]
	if last.Tool != ToolSearch || last.Outcome != OutcomeOK || last.ReportKey != "customer" {
		t.Errorf("the call log keeps the kind and never the words: %+v", last)
	}
}

func TestSearchSaysWhenThereAreMoreMatchesThanShownAndWhenThereAreNone(t *testing.T) {
	many := &fakeMaster{result: MasterResult{Matches: make([]MasterMatch, 10), Total: 37, SyncedAt: synced(5)}}
	service, _ := searchService(many, report.StockBalance)
	got, _ := service.SearchMaster(context.Background(), principal, "item", "ปูน")
	if got.TotalFound != 37 || len(got.Notes) != 1 || !strings.Contains(got.Notes[0], "ระบุชื่อเพิ่ม") {
		t.Fatalf("got = %+v", got)
	}
	none := &fakeMaster{result: MasterResult{Total: 0, SyncedAt: synced(5)}}
	service, _ = searchService(none, report.StockBalance)
	got, _ = service.SearchMaster(context.Background(), principal, "ITEM", "ไม่มีแน่")
	if got.Status != "READY" || got.Message != MessageSearchNone || len(got.Matches) != 0 {
		t.Fatalf("none = %+v", got)
	}
	old := &fakeMaster{result: MasterResult{Matches: []MasterMatch{{Code: "A"}}, Total: 1, SyncedAt: synced(4 * 24 * 60)}}
	service, _ = searchService(old, report.StockBalance)
	got, _ = service.SearchMaster(context.Background(), principal, "item", "ปูน")
	if len(got.Notes) == 0 || !strings.Contains(strings.Join(got.Notes, " "), "ไม่ได้อัปเดต") {
		t.Fatalf("a copy not refreshed for days must say so: %+v", got)
	}
}

func TestSearchOfPeopleNeedsNamesAndTheRightReportsAndItemsDoNot(t *testing.T) {
	master := &fakeMaster{result: MasterResult{Matches: []MasterMatch{{Code: "C1", Name: "x"}}, Total: 1, SyncedAt: synced(5)}}
	service, store := searchService(master, report.ARAging, report.StockBalance)
	masked, err := service.SearchMaster(context.Background(), principal, "customer", "ตัวอย่าง") // names hidden
	if err != nil || masked.Status != "UNAVAILABLE" || len(masked.Matches) != 0 || master.calls != 0 {
		t.Fatalf("masked = %+v %v (store calls %d)", masked, err, master.calls)
	}
	if store.calls[len(store.calls)-1].Outcome != OutcomeUnavailable {
		t.Errorf("outcome = %s", store.calls[len(store.calls)-1].Outcome)
	}
	items, err := service.SearchMaster(context.Background(), principal, "item", "ปูน")
	if err != nil || items.Status != "READY" {
		t.Fatalf("items need no names: %+v %v", items, err)
	}
	// A supplier search without any report about suppliers, and a kind that does not exist, look the same as a missing report.
	for _, kind := range []string{"supplier", "vendor", ""} {
		if _, err := service.SearchMaster(context.Background(), namedPrincipal, kind, "ตัวอย่าง"); !errors.Is(err, ErrNoData) {
			t.Errorf("kind %q: err = %v", kind, err)
		}
	}
}

func TestSearchRefusesTooShortOrTooLongQueriesInThaiAndNeverLogsThem(t *testing.T) {
	master := &fakeMaster{result: MasterResult{SyncedAt: synced(5)}}
	service, store := searchService(master, report.StockBalance)
	for _, query := range []string{"", "ก", " a ", "หนึ่ง สอง สาม สี่ ห้า หก", strings.Repeat("ก", 101)} {
		_, err := service.SearchMaster(context.Background(), principal, "item", query)
		var invalid *InvalidSearchError
		if !errors.As(err, &invalid) || invalid.Message == "" {
			t.Errorf("query %.10q: err = %v", query, err)
		}
	}
	if master.calls != 0 || store.calls[len(store.calls)-1].Outcome != OutcomeInvalidSearch {
		t.Errorf("a bad query must not reach the store: calls=%d outcome=%s", master.calls, store.calls[len(store.calls)-1].Outcome)
	}
}

func TestSearchSaysWhenTheShopHasNoCopyYetAndIsAbsentUntilConfigured(t *testing.T) {
	service, _ := searchService(&fakeMaster{}, report.StockBalance)
	got, err := service.SearchMaster(context.Background(), principal, "item", "ปูน")
	if err != nil || got.Status != "NOT_SYNCED" || got.Message != MessageNotSynced {
		t.Fatalf("got = %+v %v", got, err)
	}
	bare := newService(&fakeStore{permitted: []report.Key{report.StockBalance}}, &fakeSnapshots{})
	if _, err := bare.SearchMaster(context.Background(), principal, "item", "ปูน"); !errors.Is(err, ErrMasterUnavailable) {
		t.Fatalf("not configured: %v", err)
	}
}
