-- How each tenant's heavy report is fetched from SML: in one query (DIRECT) or
-- split into chunks (CHUNKED). Additive; a missing row means DIRECT. The worker
-- switches DIRECT to CHUNKED on clear size signals, and admins can set it.
create table report_execution_modes (
  tenant_id uuid not null references tenants(id) on delete cascade,
  report_key text not null,
  mode text not null default 'DIRECT' check (mode in ('DIRECT', 'CHUNKED')),
  source text not null default 'DEFAULT' check (source in ('DEFAULT', 'ENV_SEED', 'MEASURED', 'AUTO_SWITCHED', 'MANUAL')),
  reason text check (reason is null or char_length(reason) <= 200),
  last_rows integer check (last_rows is null or last_rows >= 0),
  last_duration_ms bigint check (last_duration_ms is null or last_duration_ms >= 0),
  consecutive_direct_timeouts integer not null default 0 check (consecutive_direct_timeouts >= 0),
  changed_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  primary key (tenant_id, report_key)
);
