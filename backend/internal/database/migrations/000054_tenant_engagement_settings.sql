-- How a shop's owner is looked after, chosen per shop by the admin. A shop with no row uses the defaults below (what the system did before
-- this table existed, except that the weekly summary is off until someone turns it on).
create table tenant_engagement_settings (
  tenant_id uuid primary key references tenants(id) on delete cascade,
  -- where the assistant's alerts reach the owner: the assistant's Telegram chat, a LINE message, or nowhere
  alert_channel text not null default 'TELEGRAM' check (alert_channel in ('TELEGRAM', 'LINE', 'OFF')),
  -- the assistant's own morning digest (a shop on LINE already gets the report cards, so it can be switched off)
  digest_enabled boolean not null default true,
  weekly_summary_enabled boolean not null default false,
  weekly_summary_weekday smallint not null default 1 check (weekly_summary_weekday between 1 and 7),
  weekly_summary_hour smallint not null default 8 check (weekly_summary_hour between 0 and 23),
  inactivity_notice_enabled boolean not null default true,
  inactivity_quiet_days smallint not null default 5 check (inactivity_quiet_days between 2 and 60),
  version integer not null default 1,
  updated_at timestamptz not null default now()
);
