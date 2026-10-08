-- A read-only copy of the shop's master data (customers, suppliers, items) so the assistant can find a record by a name
-- or phone number the owner says. It holds codes, names, phone numbers, the item unit and the supplier code of an item,
-- and no business figures. It is filled once a day by the worker from SML and is never written back.
create table master_items (
  tenant_id uuid not null references tenants(id) on delete cascade,
  kind text not null check (kind in ('CUSTOMER', 'SUPPLIER', 'ITEM')),
  code text not null check (char_length(code) between 1 and 80),
  name text not null default '' check (char_length(name) <= 300),
  phone text not null default '' check (char_length(phone) <= 100),
  unit text not null default '' check (char_length(unit) <= 100),
  supplier_code text not null default '' check (char_length(supplier_code) <= 80),
  active boolean not null default true,
  synced_at timestamptz not null,
  primary key (tenant_id, kind, code)
);

-- Where each copy stands: the last good copy, the last try, and a short code when the last try failed.
create table master_sync (
  tenant_id uuid not null references tenants(id) on delete cascade,
  kind text not null check (kind in ('CUSTOMER', 'SUPPLIER', 'ITEM')),
  synced_at timestamptz,
  attempted_at timestamptz not null,
  status text not null check (status in ('OK', 'ERROR')),
  row_count integer not null default 0 check (row_count >= 0),
  error_code text check (error_code is null or char_length(error_code) <= 60),
  primary key (tenant_id, kind)
);

-- The call log also records a search the assistant made (kind only, never the words searched for).
alter table agent_calls drop constraint agent_calls_tool_check;
alter table agent_calls add constraint agent_calls_tool_check check (tool in ('context', 'get_report', 'compare', 'latest_delivery', 'alerts', 'alert_set', 'draft', 'search'));
alter table agent_calls drop constraint agent_calls_outcome_check;
alter table agent_calls add constraint agent_calls_outcome_check check (outcome in ('OK', 'PREPARING', 'UNAVAILABLE', 'NO_DATA', 'INVALID_PERIOD', 'INVALID_ALERT', 'INVALID_DRAFT', 'INVALID_SEARCH', 'RATE_LIMITED'));
