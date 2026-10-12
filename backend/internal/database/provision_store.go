package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/provision"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProvisionStore keeps what the server should run for each shop's assistant (see package provision).
type ProvisionStore struct{ pool *pgxpool.Pool }

func NewProvisionStore(pool *pgxpool.Pool) *ProvisionStore { return &ProvisionStore{pool: pool} }

const provisionSelect = `
	select tenant_id, instance, desired, status, status_code, recipient_id, token_ciphertext, token_nonce,
	       coalesce(encryption_key_id, ''), updated_at, reported_at
	from assistant_instances`

func scanProvision(row pgx.Row) (provision.Stored, error) {
	var stored provision.Stored
	var status string
	var ciphertext, nonce []byte
	var keyID string
	if err := row.Scan(&stored.TenantID, &stored.Name, &stored.Desired, &status, &stored.StatusCode, &stored.RecipientID, &ciphertext, &nonce, &keyID, &stored.UpdatedAt, &stored.ReportedAt); err != nil {
		return provision.Stored{}, err
	}
	stored.Status = provision.Status(status)
	stored.Host = provision.HostFor(stored.Name)
	stored.Token = sealedOrNil(keyID, ciphertext, nonce)
	stored.TokenWaiting = stored.Token != nil
	return stored, nil
}

func (store *ProvisionStore) Get(ctx context.Context, tenantID uuid.UUID) (provision.Stored, error) {
	stored, err := scanProvision(store.pool.QueryRow(ctx, provisionSelect+` where tenant_id = $1`, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return provision.Stored{}, provision.ErrNotFound
	}
	if err != nil {
		return provision.Stored{}, fmt.Errorf("get assistant instance: %w", err)
	}
	return stored, nil
}

func (store *ProvisionStore) GetByName(ctx context.Context, name string) (provision.Stored, error) {
	stored, err := scanProvision(store.pool.QueryRow(ctx, provisionSelect+` where instance = $1`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return provision.Stored{}, provision.ErrNotFound
	}
	if err != nil {
		return provision.Stored{}, fmt.Errorf("get assistant instance: %w", err)
	}
	return stored, nil
}

func (store *ProvisionStore) Upsert(ctx context.Context, actorHash []byte, requestID string, row provision.Stored, now time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin assistant instance write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ciphertext, nonce []byte
	var keyID *string
	if row.Token != nil {
		ciphertext, nonce, keyID = row.Token.Ciphertext, row.Token.Nonce, &row.Token.KeyID
	}
	_, err = tx.Exec(ctx, `
		insert into assistant_instances (tenant_id, instance, desired, status, status_code, recipient_id, token_ciphertext, token_nonce, encryption_key_id, created_at, updated_at)
		values ($1, $2, $3, $4, '', $5, $6, $7, $8, $9, $9)
		on conflict (tenant_id) do update
		set desired = excluded.desired, status = excluded.status, status_code = '', recipient_id = excluded.recipient_id,
		    token_ciphertext = excluded.token_ciphertext, token_nonce = excluded.token_nonce, encryption_key_id = excluded.encryption_key_id,
		    updated_at = excluded.updated_at, reported_at = null`,
		row.TenantID, row.Name, row.Desired, string(row.Status), row.RecipientID, ciphertext, nonce, keyID, now)
	if err != nil {
		return fmt.Errorf("write assistant instance: %w", err)
	}
	after, _ := json.Marshal(map[string]any{"instance": row.Name, "desired": row.Desired, "status": string(row.Status)})
	if err := insertAudit(ctx, tx, row.TenantID, actorHash, "ASSISTANT_INSTANCE_REQUESTED", "ASSISTANT_INSTANCE", row.TenantID.String(), requestID, nil, after, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *ProvisionStore) SetDesired(ctx context.Context, actorHash []byte, requestID string, tenantID uuid.UUID, desired string, status provision.Status, now time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin assistant instance write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		update assistant_instances set desired = $2, status = $3, status_code = '', updated_at = $4,
		  token_ciphertext = case when $2 = 'STOPPED' then null else token_ciphertext end,
		  token_nonce = case when $2 = 'STOPPED' then null else token_nonce end
		where tenant_id = $1`, tenantID, desired, string(status), now)
	if err != nil {
		return fmt.Errorf("update assistant instance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return provision.ErrNotFound
	}
	after, _ := json.Marshal(map[string]any{"desired": desired, "status": string(status)})
	if err := insertAudit(ctx, tx, tenantID, actorHash, "ASSISTANT_INSTANCE_CHANGED", "ASSISTANT_INSTANCE", tenantID.String(), requestID, nil, after, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (store *ProvisionStore) Pending(ctx context.Context) ([]provision.Work, error) {
	rows, err := store.pool.Query(ctx, `
		select instance, tenant_id,
		       case when desired = 'RUNNING' then 'start' else 'stop' end
		from assistant_instances
		where (desired = 'RUNNING' and status in ('PENDING', 'CREATING')) or (desired = 'STOPPED' and status = 'STOPPING')
		order by updated_at, instance`)
	if err != nil {
		return nil, fmt.Errorf("list pending assistant work: %w", err)
	}
	defer rows.Close()
	var work []provision.Work
	for rows.Next() {
		var item provision.Work
		if err := rows.Scan(&item.Name, &item.TenantID, &item.Action); err != nil {
			return nil, fmt.Errorf("scan assistant work: %w", err)
		}
		work = append(work, item)
	}
	return work, rows.Err()
}

func (store *ProvisionStore) Report(ctx context.Context, name string, status provision.Status, code string, now time.Time) error {
	tag, err := store.pool.Exec(ctx, `
		update assistant_instances set status = $2, status_code = $3, reported_at = $4, updated_at = $4
		where instance = $1`, name, string(status), code, now)
	if err != nil {
		return fmt.Errorf("report assistant instance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return provision.ErrNotFound
	}
	return nil
}

func (store *ProvisionStore) ClearToken(ctx context.Context, name string, now time.Time) error {
	_, err := store.pool.Exec(ctx, `update assistant_instances set token_ciphertext = null, token_nonce = null, updated_at = $2 where instance = $1`, name, now)
	if err != nil {
		return fmt.Errorf("clear assistant token: %w", err)
	}
	return nil
}

func (store *ProvisionStore) Adopt(ctx context.Context, name string, tenantID uuid.UUID, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		insert into assistant_instances (tenant_id, instance, desired, status, reported_at, created_at, updated_at)
		values ($1, $2, 'RUNNING', 'RUNNING', $3, $3, $3)
		on conflict (tenant_id) do update set instance = excluded.instance, desired = 'RUNNING', status = 'RUNNING', status_code = '', reported_at = excluded.reported_at, updated_at = excluded.updated_at`,
		tenantID, name, now)
	if err != nil {
		return fmt.Errorf("adopt assistant instance: %w", err)
	}
	return nil
}
