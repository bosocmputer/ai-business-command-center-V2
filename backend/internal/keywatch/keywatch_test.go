package keywatch

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func money(value float64) *float64 { return &value }

type storeFake struct {
	shops   []Shop
	notices map[string]time.Time
	cleared int
}

func (fake *storeFake) Shops(context.Context) ([]Shop, error) { return fake.shops, nil }
func (fake *storeFake) LastNotice(_ context.Context, tenantID uuid.UUID, level string) (time.Time, bool, error) {
	at, ok := fake.notices[tenantID.String()+level]
	return at, ok, nil
}
func (fake *storeFake) MarkNotice(_ context.Context, tenantID uuid.UUID, level string, now time.Time) error {
	fake.notices[tenantID.String()+level] = now
	return nil
}
func (fake *storeFake) ClearNotices(_ context.Context, tenantID uuid.UUID) error {
	fake.cleared++
	for key := range fake.notices {
		if strings.HasPrefix(key, tenantID.String()) {
			delete(fake.notices, key)
		}
	}
	return nil
}

type keysFake map[uuid.UUID]string

func (fake keysFake) KeyFor(_ context.Context, tenantID uuid.UUID) (string, error) {
	return fake[tenantID], nil
}

type checkerFake struct {
	balance Balance
	err     error
	seen    []string
}

func (fake *checkerFake) Balance(_ context.Context, key string) (Balance, error) {
	fake.seen = append(fake.seen, key)
	return fake.balance, fake.err
}

type senderFake struct{ texts []string }

func (fake *senderFake) Send(_ context.Context, text string) error {
	fake.texts = append(fake.texts, text)
	return nil
}

func setup(mode Mode, balance Balance, err error) (*Watcher, *storeFake, *checkerFake, *senderFake, *time.Time, Shop) {
	shop := Shop{TenantID: uuid.New(), Name: "ร้านทดสอบ"}
	store := &storeFake{shops: []Shop{shop}, notices: map[string]time.Time{}}
	checker, sender := &checkerFake{balance: balance, err: err}, &senderFake{}
	clock := time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)
	watcher := NewWatcher(store, keysFake{shop.TenantID: "sk-or-secret-key-value-123456"}, checker, sender, mode, func() time.Time { return clock }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return watcher, store, checker, sender, &clock, shop
}

func TestLevelSaysLowUnderTheLargerOfOneDollarAndAFifthOfTheLimit(t *testing.T) {
	cases := map[string]struct {
		limit, remaining *float64
		want             string
	}{
		"plenty":                  {money(10), money(8), ""},
		"under a fifth of ten":    {money(10), money(1.99), LevelLow},
		"a fifth of ten exactly":  {money(10), money(2), ""},
		"under a dollar of three": {money(3), money(0.59), LevelLow},
		"a dollar of three":       {money(3), money(1), ""},
		"nothing left":            {money(5), money(0), LevelUnusable},
		"overspent":               {money(5), money(-0.01), LevelUnusable},
		"no limit":                {nil, nil, ""},
	}
	for name, tc := range cases {
		if got := Level(Balance{Limit: tc.limit, Remaining: tc.remaining}); got != tc.want {
			t.Fatalf("%s: level %q, want %q", name, got, tc.want)
		}
	}
}

func TestALowKeyIsReportedOnceADayAndForgottenWhenItRecovers(t *testing.T) {
	watcher, store, checker, sender, clock, _ := setup(ModeSend, Balance{Limit: money(5), Remaining: money(0.59)}, nil)
	if summary := watcher.RunOnce(context.Background()); summary.Low != 1 || summary.Told != 1 || len(sender.texts) != 1 {
		t.Fatalf("first pass = %+v texts=%d", summary, len(sender.texts))
	}
	for _, text := range sender.texts {
		if strings.Contains(text, "sk-or") || !strings.Contains(text, "$0.59") || !strings.Contains(text, "ร้านทดสอบ") {
			t.Fatalf("message must name the shop and the amount and never a key: %q", text)
		}
	}
	*clock = clock.Add(30 * time.Minute)
	if watcher.RunOnce(context.Background()); len(sender.texts) != 1 {
		t.Fatal("the same spell must not be reported again within a day")
	}
	*clock = clock.Add(25 * time.Hour)
	if watcher.RunOnce(context.Background()); len(sender.texts) != 2 {
		t.Fatal("a spell that lasts is reminded after a day")
	}
	checker.balance = Balance{Limit: money(20), Remaining: money(19)}
	if watcher.RunOnce(context.Background()); len(store.notices) != 0 || store.cleared == 0 {
		t.Fatalf("a recovered key must clear its notices: %+v", store.notices)
	}
}

