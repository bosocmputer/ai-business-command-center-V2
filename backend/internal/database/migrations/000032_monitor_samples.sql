-- Rolling history for the admin Monitor page. One row per 30-second sample of
-- host and container resource use; the API prunes rows older than 48 hours.
create table monitor_samples (
  sampled_at timestamptz primary key,
  payload jsonb not null
);
