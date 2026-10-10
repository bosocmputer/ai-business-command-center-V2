package assistantcfg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
)

type fakeStore struct {
	rows    map[uuid.UUID]*Stored
	global  StoredGlobal
	gate    TenantGate
	status  map[uuid.UUID]Status
	patches int
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[uuid.UUID]*Stored{}, global: StoredGlobal{ConfigVersion: 1}, gate: TenantGate{Name: "ร้านทดสอบ", Active: true}, status: map[uuid.UUID]Status{}}
}

func (store *fakeStore) row(id uuid.UUID) *Stored {
	if store.rows[id] == nil {
		fresh := defaults(id)
		fresh.Version, fresh.ConfigVersion = 1, 1
		store.rows[id] = &fresh
	}
	return store.rows[id]
}

func (store *fakeStore) Get(_ context.Context, id uuid.UUID) (Stored, error) {
	if store.rows[id] == nil {
		return Stored{}, ErrNotConfigured
	}
	return *store.rows[id], nil
}

func (store *fakeStore) Patch(_ context.Context, _ []byte, _ string, id uuid.UUID, patch Patch, expected int, _ time.Time) (Stored, error) {
	store.patches++
	existing := store.rows[id]
	if (existing == nil && expected != 0) || (existing != nil && existing.Version != expected) {
		return Stored{}, ErrVersionConflict
	}
	row := store.row(id)
	if existing != nil {
		row.Version++
		row.ConfigVersion++
	}
	row.Enabled, row.IsTest, row.ModelKey, row.LineMode, row.AssistantHost = *patch.Enabled, *patch.IsTest, *patch.ModelKey, *patch.LineMode, *patch.AssistantHost
	return *row, nil
}

func (store *fakeStore) PutSecret(_ context.Context, _ []byte, _ string, id uuid.UUID, field Field, sealed *secret.Sealed, last4 string, _ time.Time) (Stored, error) {
	row := store.row(id)
	switch field {
	case FieldOpenRouterKey:
		row.OpenRouterKey, row.OpenRouterLast4 = sealed, last4
	case FieldTelegramToken:
		row.TelegramToken = sealed
	case FieldLineSecret:
		row.LineSecret = sealed
	case FieldLineToken:
		row.LineToken = sealed
	}
	row.Version++
	row.ConfigVersion++
	return *row, nil
}

func (store *fakeStore) GetGlobal(context.Context) (StoredGlobal, error) { return store.global, nil }

func (store *fakeStore) PutGlobalSecret(_ context.Context, _ []byte, _ string, field Field, sealed *secret.Sealed, _ time.Time) (StoredGlobal, error) {
	if field == FieldLineSecret {
		store.global.LineSecret = sealed
	} else {
		store.global.LineToken = sealed
	}
	store.global.ConfigVersion++
	return store.global, nil
}

func (store *fakeStore) Gate(context.Context, uuid.UUID, time.Time) (TenantGate, error) {
	return store.gate, nil
}

func (store *fakeStore) RecordStatus(_ context.Context, id uuid.UUID, status Status, _ time.Time) error {
	store.status[id] = status
	return nil
}

func (store *fakeStore) GetStatus(_ context.Context, id uuid.UUID) (Status, error) {
	if status, ok := store.status[id]; ok {
		return status, nil
	}
	return Status{}, ErrNotConfigured
}

const (
	goodOpenRouter = "sk-or-v1-0123456789abcdefghijklmnopqrstuvwxyz"
	goodTelegram   = "123456789:AAEhBOweik6ad0uUl_abcdefghijklmnopqrs"
	goodLineSecret = "0123456789abcdef0123456789abcdef"
	goodLineToken  = "line-access-token-0123456789-abcdefghijklmnopqrstuvwxyz"
)

func newService(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	box, err := secret.NewBox(bytes.Repeat([]byte{7}, 32), "test-key", bytes.NewReader(bytes.Repeat([]byte{9}, 4096)))
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeStore()
	return NewService(store, box, func() time.Time { return time.Date(2026, 10, 9, 5, 0, 0, 0, time.UTC) }), store
}

var actor = []byte("actor")

func ptr[T any](value T) *T { return &value }

// versionOf is the version a page would have loaded: the tests ask for it instead of guessing.
func versionOf(store *fakeStore, id uuid.UUID) int {
	if row := store.rows[id]; row != nil {
		return row.Version
	}
	return 0
}

