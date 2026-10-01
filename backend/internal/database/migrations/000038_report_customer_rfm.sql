-- Customer RFM: how recently, how often and how much each customer bought in a
-- period, with a segment per customer. Other tables reference
-- report_definitions, so a new report needs its row here.
insert into report_definitions (report_key, version, label_th, category, is_sensitive, contract_json) values
  ('customer_rfm', '1.0.0', 'รายงานลูกค้าตามความถี่และมูลค่าการซื้อ (RFM)', 'SALES', true, '{"periodParams":["dateFrom","dateTo"]}')
on conflict (report_key) do nothing;
