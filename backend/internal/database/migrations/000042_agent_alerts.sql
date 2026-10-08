-- Alerts the assistant sets up for a recipient, and a log of what fired. A rule is the owner's own words turned into
-- a threshold ("tell me when overdue receivables pass 500,000"); nothing is sent until the worker's alert switch is on.
-- The log keeps figures, never names.
create table agent_alert_rules (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null references line_recipients(id) on delete cascade,
  rule_key text not null check (rule_key ~ '^[a-z_]{1,40}$'),
  threshold numeric(20, 4) not null check (threshold > 0),
  enabled boolean not null default true,
  last_checked_on date,
  last_status text check (last_status in ('OK', 'NOT_READY', 'NO_ACCESS', 'ERROR')),
  last_fired_at timestamptz,
  last_fired_value numeric(20, 4),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (tenant_id, recipient_id, rule_key)
);
create index agent_alert_rules_enabled_idx on agent_alert_rules (enabled) where enabled;

create table agent_alert_events (
  id uuid primary key default gen_random_uuid(),
  rule_id uuid not null references agent_alert_rules(id) on delete cascade,
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null,
  rule_key text not null,
  fired_on date not null,
  value numeric(20, 4) not null,
  threshold numeric(20, 4) not null,
  message text not null,
  status text not null default 'PENDING' check (status in ('PENDING', 'SENT', 'FAILED', 'DRY_RUN')),
  attempts integer not null default 0 check (attempts >= 0),
  last_error text,
  sent_at timestamptz,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null default (now() + interval '365 days'),
  unique (rule_id, fired_on)
);
create index agent_alert_events_unsent_idx on agent_alert_events (created_at) where status in ('PENDING', 'FAILED');
create index agent_alert_events_expiry_idx on agent_alert_events (expires_at);
create index agent_alert_events_tenant_time_idx on agent_alert_events (tenant_id, created_at desc);

-- The call log also records the two alert tools and a request the assistant got wrong.
alter table agent_calls drop constraint agent_calls_tool_check;
alter table agent_calls add constraint agent_calls_tool_check check (tool in ('context', 'get_report', 'compare', 'latest_delivery', 'alerts', 'alert_set'));
alter table agent_calls drop constraint agent_calls_outcome_check;
alter table agent_calls add constraint agent_calls_outcome_check check (outcome in ('OK', 'PREPARING', 'UNAVAILABLE', 'NO_DATA', 'INVALID_PERIOD', 'INVALID_ALERT', 'RATE_LIMITED'));
