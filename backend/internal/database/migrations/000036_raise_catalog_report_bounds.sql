-- The catalog is no longer capped at ten reports. Dashboard refreshes and
-- generations cover every report a viewer may see, so their bounds follow the
-- catalog limit (report.MaxCatalogReports = 64). A LINE card keeps its own limit
-- of ten reports (notification_schedule_reports, notification_run_reports).
do $$
declare
  item record;
begin
  for item in
    select c.conrelid::regclass as table_name, c.conname
    from pg_constraint c
    where c.contype = 'c'
      and c.conrelid in ('dashboard_refreshes'::regclass, 'dashboard_generations'::regclass, 'dashboard_generation_reports'::regclass)
      and pg_get_constraintdef(c.oid) ~ '(total|"?position"?) <= 10\)'
  loop
    execute format('alter table %s drop constraint %I', item.table_name, item.conname);
  end loop;
end $$;

alter table dashboard_refreshes
  add constraint dashboard_refreshes_total_check check (total between 1 and 64);
alter table dashboard_generations
  add constraint dashboard_generations_total_check check (total between 1 and 64);
alter table dashboard_generation_reports
  add constraint dashboard_generation_reports_position_check check ("position" between 1 and 64);

-- Fail loudly if an old bound survived, because it would silently cap the catalog.
do $$
begin
  if exists (
    select 1 from pg_constraint c
    where c.contype = 'c'
      and c.conrelid in ('dashboard_refreshes'::regclass, 'dashboard_generations'::regclass, 'dashboard_generation_reports'::regclass)
      and pg_get_constraintdef(c.oid) ~ '<= 10\)'
  ) then
    raise exception 'a dashboard report bound of 10 is still in place';
  end if;
end $$;
