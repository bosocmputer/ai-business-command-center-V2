package database

import (
	"context"
	"fmt"
	"time"

	"github.com/bosocmputer/nextstep-dashboard-backend/internal/weekly"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WeeklyStore finds the owners whose weekly summary is due. Only people the shop invited to talk to the assistant are listed, in shops whose
// admin switched the summary on (tenant_engagement_settings), on the chosen weekday from the chosen hour of the shop's own day.
type WeeklyStore struct{ pool *pgxpool.Pool }

func NewWeeklyStore(pool *pgxpool.Pool) *WeeklyStore { return &WeeklyStore{pool: pool} }

func (store *WeeklyStore) Due(ctx context.Context, now time.Time) ([]weekly.Due, error) {
	rows, err := store.pool.Query(ctx, `
		with local as (
		  select e.tenant_id, t.name as shop, e.weekly_summary_weekday as weekday, e.weekly_summary_hour as hour,
		         ($1::timestamptz at time zone t.timezone) as local_now
		  from tenant_engagement_settings e
		  join tenants t on t.id = e.tenant_id and t.status = 'ACTIVE' and t.access_ends_at > $1
		  where e.weekly_summary_enabled
		), due as (
		  select l.tenant_id, l.shop, m.recipient_id, l.local_now::date as local_day
		  from local l
		  join tenant_memberships m on m.tenant_id = l.tenant_id and m.status = 'ACTIVE' and m.ai_chat_enabled
		  join line_recipients r on r.id = m.recipient_id and r.status = 'ACTIVE'
		  where extract(isodow from l.local_now) = l.weekday and extract(hour from l.local_now) >= l.hour
		    and not exists (select 1 from owner_weekly_summaries w where w.tenant_id = l.tenant_id and w.recipient_id = m.recipient_id and w.sent_on = l.local_now::date)
		)
		select d.tenant_id, d.recipient_id, d.shop, to_char(d.local_day, 'YYYY-MM-DD'),
		  (select count(*) from agent_calls c where c.tenant_id = d.tenant_id and c.recipient_id = d.recipient_id
		     and c.tool in ('get_report', 'compare') and c.outcome = 'OK' and c.created_at > $1::timestamptz - interval '7 days'),
		  (select count(*) from report_view_events v where v.tenant_id = d.tenant_id and v.recipient_id = d.recipient_id
		     and v.kind in ('CARD_OPEN', 'REPORT_VIEW', 'OVERVIEW_VIEW', 'LINE_MENU') and v.occurred_at > $1::timestamptz - interval '7 days'),
		  (select count(*) from agent_alert_events a where a.tenant_id = d.tenant_id and a.recipient_id = d.recipient_id
		     and a.status = 'SENT' and a.sent_at > $1::timestamptz - interval '7 days')
		from due d order by d.shop, d.recipient_id`, now)
	if err != nil {
		return nil, fmt.Errorf("list weekly summaries: %w", err)
	}
	defer rows.Close()
	list := make([]weekly.Due, 0)
	for rows.Next() {
		var due weekly.Due
		if err := rows.Scan(&due.TenantID, &due.RecipientID, &due.Shop, &due.LocalDay, &due.Fetched, &due.Opened, &due.Alerts); err != nil {
			return nil, fmt.Errorf("read weekly summary: %w", err)
		}
		list = append(list, due)
	}
	return list, rows.Err()
}

func (store *WeeklyStore) Claim(ctx context.Context, due weekly.Due, now time.Time) (bool, error) {
	tag, err := store.pool.Exec(ctx, `
		insert into owner_weekly_summaries (tenant_id, recipient_id, sent_on, created_at) values ($1, $2, $3::date, $4)
		on conflict do nothing`, due.TenantID, due.RecipientID, due.LocalDay, now)
	if err != nil {
		return false, fmt.Errorf("claim weekly summary: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (store *WeeklyStore) Release(ctx context.Context, due weekly.Due) error {
	if _, err := store.pool.Exec(ctx, `delete from owner_weekly_summaries where tenant_id = $1 and recipient_id = $2 and sent_on = $3::date`, due.TenantID, due.RecipientID, due.LocalDay); err != nil {
		return fmt.Errorf("release weekly summary: %w", err)
	}
	return nil
}
