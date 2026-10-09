-- Per-shop settings of the owner-facing assistant, kept here instead of in files on the server. Secrets are sealed with the same box
-- as the SML credentials (AES-GCM, bound to the shop and the field), never returned by any API of the admin, and read back only by the
-- assistant of that shop through its own token.

create table tenant_assistant_settings (
  tenant_id uuid primary key references tenants(id) on delete cascade,
  enabled boolean not null default false,
  is_test boolean not null default false,
  model_key text not null default 'gemini-3.1-flash-lite' check (char_length(model_key) between 1 and 64),
  line_mode text not null default 'NONE' check (line_mode in ('NONE', 'CENTRAL', 'OWN')),
  openrouter_key_ciphertext bytea,
  openrouter_key_nonce bytea,
  openrouter_key_last4 text check (openrouter_key_last4 is null or char_length(openrouter_key_last4) = 4),
  telegram_token_ciphertext bytea,
  telegram_token_nonce bytea,
  line_secret_ciphertext bytea,
  line_secret_nonce bytea,
  line_token_ciphertext bytea,
  line_token_nonce bytea,
  encryption_key_id text,
  version integer not null default 1 check (version > 0),
  config_version bigint not null default 1 check (config_version > 0),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  check ((openrouter_key_ciphertext is null) = (openrouter_key_nonce is null)),
  check ((telegram_token_ciphertext is null) = (telegram_token_nonce is null)),
  check ((line_secret_ciphertext is null) = (line_secret_nonce is null)),
  check ((line_token_ciphertext is null) = (line_token_nonce is null))
);

-- The LINE channel of the operator, shared by every shop whose line_mode is CENTRAL. One row.
create table assistant_global_settings (
  singleton boolean primary key default true check (singleton),
  line_secret_ciphertext bytea,
  line_secret_nonce bytea,
  line_token_ciphertext bytea,
  line_token_nonce bytea,
  encryption_key_id text,
  config_version bigint not null default 1 check (config_version > 0),
  updated_at timestamptz not null default now(),
  check ((line_secret_ciphertext is null) = (line_secret_nonce is null)),
  check ((line_token_ciphertext is null) = (line_token_nonce is null))
);
insert into assistant_global_settings (singleton) values (true);

-- What the shop's assistant says it is running, so the admin page can show "using version 5".
create table tenant_assistant_status (
  tenant_id uuid primary key references tenants(id) on delete cascade,
  applied_config_version bigint not null check (applied_config_version >= 0),
  reported_model_key text,
  started_at timestamptz,
  last_seen_at timestamptz not null,
  last_error_code text
);
