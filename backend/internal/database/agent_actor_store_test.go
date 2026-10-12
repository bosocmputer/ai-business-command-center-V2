package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResolveActorFindsOnlyAnActiveSwitchedOnMemberOfThatShop(t *testing.T) {
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
	now := time.Date(2026, 10, 12, 5, 0, 0, 0, time.UTC)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	shopA, shopB := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{shopA, shopB} {
		exec(`insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Actor Shop', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "actor-"+id.String()[:8], now.AddDate(1, 0, 0))
	}
	person := func(status, membership string, chat bool, tenant uuid.UUID) []byte {
		id, hash := uuid.New(), []byte("actor-hash-"+uuid.NewString())
		exec(`insert into line_recipients (id, line_user_id_hash, line_user_id_ciphertext, line_user_id_nonce, encryption_key_id, status, verified_at) values ($1, $2, '\x01', '\x02', 'test', $3, $4)`, id, hash, status, now)
		exec(`insert into tenant_memberships (tenant_id, recipient_id, status, ai_chat_enabled) values ($1, $2, $3, $4)`, tenant, id, membership, chat)
		return hash
	}
	good := person("ACTIVE", "ACTIVE", true, shopA)
	notOn := person("ACTIVE", "ACTIVE", false, shopA)
	revokedMember := person("ACTIVE", "REVOKED", true, shopA)
	revokedPerson := person("REVOKED", "ACTIVE", true, shopA)
	elsewhere := person("ACTIVE", "ACTIVE", true, shopB)

	store := NewAgentStore(pool)
	if id, err := store.ResolveActor(ctx, shopA, good); err != nil || id == uuid.Nil {
		t.Fatalf("an active, switched-on member must be found: %v %v", id, err)
	}
	for name, hash := range map[string][]byte{"chat off": notOn, "membership revoked": revokedMember, "person revoked": revokedPerson, "member of another shop": elsewhere, "unknown": []byte("nobody")} {
		if _, err := store.ResolveActor(ctx, shopA, hash); !errors.Is(err, agent.ErrUnauthorized) {
			t.Fatalf("%s must be refused like a bad token: %v", name, err)
		}
	}
	if id, err := store.ResolveActor(ctx, shopB, elsewhere); err != nil || id == uuid.Nil {
		t.Fatalf("the same person is found in their own shop: %v %v", id, err)
	}
}