func TestTheAdminViewNeverHoldsASecret(t *testing.T) {
	service, _ := newService(t)
	shop := uuid.New()
	for field, value := range map[Field]string{FieldOpenRouterKey: goodOpenRouter, FieldTelegramToken: goodTelegram, FieldLineSecret: goodLineSecret, FieldLineToken: goodLineToken} {
		if _, err := service.SetSecret(context.Background(), actor, "r", shop, field, value); err != nil {
			t.Fatalf("%s: %v", field, err)
		}
	}
	view, err := service.View(context.Background(), shop)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	for _, value := range []string{goodOpenRouter, goodTelegram, goodLineSecret, goodLineToken} {
		if strings.Contains(string(raw), value) {
			t.Fatalf("the view leaked a secret: %s", raw)
		}
	}
	if !view.Secrets["openrouterKey"].IsSet || view.Secrets["openrouterKey"].Last4 != "wxyz" || !view.Secrets["lineChannelToken"].IsSet {
		t.Errorf("secrets = %+v", view.Secrets)
	}
	if view.Secrets["lineChannelToken"].Last4 != "" || view.Secrets["telegramBotToken"].Last4 != "" {
		t.Error("only the OpenRouter key shows its last characters")
	}
}

func TestASecretIsCheckedForShapeAndTheRefusalNeverEchoesIt(t *testing.T) {
	service, _ := newService(t)
	cases := map[Field][]string{
		FieldOpenRouterKey: {"", " sk-or-v1-0123456789abcdefghijkl", "sk-or-short", "sk-live-0123456789abcdefghijklmnop", goodOpenRouter + " x"},
		FieldTelegramToken: {"123:abc", "notatoken", goodTelegram + "\n"},
		FieldLineSecret:    {"ABCDEF0123456789ABCDEF0123456789", "0123", goodLineSecret + "0"},
		FieldLineToken:     {"short", strings.Repeat("x", 4097), "has space " + strings.Repeat("x", 40)},
	}
	for field, values := range cases {
		for _, value := range values {
			_, err := service.SetSecret(context.Background(), actor, "r", uuid.New(), field, value)
			var invalid *ValidationError
			if !errors.As(err, &invalid) || invalid.Code != "INVALID_FORMAT" || (value != "" && strings.Contains(err.Error(), value)) {
				t.Errorf("%s %q: err = %v", field, value, err)
			}
		}
	}
}

func TestAShopCannotBeSwitchedOnWithoutWhatItNeeds(t *testing.T) {
	service, store := newService(t)
	shop := uuid.New()
	_, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{Enabled: ptr(true)})
	assertCode(t, err, "OPENROUTER_KEY_REQUIRED")
	if _, err := service.SetSecret(context.Background(), actor, "r", shop, FieldOpenRouterKey, goodOpenRouter); err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), actor, "r", shop, UpdateInput{Enabled: ptr(true), LineMode: ptr(LineOwn), Version: versionOf(store, shop)})
	assertCode(t, err, "LINE_OWN_INCOMPLETE")
	_, err = service.Update(context.Background(), actor, "r", shop, UpdateInput{Enabled: ptr(true), LineMode: ptr(LineCentral), Version: versionOf(store, shop)})
	assertCode(t, err, "LINE_CENTRAL_NOT_CONFIGURED")
	view, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{Enabled: ptr(true), Version: versionOf(store, shop)})
	if err != nil || !view.Enabled {
		t.Fatalf("a shop with a key may be switched on: %+v %v", view, err)
	}
	if _, err := service.ClearSecret(context.Background(), actor, "r", shop, FieldOpenRouterKey); err == nil {
		t.Error("the key of a shop that is on must not be removed")
	}
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var invalid *ValidationError
	if !errors.As(err, &invalid) || invalid.Code != code {
		t.Fatalf("err = %v, want code %s", err, code)
	}
}

func TestTheFreeModelIsForTestShopsOnly(t *testing.T) {
	service, store := newService(t)
	shop := uuid.New()
	_, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{ModelKey: ptr("openrouter-free")})
	assertCode(t, err, "MODEL_FOR_TEST_SHOPS_ONLY")
	view, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{ModelKey: ptr("openrouter-free"), IsTest: ptr(true)})
	if err != nil || view.ModelKey != "openrouter-free" {
		t.Fatalf("view = %+v err = %v", view, err)
	}
	_, err = service.Update(context.Background(), actor, "r", shop, UpdateInput{IsTest: ptr(false), Version: versionOf(store, shop)})
	assertCode(t, err, "MODEL_FOR_TEST_SHOPS_ONLY")
	_, err = service.Update(context.Background(), actor, "r", shop, UpdateInput{ModelKey: ptr("no-such-model"), Version: versionOf(store, shop)})
	assertCode(t, err, "UNKNOWN_MODEL")
	for _, model := range view.Models {
		if model.Key == "openrouter-free" && !model.Selectable {
			t.Error("for a test shop the free model is selectable")
		}
	}
}

