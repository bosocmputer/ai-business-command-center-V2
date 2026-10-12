-- One row per shop whose assistant is created from the admin page instead of by hand on the server. The row says what the server should
-- run (desired) and what it reports it runs (status). The assistant token the new assistant needs is kept sealed here only until the
-- host script has taken it (then it is cleared), so no one has to see or copy it.
create table assistant_instances (
  tenant_id uuid primary key references tenants(id) on delete cascade,
  instance text not null unique check (instance ~ '^[a-z0-9]([a-z0-9-]{0,22}[a-z0-9])?$'),
  desired text not null default 'RUNNING' check (desired in ('RUNNING', 'STOPPED')),
  status text not null default 'PENDING' check (status in ('PENDING', 'CREATING', 'RUNNING', 'STOPPING', 'STOPPED', 'ERROR')),
  status_code text not null default '' check (status_code ~ '^[A-Z0-9_]{0,40}$'),
  recipient_id uuid references line_recipients(id) on delete set null,
  token_ciphertext bytea,
  token_nonce bytea,
  encryption_key_id text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  reported_at timestamptz,
  check ((token_ciphertext is null) = (token_nonce is null))
);
