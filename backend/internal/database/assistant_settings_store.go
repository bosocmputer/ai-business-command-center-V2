package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/assistantcfg"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/secret"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AssistantSettingsStore struct {
	pool *pgxpool.Pool
}

func NewAssistantSettingsStore(pool *pgxpool.Pool) *AssistantSettingsStore {
	return &AssistantSettingsStore{pool: pool}
}

const assistantSelect = `
	select tenant_id, enabled, is_test, model_key, line_mode,
	       openrouter_key_ciphertext, openrouter_key_nonce, coalesce(openrouter_key_last4, ''),
	       telegram_token_ciphertext, telegram_token_nonce,
	       line_secret_ciphertext, line_secret_nonce, line_token_ciphertext, line_token_nonce,
	       coalesce(encryption_key_id, ''), version, config_version, updated_at
	from tenant_assistant_settings`

func sealedOrNil(keyID string, ciphertext, nonce []byte) *secret.Sealed {
	if ciphertext == nil {
		return nil
	}
	return &secret.Sealed{KeyID: keyID, Nonce: nonce, Ciphertext: ciphertext}
}

func scanAssistant(row pgx.Row) (assistantcfg.Stored, error) {
	var stored assistantcfg.Stored
	var keyID string
	var orC, orN, tgC, tgN, lsC, lsN, ltC, ltN []byte
	err := row.Scan(&stored.TenantID, &stored.Enabled, &stored.IsTest, &stored.ModelKey, &stored.LineMode,
		&orC, &orN, &stored.OpenRouterLast4, &tgC, &tgN, &lsC, &lsN, &ltC, &ltN, &keyID, &stored.Version, &stored.ConfigVersion, &stored.UpdatedAt)
	if err != nil {
		return assistantcfg.Stored{}, err
	}
	stored.OpenRouterKey = sealedOrNil(keyID, orC, orN)
	stored.TelegramToken = sealedOrNil(keyID, tgC, tgN)
	stored.LineSecret = sealedOrNil(keyID, lsC, lsN)
	stored.LineToken = sealedOrNil(keyID, ltC, ltN)
	return stored, nil
}

func (store *AssistantSettingsStore) Get(ctx context.Context, tenantID uuid.UUID) (assistantcfg.Stored, error) {
	stored, err := scanAssistant(store.pool.QueryRow(ctx, assistantSelect+` where tenant_id = $1`, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return assistantcfg.Stored{}, assistantcfg.ErrNotConfigured
	}
	if err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("get assistant settings: %w", err)
	}
	return stored, nil
}

func (store *AssistantSettingsStore) Patch(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, patch assistantcfg.Patch, expectedVersion int, now time.Time) (assistantcfg.Stored, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("begin assistant settings update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var existing int
	err = tx.QueryRow(ctx, `select version from tenant_assistant_settings where tenant_id = $1 for update`, tenantID).Scan(&existing)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if expectedVersion != 0 {
			return assistantcfg.Stored{}, assistantcfg.ErrVersionConflict
		}
		_, err = tx.Exec(ctx, `
			insert into tenant_assistant_settings (tenant_id, enabled, is_test, model_key, line_mode, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6, $6)`, tenantID, *patch.Enabled, *patch.IsTest, *patch.ModelKey, *patch.LineMode, now)
	case err != nil:
		return assistantcfg.Stored{}, fmt.Errorf("lock assistant settings: %w", err)
	default:
		if existing != expectedVersion {
			return assistantcfg.Stored{}, assistantcfg.ErrVersionConflict
		}
		_, err = tx.Exec(ctx, `
			update tenant_assistant_settings
			set enabled = $2, is_test = $3, model_key = $4, line_mode = $5, version = version + 1, config_version = config_version + 1, updated_at = $6
			where tenant_id = $1`, tenantID, *patch.Enabled, *patch.IsTest, *patch.ModelKey, *patch.LineMode, now)
	}
	if err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("write assistant settings: %w", err)
	}
	stored, err := scanAssistant(tx.QueryRow(ctx, assistantSelect+` where tenant_id = $1`, tenantID))
	if err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("read assistant settings: %w", err)
	}
	audit, _ := json.Marshal(map[string]any{"enabled": stored.Enabled, "isTest": stored.IsTest, "modelKey": stored.ModelKey, "lineMode": stored.LineMode, "version": stored.Version})
	if err := insertAudit(ctx, tx, tenantID, actorHash, "ASSISTANT_SETTINGS_CHANGED", "ASSISTANT_SETTINGS", tenantID.String(), requestID, nil, audit, now); err != nil {
		return assistantcfg.Stored{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("commit assistant settings: %w", err)
	}
	return stored, nil
}

