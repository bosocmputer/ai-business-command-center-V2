-- Receivable aging: what each customer still owes, document by document, with
-- buckets by days past due. Other tables reference report_definitions, so a new
-- report needs its row here.
insert into report_definitions (report_key, version, label_th, category, is_sensitive, contract_json) values
  ('ar_aging', '1.0.0', 'รายงานอายุหนี้ลูกหนี้', 'AR', true, '{"periodParams":["asOfDate"]}')
on conflict (report_key) do nothing;
