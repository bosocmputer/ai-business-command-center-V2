-- Agent API: tokens for the owner-facing assistant and a log of what it asked for.
-- A token belongs to one recipient in one tenant and is stored only as a hash.
-- The log keeps no values, names or question text.
create table agent_tokens (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null references line_recipients(id) on delete cascade,
  token_hash bytea not null unique,
  names_visible boolean not null default false,
  status text not null default 'ACTIVE' check (status in ('ACTIVE', 'REVOKED')),
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_used_at timestamptz,
  revoked_at timestamptz,
  check ((status = 'REVOKED') = (revoked_at is not null))
);
-- One live token per recipient and tenant: issuing a new one revokes the old.
create unique index agent_tokens_one_active_idx on agent_tokens (tenant_id, recipient_id) where status = 'ACTIVE';

create table agent_calls (
  id bigserial primary key,
  token_id uuid not null references agent_tokens(id) on delete cascade,
  tenant_id uuid not null references tenants(id) on delete cascade,
  recipient_id uuid not null,
  tool text not null check (tool in ('context', 'get_report', 'compare', 'latest_delivery')),
  report_key text,
  period_from date,
  period_to date,
  outcome text not null check (outcome in ('OK', 'PREPARING', 'UNAVAILABLE', 'NO_DATA', 'INVALID_PERIOD', 'RATE_LIMITED')),
  duration_ms integer not null check (duration_ms >= 0),
  snapshot_run_id uuid,
  created_at timestamptz not null default now(),
  expires_at timestamptz not null default (now() + interval '365 days')
);
create index agent_calls_token_time_idx on agent_calls (token_id, created_at desc);
create index agent_calls_tenant_time_idx on agent_calls (tenant_id, created_at desc);
create index agent_calls_expiry_idx on agent_calls (expires_at);