// secretColumns maps a field to its columns. The names are fixed text here, never taken from a request.
var secretColumns = map[assistantcfg.Field][2]string{
	assistantcfg.FieldOpenRouterKey: {"openrouter_key_ciphertext", "openrouter_key_nonce"},
	assistantcfg.FieldTelegramToken: {"telegram_token_ciphertext", "telegram_token_nonce"},
	assistantcfg.FieldLineSecret:    {"line_secret_ciphertext", "line_secret_nonce"},
	assistantcfg.FieldLineToken:     {"line_token_ciphertext", "line_token_nonce"},
}

func (store *AssistantSettingsStore) PutSecret(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, field assistantcfg.Field, sealed *secret.Sealed, last4 string, now time.Time) (assistantcfg.Stored, error) {
	columns, ok := secretColumns[field]
	if !ok {
		return assistantcfg.Stored{}, errors.New("unknown assistant secret field")
	}
	var ciphertext, nonce any
	keyID := ""
	if sealed != nil {
		ciphertext, nonce, keyID = sealed.Ciphertext, sealed.Nonce, sealed.KeyID
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("begin assistant secret update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `insert into tenant_assistant_settings (tenant_id, created_at, updated_at) values ($1, $2, $2) on conflict (tenant_id) do nothing`, tenantID, now); err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("create assistant settings: %w", err)
	}
	statement := fmt.Sprintf(`
		update tenant_assistant_settings
		set %s = $2, %s = $3, encryption_key_id = coalesce(nullif($4, ''), encryption_key_id),
		    version = version + 1, config_version = config_version + 1, updated_at = $5`, columns[0], columns[1])
	args := []any{tenantID, ciphertext, nonce, keyID, now}
	if field == assistantcfg.FieldOpenRouterKey {
		statement += `, openrouter_key_last4 = nullif($6, '')`
		args = append(args, last4)
	}
	if _, err := tx.Exec(ctx, statement+` where tenant_id = $1`, args...); err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("write assistant secret: %w", err)
	}
	stored, err := scanAssistant(tx.QueryRow(ctx, assistantSelect+` where tenant_id = $1`, tenantID))
	if err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("read assistant settings: %w", err)
	}
	action := "ASSISTANT_SECRET_SET"
	if sealed == nil {
		action = "ASSISTANT_SECRET_CLEARED"
	}
	audit, _ := json.Marshal(map[string]any{"field": string(field), "version": stored.Version})
	if err := insertAudit(ctx, tx, tenantID, actorHash, action, "ASSISTANT_SETTINGS", tenantID.String(), requestID, nil, audit, now); err != nil {
		return assistantcfg.Stored{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return assistantcfg.Stored{}, fmt.Errorf("commit assistant secret: %w", err)
	}
	return stored, nil
}

func (store *AssistantSettingsStore) GetGlobal(ctx context.Context) (assistantcfg.StoredGlobal, error) {
	var global assistantcfg.StoredGlobal
	var keyID string
	var lsC, lsN, ltC, ltN []byte
	err := store.pool.QueryRow(ctx, `
		select line_secret_ciphertext, line_secret_nonce, line_token_ciphertext, line_token_nonce, coalesce(encryption_key_id, ''), config_version, updated_at
		from assistant_global_settings where singleton`).Scan(&lsC, &lsN, &ltC, &ltN, &keyID, &global.ConfigVersion, &global.UpdatedAt)
	if err != nil {
		return assistantcfg.StoredGlobal{}, fmt.Errorf("get assistant global settings: %w", err)
	}
	global.LineSecret, global.LineToken = sealedOrNil(keyID, lsC, lsN), sealedOrNil(keyID, ltC, ltN)
	return global, nil
}