func TestSettingsChangedFromAStalePageAreRefused(t *testing.T) {
	service, _ := newService(t)
	shop := uuid.New()
	if _, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{ModelKey: ptr("glm-5.3-flash")}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{ModelKey: ptr("qwen-3.8-27b"), Version: 7}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("err = %v", err)
	}
}

func readyShop(t *testing.T, service *Service) uuid.UUID {
	t.Helper()
	store := service.store.(*fakeStore)
	shop := uuid.New()
	if _, err := service.SetSecret(context.Background(), actor, "r", shop, FieldOpenRouterKey, goodOpenRouter); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetSecret(context.Background(), actor, "r", shop, FieldTelegramToken, goodTelegram); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{Enabled: ptr(true), ModelKey: ptr("deepseek-v4.1-flash"), Version: versionOf(store, shop)}); err != nil {
		t.Fatal(err)
	}
	return shop
}

func TestTheAssistantGetsItsModelWithPinnedProvidersAndItsSecrets(t *testing.T) {
	service, _ := newService(t)
	shop := readyShop(t, service)
	config, err := service.AgentConfig(context.Background(), shop)
	if err != nil || !config.Enabled || config.Model == nil || config.Secrets == nil {
		t.Fatalf("config = %+v %v", config, err)
	}
	if config.Model.ModelID != "deepseek/deepseek-v4.1-flash" || len(config.Model.Providers) == 0 || !config.Model.DataCollectionDeny {
		t.Errorf("model = %+v: a model is never sent to any provider", config.Model)
	}
	if config.Secrets.OpenRouterKey != goodOpenRouter || config.Secrets.TelegramBotToken != goodTelegram || config.ShopName != "ร้านทดสอบ" {
		t.Errorf("config = %+v", config)
	}
}

func TestAnAssistantThatMustNotRunGetsNoSecretAndNoModel(t *testing.T) {
	service, store := newService(t)
	shop := readyShop(t, service)
	store.gate.Active = false
	config, _ := service.AgentConfig(context.Background(), shop)
	if config.Enabled || config.Reason != "EXPIRED" || config.Secrets != nil || config.Model != nil {
		t.Errorf("past the end date: %+v", config)
	}
	store.gate.Active = true
	if _, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{Enabled: ptr(false), Version: store.rows[shop].Version}); err != nil {
		t.Fatal(err)
	}
	config, _ = service.AgentConfig(context.Background(), shop)
	if config.Enabled || config.Reason != "DISABLED" || config.Secrets != nil {
		t.Errorf("switched off: %+v", config)
	}
	none, _ := service.AgentConfig(context.Background(), uuid.New())
	if none.Enabled || none.Reason != "NOT_CONFIGURED" {
		t.Errorf("unknown shop: %+v", none)
	}
}

func TestTheVersionTagChangesWhenSomethingChangesOrTheShopExpires(t *testing.T) {
	service, store := newService(t)
	shop := readyShop(t, service)
	first, _ := service.AgentConfig(context.Background(), shop)
	if _, err := service.SetSecret(context.Background(), actor, "r", shop, FieldTelegramToken, goodTelegram); err != nil {
		t.Fatal(err)
	}
	second, _ := service.AgentConfig(context.Background(), shop)
	store.gate.Active = false
	third, _ := service.AgentConfig(context.Background(), shop)
	if first.ETag() == second.ETag() || second.ETag() == third.ETag() {
		t.Errorf("tags = %s %s %s", first.ETag(), second.ETag(), third.ETag())
	}
	if _, err := service.SetGlobalSecret(context.Background(), actor, "r", FieldLineSecret, goodLineSecret); err != nil {
		t.Fatal(err)
	}
	store.gate.Active = true
	fourth, _ := service.AgentConfig(context.Background(), shop)
	if fourth.ConfigVersion <= second.ConfigVersion {
		t.Errorf("a change of the shared LINE channel must move every shop's version: %d <= %d", fourth.ConfigVersion, second.ConfigVersion)
	}
}