func TestARefusedKeyIsReportedAsUnusableAndUnreachableOpenRouterSaysNothing(t *testing.T) {
	watcher, _, _, sender, _, _ := setup(ModeSend, Balance{}, &ErrRefused{Status: 403})
	if summary := watcher.RunOnce(context.Background()); summary.Unusable != 1 || len(sender.texts) != 1 || !strings.Contains(sender.texts[0], "🛑") {
		t.Fatalf("summary=%+v texts=%v", summary, sender.texts)
	}
	watcher, _, _, sender, _, _ = setup(ModeSend, Balance{}, errors.New("network down"))
	if summary := watcher.RunOnce(context.Background()); summary.Unknown != 1 || len(sender.texts) != 0 {
		t.Fatalf("a check that could not be made must not alarm anyone: %+v texts=%v", summary, sender.texts)
	}
}

func TestObserveAndOffModesTellNobodyAndAShopWithoutAKeyIsNotAsked(t *testing.T) {
	for _, mode := range []Mode{ModeObserve, ModeOff} {
		watcher, store, _, sender, _, _ := setup(mode, Balance{Limit: money(5), Remaining: money(0)}, nil)
		watcher.RunOnce(context.Background())
		if len(sender.texts) != 0 || len(store.notices) != 0 {
			t.Fatalf("%s must send and remember nothing", mode)
		}
	}
	watcher, _, checker, _, _, shop := setup(ModeSend, Balance{Limit: money(5), Remaining: money(0)}, nil)
	watcher.keys = keysFake{shop.TenantID: ""}
	watcher.RunOnce(context.Background())
	if len(checker.seen) != 0 {
		t.Fatal("a shop with no key to use must not be checked")
	}
}

func TestOpenRouterClientSendsTheKeyOnlyAsABearerAndNeverReturnsItInAnError(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		auth = request.Header.Get("Authorization")
		switch request.URL.Query().Get("case") {
		case "refused":
			response.WriteHeader(http.StatusForbidden)
		case "down":
			response.WriteHeader(http.StatusInternalServerError)
			_, _ = response.Write([]byte("sk-or-secret-key-value-123456"))
		case "unlimited":
			_, _ = response.Write([]byte(`{"data":{"limit":null,"limit_remaining":null,"usage":3.2}}`))
		default:
			_, _ = response.Write([]byte(`{"data":{"limit":5,"limit_remaining":0.59,"usage":4.41}}`))
		}
	}))
	defer server.Close()
	key := "sk-or-secret-key-value-123456"
	client := func(query string) *OpenRouter { return NewOpenRouter(server.URL+"?case="+query, 2*time.Second) }

	balance, err := client("ok").Balance(context.Background(), key)
	if err != nil || balance.Remaining == nil || *balance.Remaining != 0.59 || *balance.Limit != 5 || auth != "Bearer "+key {
		t.Fatalf("balance = %+v %v auth=%q", balance, err, auth)
	}
	if balance, err = client("unlimited").Balance(context.Background(), key); err != nil || balance.Limit != nil || Level(balance) != "" {
		t.Fatalf("a key without a limit is never low: %+v %v", balance, err)
	}
	var refused *ErrRefused
	if _, err = client("refused").Balance(context.Background(), key); !asRefused(err, &refused) || refused.Status != 403 {
		t.Fatalf("403 must be a refused key: %v", err)
	}
	if _, err = client("down").Balance(context.Background(), key); err == nil || strings.Contains(err.Error(), "sk-or") {
		t.Fatalf("an error must carry neither the key nor the body: %v", err)
	}
}
