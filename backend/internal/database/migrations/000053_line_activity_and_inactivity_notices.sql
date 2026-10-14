-- Two things for noticing an owner who stops using the assistant:
-- 1. what a person does in LINE counts as use, so the front gate records a message or a menu button press as a view event (still internal
--    recipient ids only, never LINE user ids, words or figures);
-- 2. which owners the operator was already told about, so each quiet spell is reported once.
alter table report_view_events drop constraint report_view_events_kind_check;
alter table report_view_events add constraint report_view_events_kind_check
  check (kind in ('CARD_OPEN', 'REPORT_VIEW', 'OVERVIEW_VIEW', 'REFRESH_REQUEST', 'LINE_MESSAGE', 'LINE_MENU'));

create table owner_inactivity_notices (
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null references line_recipients(id) on delete cascade,
  kind text not null check (kind in ('INACTIVE', 'NEVER_USED')),
  notified_at timestamptz not null,
  primary key (tenant_id, recipient_id, kind)
);
