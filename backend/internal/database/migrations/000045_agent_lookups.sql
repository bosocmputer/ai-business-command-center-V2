-- The call log also records a live lookup the assistant asked for (the kind only, never the customer or item) and a
-- lookup request it got wrong.
alter table agent_calls drop constraint agent_calls_tool_check;
alter table agent_calls add constraint agent_calls_tool_check check (tool in ('context', 'get_report', 'compare', 'latest_delivery', 'alerts', 'alert_set', 'draft', 'search', 'lookup'));
alter table agent_calls drop constraint agent_calls_outcome_check;
alter table agent_calls add constraint agent_calls_outcome_check check (outcome in ('OK', 'PREPARING', 'UNAVAILABLE', 'NO_DATA', 'INVALID_PERIOD', 'INVALID_ALERT', 'INVALID_DRAFT', 'INVALID_SEARCH', 'INVALID_LOOKUP', 'RATE_LIMITED'));
