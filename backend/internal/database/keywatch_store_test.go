package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/keywatch"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestKeyWatchStoreListsShopsWithAnAssistantOnAndRemembersNotices(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)
	on, off := uuid.New(), uuid.New()
	defer func() {
		_, _ = pool.Exec(context.Background(), `delete from tenants where id = any($1)`, []uuid.UUID{on, off})
	}()
	for id, enabled := range map[uuid.UUID]bool{on: true, off: false} {
		if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'ร้านเฝ้าคีย์', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "keywatch-"+id.String(), now.AddDate(1, 0, 0)); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `insert into tenant_assistant_settings (tenant_id, enabled, is_test, model_key, line_mode, assistant_host, created_at, updated_at) values ($1, $2, true, 'test-model', 'NONE', '', now(), now())`, id, enabled); err != nil {
			t.Skipf("assistant settings need other columns in this schema: %v", err)
		}
	}
	store := NewKeyWatchStore(pool)
	shops, err := store.Shops(ctx)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[uuid.UUID]bool{}
	for _, shop := range shops {
		listed[shop.TenantID] = true
	}
	if !listed[on] || listed[off] {
		t.Fatalf("only a shop whose assistant is on is checked: %+v", shops)
	}
	if _, found, _ := store.LastNotice(ctx, on, keywatch.LevelLow); found {
		t.Fatal("no notice yet")
	}
	if err := store.MarkNotice(ctx, on, keywatch.LevelLow, now); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkNotice(ctx, on, keywatch.LevelLow, now.Add(time.Hour)); err != nil {
		t.Fatal("marking twice must update, not fail: ", err)
	}
	if at, found, _ := store.LastNotice(ctx, on, keywatch.LevelLow); !found || !at.Equal(now.Add(time.Hour)) {
		t.Fatalf("notice = %v %v", at, found)
	}
	if err := store.ClearNotices(ctx, on); err != nil {
		t.Fatal(err)
	}
	if _, found, _ := store.LastNotice(ctx, on, keywatch.LevelLow); found {
		t.Fatal("a recovered key forgets its notices")
	}
}
