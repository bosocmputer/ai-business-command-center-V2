-- A question that asked the assistant to read the latest figures from the shop's system again counts against the shop's hourly budget of reads.
alter table agent_calls drop constraint agent_calls_outcome_check;
alter table agent_calls add constraint agent_calls_outcome_check check (outcome in ('OK', 'PREPARING', 'UNAVAILABLE', 'NO_DATA', 'INVALID_PERIOD', 'INVALID_ALERT', 'INVALID_DRAFT', 'INVALID_SEARCH', 'INVALID_LOOKUP', 'RATE_LIMITED', 'REFRESHED'));
