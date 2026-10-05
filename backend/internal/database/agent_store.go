package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/agent"
	"github.com/bosocmputer/nextstep-dashboard-backend/internal/report"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AgentStore keeps the assistant's tokens and its call log.
type AgentStore struct{ pool *pgxpool.Pool }

func NewAgentStore(pool *pgxpool.Pool) *AgentStore { return &AgentStore{pool: pool} }

// Authenticate finds the person and shop a token stands for, only while every condition holds: the token is
// live, the shop is active and in its access period, the recipient and their membership are active, and they
// have been given the assistant. Anything else is the same ErrUnauthorized.
func (store *AgentStore) Authenticate(ctx context.Context, tokenHash []byte, now time.Time) (agent.Principal, error) {
	var principal agent.Principal
	err := store.pool.QueryRow(ctx, `
		select a.id, a.tenant_id, a.recipient_id, a.names_visible, t.name, t.timezone
		from agent_tokens a
		join tenants t on t.id = a.tenant_id and t.status = 'ACTIVE' and t.access_ends_at > $2
		join line_recipients r on r.id = a.recipient_id and r.status = 'ACTIVE'
		join tenant_memberships m on m.tenant_id = a.tenant_id and m.recipient_id = a.recipient_id
		  and m.status = 'ACTIVE' and m.ai_chat_enabled
		where a.token_hash = $1 and a.status = 'ACTIVE' and a.expires_at > $2`, tokenHash, now).Scan(
		&principal.TokenID, &principal.TenantID, &principal.RecipientID, &principal.NamesVisible, &principal.ShopName, &principal.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.Principal{}, agent.ErrUnauthorized
	}
	if err != nil {
		return agent.Principal{}, fmt.Errorf("authenticate agent token: %w", err)
	}
	return principal, nil
}

func (store *AgentStore) TouchToken(ctx context.Context, tokenID uuid.UUID, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		update agent_tokens set last_used_at = $2::timestamptz
		where id = $1 and (last_used_at is null or last_used_at < $2::timestamptz - interval '1 minute')`, tokenID, now)
	return err
}

// FreePreparingChecks is how many "still being prepared" answers an hour a token may collect without them counting
// against the limit. The assistant waits for a report to be ready by asking every few seconds, and one wait must not
// use up the hour. Past this many, they count like any other call, so a runaway loop is still stopped.
const FreePreparingChecks = 240

// CallsSince counts the calls that were answered; calls refused for being too frequent do not count against the
// limit, so a caller that keeps asking is not locked out for ever. The first FreePreparingChecks answers of
// "preparing" do not count either.
func (store *AgentStore) CallsSince(ctx context.Context, tokenID uuid.UUID, since time.Time) (int, error) {
	var count int
	err := store.pool.QueryRow(ctx, `
		select count(*) filter (where outcome not in ('RATE_LIMITED', 'PREPARING'))
		     + greatest(0, count(*) filter (where outcome = 'PREPARING') - $3)
		from agent_calls where token_id = $1 and created_at >= $2`, tokenID, since, FreePreparingChecks).Scan(&count)
	return count, err
}

// PreparingSince counts the different reports and periods the assistant has had fetched in the window. Asking again
// for the same one while it is being fetched is not another fetch.
func (store *AgentStore) PreparingSince(ctx context.Context, tenantID uuid.UUID, since time.Time) (int, error) {
	var count int
	err := store.pool.QueryRow(ctx, `
		select count(distinct (report_key, period_from, period_to)) from agent_calls
		where tenant_id = $1 and outcome = 'PREPARING' and created_at >= $2`, tenantID, since).Scan(&count)
	return count, err
}

func (store *AgentStore) RecordCall(ctx context.Context, call agent.Call, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		insert into agent_calls (token_id, tenant_id, recipient_id, tool, report_key, period_from, period_to, outcome, duration_ms, snapshot_run_id, created_at, expires_at)
		values ($1, $2, $3, $4, nullif($5, ''), nullif($6, '')::date, nullif($7, '')::date, $8, $9, $10, $11, $12)`,
		call.TokenID, call.TenantID, call.RecipientID, call.Tool, call.ReportKey, call.PeriodFrom, call.PeriodTo,
		call.Outcome, int(call.Duration.Milliseconds()), call.SnapshotRunID, now, now.AddDate(1, 0, 0))
	return err
}