func (store *AssistantSettingsStore) PutGlobalSecret(ctx context.Context, actorHash []byte, requestID string, field assistantcfg.Field, sealed *secret.Sealed, now time.Time) (assistantcfg.StoredGlobal, error) {
	var columns [2]string
	switch field {
	case assistantcfg.FieldLineSecret:
		columns = [2]string{"line_secret_ciphertext", "line_secret_nonce"}
	case assistantcfg.FieldLineToken:
		columns = [2]string{"line_token_ciphertext", "line_token_nonce"}
	default:
		return assistantcfg.StoredGlobal{}, errors.New("unknown assistant global field")
	}
	var ciphertext, nonce any
	keyID := ""
	if sealed != nil {
		ciphertext, nonce, keyID = sealed.Ciphertext, sealed.Nonce, sealed.KeyID
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return assistantcfg.StoredGlobal{}, fmt.Errorf("begin assistant global update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	statement := fmt.Sprintf(`
		update assistant_global_settings
		set %s = $1, %s = $2, encryption_key_id = coalesce(nullif($3, ''), encryption_key_id), config_version = config_version + 1, updated_at = $4
		where singleton`, columns[0], columns[1])
	if _, err := tx.Exec(ctx, statement, ciphertext, nonce, keyID, now); err != nil {
		return assistantcfg.StoredGlobal{}, fmt.Errorf("write assistant global secret: %w", err)
	}
	audit, _ := json.Marshal(map[string]any{"field": string(field)})
	if _, err := tx.Exec(ctx, `
		insert into audit_logs (tenant_id, actor_type, actor_id_hash, action, resource_type, resource_id, request_id, after_json, result, created_at, expires_at)
		values (null, 'ADMIN', $1, 'ASSISTANT_GLOBAL_SECRET_SET', 'ASSISTANT_SETTINGS', 'global', $2, $3, 'SUCCESS', $4, $5)`,
		actorHash, requestID, audit, now, now.AddDate(1, 0, 0)); err != nil {
		return assistantcfg.StoredGlobal{}, fmt.Errorf("audit assistant global secret: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return assistantcfg.StoredGlobal{}, fmt.Errorf("commit assistant global secret: %w", err)
	}
	return store.GetGlobal(ctx)
}

func (store *AssistantSettingsStore) Gate(ctx context.Context, tenantID uuid.UUID, now time.Time) (assistantcfg.TenantGate, error) {
	var gate assistantcfg.TenantGate
	err := store.pool.QueryRow(ctx, `select name, status = 'ACTIVE' and access_ends_at > $2 from tenants where id = $1`, tenantID, now).Scan(&gate.Name, &gate.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return assistantcfg.TenantGate{}, nil
	}
	if err != nil {
		return assistantcfg.TenantGate{}, fmt.Errorf("read tenant gate: %w", err)
	}
	return gate, nil
}

func (store *AssistantSettingsStore) RecordStatus(ctx context.Context, tenantID uuid.UUID, status assistantcfg.Status, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		insert into tenant_assistant_status (tenant_id, applied_config_version, reported_model_key, started_at, last_seen_at, last_error_code)
		values ($1, $2, nullif($3, ''), $4, $5, nullif($6, ''))
		on conflict (tenant_id) do update
		set applied_config_version = excluded.applied_config_version, reported_model_key = excluded.reported_model_key,
		    started_at = coalesce(excluded.started_at, tenant_assistant_status.started_at), last_seen_at = excluded.last_seen_at,
		    last_error_code = excluded.last_error_code`,
		tenantID, status.AppliedConfigVersion, status.ReportedModelKey, status.StartedAt, now, status.LastErrorCode)
	if err != nil {
		return fmt.Errorf("record assistant status: %w", err)
	}
	return nil
}

func (store *AssistantSettingsStore) GetStatus(ctx context.Context, tenantID uuid.UUID) (assistantcfg.Status, error) {
	var status assistantcfg.Status
	err := store.pool.QueryRow(ctx, `
		select applied_config_version, coalesce(reported_model_key, ''), started_at, last_seen_at, coalesce(last_error_code, '')
		from tenant_assistant_status where tenant_id = $1`, tenantID).Scan(&status.AppliedConfigVersion, &status.ReportedModelKey, &status.StartedAt, &status.LastSeenAt, &status.LastErrorCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return assistantcfg.Status{}, assistantcfg.ErrNotConfigured
	}
	if err != nil {
		return assistantcfg.Status{}, fmt.Errorf("get assistant status: %w", err)
	}
	return status, nil
}
