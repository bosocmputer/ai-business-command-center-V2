package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLineGateStoreListsOnlyShopsThePersonMayAskAboutAndKeepsTheirChoice(t *testing.T) {
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
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 10, 10, 5, 0, 0, 0, time.UTC)
	person, lineHash := uuid.New(), []byte("gate-hash-"+uuid.NewString())
	exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', 'ACTIVE', $3)`, person, lineHash, now)

	// shop(...) makes a shop with the given state; each flag is one reason the shop must not be offered.
	shop := func(name, status string, endsAt time.Time, membership string, chat, enabled bool, mode, host string) uuid.UUID {
		id := uuid.New()
		exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, $3, 'Asia/Bangkok', $4, $5)`, id, "gate-"+id.String()[:8], name, status, endsAt)
		exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, $3, $4)`, id, person, membership, chat)
		exec(`insert into tenant_assistant_settings (tenant_id, enabled, is_test, model_key, line_mode, assistant_host) values ($1, $2, true, 'google/gemini-3.1-flash-lite', $3, $4)`, id, enabled, mode, host)
		return id
	}
	future := now.AddDate(1, 0, 0)
	good1 := shop("A good", "ACTIVE", future, "ACTIVE", true, true, "CENTRAL", "assistant")
	good2 := shop("B good", "ACTIVE", future, "ACTIVE", true, true, "CENTRAL", "assistant-b")
	shop("expired", "ACTIVE", now.Add(-time.Hour), "ACTIVE", true, true, "CENTRAL", "assistant-c")
	shop("disabled shop", "DISABLED", future, "ACTIVE", true, true, "CENTRAL", "assistant-d")
	shop("revoked", "ACTIVE", future, "REVOKED", true, true, "CENTRAL", "assistant-e")
	shop("no chat", "ACTIVE", future, "ACTIVE", false, true, "CENTRAL", "assistant-f")
	shop("assistant off", "ACTIVE", future, "ACTIVE", true, false, "CENTRAL", "assistant-g")
	shop("own line", "ACTIVE", future, "ACTIVE", true, true, "OWN", "assistant-h")
	shop("no host", "ACTIVE", future, "ACTIVE", true, true, "CENTRAL", "")

	store := NewLineGateStore(pool)
	gotPerson, shops, err := store.Shops(ctx, lineHash, now)
	if err != nil || gotPerson != person || len(shops) != 2 || shops[0].TenantID != good1 || shops[1].TenantID != good2 || shops[1].Host != "assistant-b" {
		t.Fatalf("person=%v shops=%+v err=%v", gotPerson, shops, err)
	}
	if _, none, err := store.Shops(ctx, []byte("someone-else"), now); err != nil || len(none) != 0 {
		t.Fatalf("a stranger must get no shops: %v %v", none, err)
	}

	if _, found, err := store.Selected(ctx, person); err != nil || found {
		t.Fatalf("nothing chosen yet: %v %v", found, err)
	}
	if err := store.Select(ctx, person, good1, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Select(ctx, person, good2, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if chosen, found, err := store.Selected(ctx, person); err != nil || !found || chosen != good2 {
		t.Fatalf("the latest choice must win: %v %v %v", chosen, found, err)
	}
}
