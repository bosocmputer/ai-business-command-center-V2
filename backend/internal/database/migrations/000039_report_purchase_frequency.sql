-- Purchase frequency: how often each customer buys and whether one has gone
-- quiet for longer than usual. Other tables reference report_definitions, so a
-- new report needs its row here.
insert into report_definitions (report_key, version, label_th, category, is_sensitive, contract_json) values
  ('purchase_frequency', '1.0.0', 'รายงานความถี่การซื้อของลูกค้า', 'SALES', true, '{"periodParams":["dateFrom","dateTo"]}')
on conflict (report_key) do nothing;
