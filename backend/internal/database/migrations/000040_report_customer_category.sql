-- Customer RFM and purchase frequency are about customers, not about the sales
-- documents themselves. They get their own category so the permission page can
-- group the reports that answer questions about customers together.
update report_definitions set category = 'CUSTOMER' where report_key in ('customer_rfm', 'purchase_frequency');