func TestLineCentralUsesTheOperatorsChannelAndOwnUsesTheShops(t *testing.T) {
	service, _ := newService(t)
	ctx := context.Background()
	if _, err := service.SetGlobalSecret(ctx, actor, "r", FieldLineSecret, goodLineSecret); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetGlobalSecret(ctx, actor, "r", FieldLineToken, "central-token-0123456789-abcdefghijklmnopqrstuv"); err != nil {
		t.Fatal(err)
	}
	central := readyShop(t, service)
	if _, err := service.Update(ctx, actor, "r", central, UpdateInput{LineMode: ptr(LineCentral), Version: versionOf(service.store.(*fakeStore), central)}); err != nil {
		t.Fatal(err)
	}
	config, _ := service.AgentConfig(ctx, central)
	if config.Secrets.LineChannelSecret != goodLineSecret || config.Secrets.LineChannelAccessToken != "central-token-0123456789-abcdefghijklmnopqrstuv" || config.LineMode != LineCentral {
		t.Errorf("central = %+v", config.Secrets)
	}
	own := readyShop(t, service)
	if _, err := service.SetSecret(ctx, actor, "r", own, FieldLineSecret, "fedcba9876543210fedcba9876543210"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetSecret(ctx, actor, "r", own, FieldLineToken, "own-token-0123456789-abcdefghijklmnopqrstuvwxyz-0123"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(ctx, actor, "r", own, UpdateInput{LineMode: ptr(LineOwn), Version: versionOf(service.store.(*fakeStore), own)}); err != nil {
		t.Fatal(err)
	}
	config, _ = service.AgentConfig(ctx, own)
	if config.Secrets.LineChannelSecret != "fedcba9876543210fedcba9876543210" || !strings.HasPrefix(config.Secrets.LineChannelAccessToken, "own-token") {
		t.Errorf("own = %+v", config.Secrets)
	}
	if _, err := service.SetGlobalSecret(ctx, actor, "r", FieldOpenRouterKey, goodOpenRouter); err == nil {
		t.Error("only the LINE channel is shared between shops")
	}
}

func TestASecretSealedForOneShopCannotBeOpenedAsAnothers(t *testing.T) {
	service, store := newService(t)
	a, b := readyShop(t, service), readyShop(t, service)
	store.rows[b].OpenRouterKey = store.rows[a].OpenRouterKey // a copy of shop A's sealed key written into shop B's row
	config, _ := service.AgentConfig(context.Background(), b)
	if config.Enabled || config.Reason != "SECRET_UNREADABLE" || config.Secrets != nil {
		t.Fatalf("a key moved between shops must not open: %+v", config)
	}
}

func TestTheAssistantsStatusReportKeepsOnlySafeValues(t *testing.T) {
	service, _ := newService(t)
	shop := readyShop(t, service)
	started := time.Date(2026, 10, 9, 4, 0, 0, 0, time.UTC)
	if err := service.RecordStatus(context.Background(), shop, StatusReport{ConfigVersion: 3, ModelKey: "deepseek-v4.1-flash", StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []StatusReport{{ConfigVersion: -1}, {ModelKey: "mystery"}, {ErrorCode: "free text with a secret sk-or-xyz"}} {
		if err := service.RecordStatus(context.Background(), shop, bad); err == nil {
			t.Errorf("%+v must be refused", bad)
		}
	}
	view, _ := service.View(context.Background(), shop)
	if view.Status == nil || view.Status.AppliedConfigVersion != 3 || view.Status.UpToDate {
		t.Errorf("status = %+v", view.Status)
	}
}

func TestAShopThatDoesNotExistIsNotFoundEverywhere(t *testing.T) {
	service, store := newService(t)
	store.gate = TenantGate{}
	ctx, id := context.Background(), uuid.New()
	if _, err := service.View(ctx, id); !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("View err = %v", err)
	}
	if _, err := service.Update(ctx, actor, "r", id, UpdateInput{Enabled: ptr(false)}); !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("Update err = %v", err)
	}
	if _, err := service.SetSecret(ctx, actor, "r", id, FieldOpenRouterKey, goodOpenRouter); !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("SetSecret err = %v", err)
	}
	if _, err := service.ClearSecret(ctx, actor, "r", id, FieldTelegramToken); !errors.Is(err, ErrTenantNotFound) {
		t.Errorf("ClearSecret err = %v", err)
	}
	if len(store.rows) != 0 {
		t.Error("nothing may be written for a shop that does not exist")
	}
}

func TestTheAssistantHostIsOneInternalNameAndNothingElse(t *testing.T) {
	service, store := newService(t)
	shop := uuid.New()
	for _, bad := range []string{"assistant.example.com", "http://assistant", "assistant:8646", "Assistant", "-assistant", "a/b", "10.0.0.1/x", "ass istant"} {
		_, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{AssistantHost: ptr(bad), Version: versionOf(store, shop)})
		assertCode(t, err, "INVALID_HOST")
	}
	view, err := service.Update(context.Background(), actor, "r", shop, UpdateInput{AssistantHost: ptr(" assistant-shop2 "), Version: versionOf(store, shop)})
	if err != nil || view.AssistantHost != "assistant-shop2" {
		t.Fatalf("a plain service name must be kept: %+v %v", view, err)
	}
	view, err = service.Update(context.Background(), actor, "r", shop, UpdateInput{AssistantHost: ptr(""), Version: versionOf(store, shop)})
	if err != nil || view.AssistantHost != "" {
		t.Fatalf("the host may be cleared: %+v %v", view, err)
	}
}
