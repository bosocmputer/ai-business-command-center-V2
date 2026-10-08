package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/onboarding"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OnboardingStore reads a shop's configuration for the readiness check. Counts only.
type OnboardingStore struct{ pool *pgxpool.Pool }

func NewOnboardingStore(pool *pgxpool.Pool) *OnboardingStore { return &OnboardingStore{pool: pool} }

func (store *OnboardingStore) Facts(ctx context.Context, tenantID uuid.UUID, now time.Time) (onboarding.Facts, error) {
	facts := onboarding.Facts{MasterCopies: map[string]onboarding.MasterState{}}
	var status string
	err := store.pool.QueryRow(ctx, `
		select t.status, t.access_ends_at, coalesce(t.timezone, ''), coalesce(c.readiness_status, 'UNTESTED'), c.last_tested_at
		from tenants t left join tenant_sml_connections c on c.tenant_id = t.id where t.id = $1`, tenantID).
		Scan(&status, &facts.AccessEndsAt, &facts.Timezone, &facts.SMLReadiness, &facts.SMLTestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return facts, nil
	}
	if err != nil {
		return facts, fmt.Errorf("read tenant: %w", err)
	}
	facts.Found, facts.Active = true, status == "ACTIVE"
	if err := store.pool.QueryRow(ctx, `
		select
		  count(*) filter (where m.status = 'ACTIVE' and r.status = 'ACTIVE'),
		  count(*) filter (where m.status = 'ACTIVE' and r.status = 'ACTIVE' and r.verified_at is not null),
		  count(*) filter (where m.status = 'ACTIVE' and r.status = 'ACTIVE' and not exists (
		    select 1 from recipient_report_permissions p where p.tenant_id = m.tenant_id and p.recipient_id = m.recipient_id)),
		  count(*) filter (where m.status = 'ACTIVE' and r.status = 'ACTIVE' and m.ai_chat_enabled)
		from tenant_memberships m join line_recipients r on r.id = m.recipient_id where m.tenant_id = $1`, tenantID).
		Scan(&facts.ActiveRecipients, &facts.VerifiedRecipients, &facts.RecipientsWithoutReports, &facts.AssistantMembers); err != nil {
		return facts, fmt.Errorf("count recipients: %w", err)
	}
	if err := store.pool.QueryRow(ctx, `
		select (select count(*) from notification_schedules where tenant_id = $1 and status = 'ACTIVE'),
		       (select count(*) from agent_tokens where tenant_id = $1 and status = 'ACTIVE' and expires_at > $2)`, tenantID, now).
		Scan(&facts.ActiveSchedules, &facts.AssistantTokens); err != nil {
		return facts, fmt.Errorf("count schedules and tokens: %w", err)
	}
	rows, err := store.pool.Query(ctx, `
		select s.kind, s.synced_at, s.status, (select count(*) from master_items i where i.tenant_id = s.tenant_id and i.kind = s.kind)
		from master_sync s where s.tenant_id = $1`, tenantID)
	if err != nil {
		return facts, fmt.Errorf("read master copies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var state onboarding.MasterState
		if err := rows.Scan(&kind, &state.SyncedAt, &state.Status, &state.Rows); err != nil {
			return facts, err
		}
		facts.MasterCopies[kind] = state
	}
	return facts, rows.Err()
}
