package database

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAssistantSettingsStoreKeepsSecretsSealedVersionsAndAuditsWithoutValues(t *testing.T) {
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
	now := time.Date(2026, 10, 9, 5, 0, 0, 0, time.UTC)
	tenantID := uuid.New()
	if _, err := pool.Exec(ctx, `insert into tenants (id, slug, name, timezone, status, access_ends_at) values ($1, $2, 'Assistant Shop', 'Asia/Bangkok', 'ACTIVE', $3)`, tenantID, "assistant-"+tenantID.String()[:8], now.AddDate(0, 1, 0)); err != nil {
		t.Fatal(err)
	}
	service := assistantcfg.NewService(NewAssistantSettingsStore(pool), box, func() time.Time { return now })
	actor := []byte("actor")
	const key = "sk-or-v1-0123456789abcdefghijklmnopqrstuvwxyz"

	view, err := service.SetSecret(ctx, actor, "req-1", tenantID, assistantcfg.FieldOpenRouterKey, key)
	if err != nil || !view.Secrets["openrouterKey"].IsSet || view.Secrets["openrouterKey"].Last4 != "wxyz" {
		t.Fatalf("SetSecret = %+v %v", view, err)
	}
	var stored []byte
	if err := pool.QueryRow(ctx, `select openrouter_key_ciphertext from tenant_assistant_settings where tenant_id = $1`, tenantID).Scan(&stored); err != nil || bytes.Contains(stored, []byte(key)) || len(stored) == 0 {
		t.Fatalf("the key must be stored sealed, not as text: %v", err)
	}
	if _, err := service.Update(ctx, actor, "req-2", tenantID, assistantcfg.UpdateInput{Enabled: ptrBool(true), Version: 99}); !errors.Is(err, assistantcfg.ErrVersionConflict) {
		t.Fatalf("a stale version must be refused: %v", err)
	}
	if _, err := service.Update(ctx, actor, "req-3", tenantID, assistantcfg.UpdateInput{Enabled: ptrBool(true), Version: view.Version}); err != nil {
		t.Fatal(err)
	}
	config, err := service.AgentConfig(ctx, tenantID)
	if err != nil || !config.Enabled || config.Secrets.OpenRouterKey != key || config.ShopName != "Assistant Shop" {
		t.Fatalf("AgentConfig = %+v %v", config, err)
	}
	if _, err := pool.Exec(ctx, `update tenants set access_ends_at = $2 where id = $1`, tenantID, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if expired, _ := service.AgentConfig(ctx, tenantID); expired.Enabled || expired.Reason != "EXPIRED" {
		t.Fatalf("past its end date the assistant must not run: %+v", expired)
	}
	rows, err := pool.Query(ctx, `select coalesce(after_json::text, '') from audit_logs where resource_type = 'ASSISTANT_SETTINGS' and tenant_id = $1`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var text string
		_ = rows.Scan(&text)
		count++
		if strings.Contains(text, key) || strings.Contains(text, "sk-or") {
			t.Fatalf("an audit row holds a secret: %s", text)
		}
	}
	if count < 3 {
		t.Fatalf("every change is audited, got %d rows", count)
	}
	if _, err := service.SetGlobalSecret(ctx, actor, "req-4", assistantcfg.FieldLineSecret, "0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	var global int
	if err := pool.QueryRow(ctx, `select count(*) from audit_logs where action = 'ASSISTANT_GLOBAL_SECRET_SET' and tenant_id is null`).Scan(&global); err != nil || global < 1 {
		t.Fatalf("the shared channel change is audited with no shop: %d %v", global, err)
	}
	started := now
	if err := service.RecordStatus(ctx, tenantID, assistantcfg.StatusReport{ConfigVersion: 3, ModelKey: "gemini-3.1-flash-lite", StartedAt: &started}); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordStatus(ctx, tenantID, assistantcfg.StatusReport{ConfigVersion: 4}); err != nil {
		t.Fatal(err)
	}
	var applied int64
	var startedAt *time.Time
	if err := pool.QueryRow(ctx, `select applied_config_version, started_at from tenant_assistant_status where tenant_id = $1`, tenantID).Scan(&applied, &startedAt); err != nil || applied != 4 || startedAt == nil {
		t.Fatalf("status upsert = %d %v %v", applied, startedAt, err)
	}
}

func ptrBool(value bool) *bool { return &value }
