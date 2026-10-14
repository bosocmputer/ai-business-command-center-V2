-- One row per owner and shop and local day a weekly summary went out, so a restarted worker never sends the same summary twice.
create table owner_weekly_summaries (
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null,
  sent_on date not null,
  created_at timestamptz not null default now(),
  primary key (tenant_id, recipient_id, sent_on)
);
