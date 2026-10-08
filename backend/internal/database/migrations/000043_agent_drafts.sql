-- The call log also records that the assistant made a draft for the owner to copy, and a draft request it got wrong.
-- It never holds the draft text or the customer asked about.
alter table agent_calls drop constraint agent_calls_tool_check;
alter table agent_calls add constraint agent_calls_tool_check check (tool in ('context', 'get_report', 'compare', 'latest_delivery', 'alerts', 'alert_set', 'draft'));
alter table agent_calls drop constraint agent_calls_outcome_check;
alter table agent_calls add constraint agent_calls_outcome_check check (outcome in ('OK', 'PREPARING', 'UNAVAILABLE', 'NO_DATA', 'INVALID_PERIOD', 'INVALID_ALERT', 'INVALID_DRAFT', 'RATE_LIMITED'));
