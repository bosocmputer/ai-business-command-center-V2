import unittest

import health_report


def result(*keys):
    items = [{"area": "ข้อมูลของร้าน", "key": key, "status": "WARN", "title": f"หัวข้อ {key} 95%", "detail": "x"} for key in keys]
    items.append({"area": "รายงาน", "key": "sales", "status": "PASS", "title": "ผ่าน"})
    return {"tenantId": "t", "generatedAt": "2026-10-09T09:00:00Z", "items": items}


class HealthReportTests(unittest.TestCase):
    def test_the_most_important_finding_comes_first_and_only_data_findings_are_shown(self):
        page = health_report.render(result("customer_phones", "negative_stock", "ar_due_dates"), "ร้านตัวอย่าง")
        self.assertLess(page.index("ติดลบ"), page.index("เบอร์โทรลูกค้า"))
        self.assertNotIn("หัวข้อ sales", page)
        self.assertIn("ร้านตัวอย่าง", page)
        self.assertIn("อ่านอย่างเดียว", page)

    def test_the_shops_own_number_is_kept_and_text_is_escaped(self):
        data = result("ar_due_dates")
        data["items"][0]["title"] = "ลูกหนี้ <b>95%</b> ไม่มีวันครบกำหนด"
        page = health_report.render(data, "<script>x</script>")
        self.assertIn("&lt;b&gt;95%&lt;/b&gt;", page)
        self.assertNotIn("<script>x</script>", page)

    def test_a_shop_with_nothing_to_fix_gets_a_plain_message(self):
        self.assertIn("ไม่พบจุดที่ต้องแก้", health_report.render(result(), "ร้าน"))


if __name__ == "__main__":
    unittest.main()
