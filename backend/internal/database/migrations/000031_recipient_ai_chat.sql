-- Per-recipient switch for talking to the AI assistant. It is stored ahead of
-- the assistant itself: nothing enforces it yet, and it defaults to off so no
-- existing recipient gains access by this migration.
alter table tenant_memberships
  add column ai_chat_enabled boolean not null default false,
  add column ai_chat_updated_at timestamptz;
