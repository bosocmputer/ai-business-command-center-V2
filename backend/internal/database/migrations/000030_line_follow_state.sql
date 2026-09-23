-- Follow state comes from LINE follow/unfollow webhook events. Null means no
-- event has been received for this recipient yet.
alter table line_recipients
  add column line_follow_status text check (line_follow_status in ('FOLLOWING', 'BLOCKED')),
  add column line_follow_changed_at timestamptz;
