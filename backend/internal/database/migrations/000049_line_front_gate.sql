-- The LINE front gate: AI-BCC decides which shop's assistant a message on the shared LINE channel goes to.
-- assistant_host is the internal name of the shop's assistant container (a single label, so it can only name a service on the internal network).
alter table tenant_assistant_settings
  add column assistant_host text not null default '' check (assistant_host = '' or assistant_host ~ '^[a-z0-9]([a-z0-9-]{0,60}[a-z0-9])?$');

-- Which shop a person with several shops is talking to on the shared channel.
create table line_chat_selection (
  recipient_id uuid primary key references line_recipients(id) on delete cascade,
  tenant_id uuid not null references tenants(id) on delete cascade,
  selected_at timestamptz not null default now()
);
