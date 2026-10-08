-- When someone says a number was wrong, the proof (what was asked, which stored report answered, what the cards and alerts
-- said) would be deleted by the normal retention before the question is settled: snapshots after 90 days, call logs after
-- 365. A case is a copy of that proof taken on purpose, kept for three years, in one row, apart from the retention of the
-- tables it was copied from. It holds report figures and may hold names that appear in a report's top lists, so it is
-- deleted with the shop and expires.
create table evidence_cases (
  id uuid primary key default gen_random_uuid(),
  tenant_id uuid not null references tenants(id) on delete cascade,
  reason text not null check (char_length(reason) between 1 and 300),
  requested_by text not null check (char_length(requested_by) between 1 and 80),
  window_from timestamptz not null,
  window_to timestamptz not null check (window_to >= window_from),
  created_at timestamptz not null default now(),
  expires_at timestamptz not null default (now() + interval '1095 days'),
  payload jsonb not null check (octet_length(payload::text) <= 16777216)
);
create index evidence_cases_tenant_idx on evidence_cases (tenant_id, created_at desc);
create index evidence_cases_expiry_idx on evidence_cases (expires_at);
