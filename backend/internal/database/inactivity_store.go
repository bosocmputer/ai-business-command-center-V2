package database

import (
	"context"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/inactivity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InactivityStore finds the owners the operator should hear about. Only people the shop has invited to talk to the assistant are watched
// (the "AI chat" switch), so staff who only receive cards do not fill the operator's chat.
// A shop's own choices (tenant_engagement_settings) win over the worker's: a shop whose admin switched the notice off is never listed, and one
// that chose its own number of quiet days is judged by it. A shop with nothing chosen uses the worker's quiet period.
type InactivityStore struct{ pool *pgxpool.Pool }

func NewInactivityStore(pool *pgxpool.Pool) *InactivityStore { return &InactivityStore{pool: pool} }

// inactivityLookback: someone who last used the assistant longer ago than this is not "stopped", they are long gone.
const inactivityLookback = 90 * 24 * time.Hour

func (store *InactivityStore) Candidates(ctx context.Context, now time.Time, quiet, never time.Duration) ([]inactivity.Candidate, error) {
	rows, err := store.pool.Query(ctx, `
		with last_use as (
		  select tenant_id, recipient_id, max(occurred_at) as last_at from report_view_events group by 1, 2
		), watched as (
		  select m.tenant_id, m.recipient_id, t.name as shop, greatest(m.created_at, coalesce(m.ai_chat_updated_at, m.created_at)) as joined,
		         coalesce(e.inactivity_quiet_days::double precision * 86400, $2::double precision) as quiet_secs
		  from tenant_memberships m
		  join line_recipients r on r.id = m.recipient_id and r.status = 'ACTIVE'
		  join tenants t on t.id = m.tenant_id and t.status = 'ACTIVE' and t.access_ends_at > $1
		  left join tenant_engagement_settings e on e.tenant_id = m.tenant_id
		  where m.status = 'ACTIVE' and m.ai_chat_enabled and coalesce(e.inactivity_notice_enabled, true)
		)
		select w.tenant_id, w.recipient_id, w.shop, 'INACTIVE', u.last_at, w.joined
		from watched w
		join last_use u on u.tenant_id = w.tenant_id and u.recipient_id = w.recipient_id
		left join owner_inactivity_notices n on n.tenant_id = w.tenant_id and n.recipient_id = w.recipient_id and n.kind = 'INACTIVE'
		where u.last_at < $1::timestamptz - make_interval(secs => w.quiet_secs)
		  and u.last_at > $1::timestamptz - make_interval(secs => $4)
		  and (n.notified_at is null or n.notified_at < u.last_at)
		union all
		select w.tenant_id, w.recipient_id, w.shop, 'NEVER_USED', null::timestamptz, w.joined
		from watched w
		left join last_use u on u.tenant_id = w.tenant_id and u.recipient_id = w.recipient_id
		left join owner_inactivity_notices n on n.tenant_id = w.tenant_id and n.recipient_id = w.recipient_id and n.kind = 'NEVER_USED'
		where u.last_at is null and n.notified_at is null and w.joined < $1::timestamptz - make_interval(secs => $3)
		order by 3, 5 nulls last`, now, quiet.Seconds(), never.Seconds(), inactivityLookback.Seconds())
	if err != nil {
		return nil, fmt.Errorf("list inactive owners: %w", err)
	}
	defer rows.Close()
	candidates := make([]inactivity.Candidate, 0)
	for rows.Next() {
		var candidate inactivity.Candidate
		var kind string
		var tenantID, recipientID uuid.UUID
		if err := rows.Scan(&tenantID, &recipientID, &candidate.Shop, &kind, &candidate.LastActivity, &candidate.Joined); err != nil {
			return nil, fmt.Errorf("read inactive owner: %w", err)
		}
		candidate.TenantID, candidate.RecipientID, candidate.Kind = tenantID, recipientID, inactivity.Kind(kind)
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (store *InactivityStore) MarkNotified(ctx context.Context, candidate inactivity.Candidate, now time.Time) error {
	_, err := store.pool.Exec(ctx, `
		insert into owner_inactivity_notices (tenant_id, recipient_id, kind, notified_at) values ($1, $2, $3, $4)
		on conflict (tenant_id, recipient_id, kind) do update set notified_at = excluded.notified_at`,
		candidate.TenantID, candidate.RecipientID, string(candidate.Kind), now)
	if err != nil {
		return fmt.Errorf("remember inactivity notice: %w", err)
	}
	return nil
}