func (store *AgentStore) PermittedReports(ctx context.Context, principal agent.Principal, _ time.Time) ([]report.Key, error) {
	rows, err := store.pool.Query(ctx, `
		select report_key from recipient_report_permissions where tenant_id = $1 and recipient_id = $2 order by report_key`, principal.TenantID, principal.RecipientID)
	if err != nil {
		return nil, fmt.Errorf("list agent report permissions: %w", err)
	}
	defer rows.Close()
	keys := make([]report.Key, 0)
	for rows.Next() {
		var key report.Key
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// LatestDelivery finds the report as it was last sent to this recipient on a card.
func (store *AgentStore) LatestDelivery(ctx context.Context, principal agent.Principal, key report.Key) (agent.Delivered, error) {
	var delivered agent.Delivered
	var dashboardJSON []byte
	err := store.pool.QueryRow(ctx, `
		select rr.id, rr.dashboard_json, d.accepted_at, coalesce(rr.source_finished_at, rr.finished_at)
		from line_deliveries d
		join notification_runs nr on nr.id = d.notification_run_id and nr.tenant_id = d.tenant_id
		join notification_run_reports nrr on nrr.notification_run_id = nr.id and nrr.report_key = $3
		join report_runs rr on rr.id = nrr.report_run_id and rr.status = 'SUCCEEDED' and rr.dashboard_json <> '{}'::jsonb
		where d.tenant_id = $1 and d.recipient_id = $2 and d.status = 'ACCEPTED' and d.accepted_at is not null
		order by d.accepted_at desc limit 1`, principal.TenantID, principal.RecipientID, key).Scan(
		&delivered.RunID, &dashboardJSON, &delivered.DeliveredAt, &delivered.CollectedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.Delivered{}, agent.ErrNoData
	}
	if err != nil {
		return agent.Delivered{}, fmt.Errorf("get latest agent delivery: %w", err)
	}
	if err := json.Unmarshal(dashboardJSON, &delivered.Dashboard); err != nil {
		return agent.Delivered{}, fmt.Errorf("decode delivered dashboard: %w", err)
	}
	return delivered, nil
}

// IssueToken makes a new token for a recipient who has been given the assistant, and revokes the one they had.
func (store *AgentStore) IssueToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, namesVisible bool, tokenHash []byte, expiresAt, now time.Time) (agent.TokenInfo, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return agent.TokenInfo{}, fmt.Errorf("begin agent token issue: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var aiChat bool
	err = tx.QueryRow(ctx, `
		select m.ai_chat_enabled from tenant_memberships m
		join line_recipients r on r.id = m.recipient_id and r.status = 'ACTIVE'
		where m.tenant_id = $1 and m.recipient_id = $2 and m.status = 'ACTIVE'
		for update of m`, tenantID, recipientID).Scan(&aiChat)
	if errors.Is(err, pgx.ErrNoRows) {
		return agent.TokenInfo{}, agent.ErrRecipientNotFound
	}
	if err != nil {
		return agent.TokenInfo{}, fmt.Errorf("lock recipient membership: %w", err)
	}
	if !aiChat {
		return agent.TokenInfo{}, agent.ErrAIChatDisabled
	}
	if _, err := tx.Exec(ctx, `
		update agent_tokens set status = 'REVOKED', revoked_at = $3
		where tenant_id = $1 and recipient_id = $2 and status = 'ACTIVE'`, tenantID, recipientID, now); err != nil {
		return agent.TokenInfo{}, fmt.Errorf("revoke previous agent token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_tokens (tenant_id, recipient_id, token_hash, names_visible, created_at, expires_at)
		values ($1, $2, $3, $4, $5, $6)`, tenantID, recipientID, tokenHash, namesVisible, now, expiresAt); err != nil {
		return agent.TokenInfo{}, fmt.Errorf("insert agent token: %w", err)
	}
	afterJSON, _ := json.Marshal(map[string]any{"namesVisible": namesVisible, "expiresAt": expiresAt.UTC().Format(time.RFC3339)})
	if err := insertAudit(ctx, tx, tenantID, actorHash, "AGENT_TOKEN_ISSUED", "LINE_RECIPIENT", recipientID.String(), requestID, nil, afterJSON, now); err != nil {
		return agent.TokenInfo{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return agent.TokenInfo{}, fmt.Errorf("commit agent token issue: %w", err)
	}
	return store.TokenInfo(ctx, tenantID, recipientID, now)
}

// RevokeToken ends the recipient's live token. Revoking when there is none does nothing.
func (store *AgentStore) RevokeToken(ctx context.Context, actorHash []byte, requestID string, tenantID, recipientID uuid.UUID, now time.Time) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin agent token revoke: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		update agent_tokens set status = 'REVOKED', revoked_at = $3
		where tenant_id = $1 and recipient_id = $2 and status = 'ACTIVE'`, tenantID, recipientID, now)
	if err != nil {
		return fmt.Errorf("revoke agent token: %w", err)
	}
	if tag.RowsAffected() > 0 {
		if err := insertAudit(ctx, tx, tenantID, actorHash, "AGENT_TOKEN_REVOKED", "LINE_RECIPIENT", recipientID.String(), requestID, nil, nil, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// TokenInfo describes the recipient's live token, if any, and how much it was used in the last day.
func (store *AgentStore) TokenInfo(ctx context.Context, tenantID, recipientID uuid.UUID, now time.Time) (agent.TokenInfo, error) {
	info := agent.TokenInfo{Status: "NONE"}
	var id uuid.UUID
	var createdAt, expiresAt time.Time
	var lastUsedAt *time.Time
	err := store.pool.QueryRow(ctx, `
		select id, names_visible, created_at, expires_at, last_used_at from agent_tokens
		where tenant_id = $1 and recipient_id = $2 and status = 'ACTIVE'`, tenantID, recipientID).Scan(&id, &info.NamesVisible, &createdAt, &expiresAt, &lastUsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return info, nil
	}
	if err != nil {
		return agent.TokenInfo{}, fmt.Errorf("get agent token: %w", err)
	}
	info.Status = "ACTIVE"
	if !expiresAt.After(now) {
		info.Status = "EXPIRED"
	}
	info.CreatedAt, info.ExpiresAt, info.LastUsedAt = &createdAt, &expiresAt, lastUsedAt
	if err := store.pool.QueryRow(ctx, `select count(*) from agent_calls where token_id = $1 and created_at >= $2`, id, now.Add(-24*time.Hour)).Scan(&info.Calls24h); err != nil {
		return agent.TokenInfo{}, fmt.Errorf("count agent calls: %w", err)
	}
	return info, nil
}
