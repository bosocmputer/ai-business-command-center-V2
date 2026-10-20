-- Which shops the operator was already told about because their model key is nearly or fully spent, so one spell is reported once a day and
-- not every half hour. A row goes when the key recovers.
create table assistant_key_notices (
  tenant_id uuid not null references tenants(id) on delete cascade,
  level text not null check (level in ('LOW', 'UNUSABLE')),
  notified_at timestamptz not null,
  primary key (tenant_id, level)
);
