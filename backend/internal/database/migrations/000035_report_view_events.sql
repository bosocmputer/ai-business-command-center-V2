-- Who opened which card or report, and when. It exists to answer "is anyone
-- actually using this" before more is built. It holds internal recipient ids
-- only, never LINE user ids, names or report values, and is removed with the
-- recipient or tenant and after one year.
create table report_view_events (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null references line_recipients(id) on delete cascade,
  kind text not null check (kind in ('CARD_OPEN', 'REPORT_VIEW', 'OVERVIEW_VIEW', 'REFRESH_REQUEST')),
  report_key text,
  delivery_id uuid,
  occurred_at timestamptz not null,
  expires_at timestamptz not null
);
create index report_view_events_tenant_time_idx on report_view_events (tenant_id, occurred_at desc);
create index report_view_events_recipient_time_idx on report_view_events (recipient_id, occurred_at desc);
create index report_view_events_expiry_idx on report_view_events (expires_at);
