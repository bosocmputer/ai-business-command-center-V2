-- The call log also records a report export the assistant asked for (the report and period only, never a row).
alter table agent_calls drop constraint agent_calls_tool_check;
alter table agent_calls add constraint agent_calls_tool_check check (tool in ('context', 'get_report', 'compare', 'latest_delivery', 'alerts', 'alert_set', 'draft', 'search', 'lookup', 'export'));
