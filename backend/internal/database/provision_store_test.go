package database

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/provision"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProvisionStoreKeepsTheSealedTokenOnlyUntilTheServerHasIt(t *testing.T) {
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
	box, err := secret.NewBox(bytes.Repeat([]byte{5}, 32), "key-1", bytes.NewReader(bytes.Repeat([]byte{3}, 4096)))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 5, 0, 0, 0, time.UTC)
	tenantID, other := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenantID, other} {
		if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Instance Shop', 'Asia/Bangkok', 'ACTIVE', $3)`, id, "inst-"+id.String()[:8], now.AddDate(1, 0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	store := NewProvisionStore(pool)
	service := provision.NewService(store, box, nil, nil, func() time.Time { return now })
	name := "t" + tenantID.String()[:8]

	sealed, err := box.Encrypt([]byte("abcc_TOKEN_VALUE"), []byte(tenantID.String()+":provision:token"))
	if err != nil {
		t.Fatal(err)
	}
	row := provision.Stored{Instance: provision.Instance{TenantID: tenantID, Name: name, Desired: provision.DesiredRunning, Status: provision.StatusPending}, Token: &sealed}
	if err := store.Upsert(ctx, []byte("actor"), "r1", row, now); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, tenantID)
	if err != nil || got.Status != provision.StatusPending || !got.TokenWaiting || got.Host != "assistant-"+name {
		t.Fatalf("row = %+v err = %v", got, err)
	}
	var plain []byte
	if err := pool.QueryRow(ctx, `select token_ciphertext from assistant_instances where tenant_id = $1`, tenantID).Scan(&plain); err != nil || bytes.Contains(plain, []byte("TOKEN_VALUE")) {
		t.Fatalf("the token must be stored sealed: %v", err)
	}
	token, err := service.Token(ctx, name)
	if err != nil || token != "abcc_TOKEN_VALUE" {
		t.Fatalf("token = %q err = %v", token, err)
	}
	if _, err := service.Token(ctx, "t"+other.String()[:8]); !errors.Is(err, provision.ErrNotFound) {
		t.Fatalf("an instance that does not exist: %v", err)
	}

	work, err := store.Pending(ctx)
	found := false
	for _, item := range work {
		found = found || (item.Name == name && item.Action == "start")
	}
	if err != nil || !found {
		t.Fatalf("work = %+v err = %v", work, err)
	}
	if err := service.Report(ctx, name, provision.StatusCreating, ""); err != nil {
		t.Fatal(err)
	}
	if err := service.Report(ctx, name, provision.StatusRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := service.Ack(ctx, name); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(ctx, tenantID)
	if got.Status != provision.StatusRunning || got.TokenWaiting || got.ReportedAt == nil {
		t.Fatalf("after the server is done: %+v", got)
	}
	if _, err := service.Token(ctx, name); !errors.Is(err, provision.ErrNoToken) {
		t.Fatalf("no token may remain: %v", err)
	}
	var count int
	_ = pool.QueryRow(ctx, `select count(*) from audit_logs where tenant_id = $1 and action = 'ASSISTANT_INSTANCE_REQUESTED'`, tenantID).Scan(&count)
	if count != 1 {
		t.Fatalf("the request must be audited once, got %d", count)
	}

	// Stopping clears any token still waiting and turns into work for the server.
	if err := store.SetDesired(ctx, []byte("actor"), "r2", tenantID, provision.DesiredStopped, provision.StatusStopping, now); err != nil {
		t.Fatal(err)
	}
	work, _ = store.Pending(ctx)
	stop := false
	for _, item := range work {
		stop = stop || (item.Name == name && item.Action == "stop")
	}
	if !stop {
		t.Fatalf("a stop is work for the server: %+v", work)
	}
	if err := store.SetDesired(ctx, nil, "r3", uuid.New(), provision.DesiredStopped, provision.StatusStopped, now); !errors.Is(err, provision.ErrNotFound) {
		t.Fatalf("unknown shop: %v", err)
	}

	// Adopting records a hand-made assistant as running, with no token. The name is unique.
	if err := store.Adopt(ctx, "adopted-"+other.String()[:6], other, now); err != nil {
		t.Fatal(err)
	}
	adopted, err := store.Get(ctx, other)
	if err != nil || adopted.Status != provision.StatusRunning || adopted.TokenWaiting {
		t.Fatalf("adopted = %+v %v", adopted, err)
	}
	if err := store.Adopt(ctx, name, other, now); err == nil {
		t.Fatal("two shops cannot hold the same instance name")
	}
}
