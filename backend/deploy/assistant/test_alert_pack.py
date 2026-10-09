import unittest

import alert_pack


def alerts(**rules):
    names = ["morning_digest", "sales_drop", "receipts_drop", "ar_overdue", "ar_over_year", "margin_drop", "stock_reorder"]
    return {"alerts": [{"rule": name, "available": True, "enabled": False, **rules.get(name, {})} for name in names]}


class PackTests(unittest.TestCase):
    def actions(self, data, **kwargs):
        return {rule: action for rule, _, _, action in alert_pack.plan(data, **kwargs)}

    def test_a_new_owner_gets_the_standard_set_without_stock(self):
        actions = self.actions(alerts())
        self.assertEqual([rule for rule, action in actions.items() if action == "set"], ["morning_digest", "sales_drop", "receipts_drop", "ar_overdue", "ar_over_year", "margin_drop"])
        self.assertNotIn("stock_reorder", actions)

    def test_stock_is_added_only_when_asked(self):
        self.assertEqual(self.actions(alerts(), with_stock=True)["stock_reorder"], "set")

    def test_an_owners_own_choice_is_never_touched(self):
        actions = self.actions(alerts(sales_drop={"threshold": "20", "enabled": False}, ar_overdue={"enabled": True}))
        self.assertTrue(actions["sales_drop"].startswith("ข้าม"))
        self.assertTrue(actions["ar_overdue"].startswith("ข้าม"))

    def test_a_rule_the_person_may_not_use_is_skipped(self):
        data = alerts(margin_drop={"available": False})
        self.assertTrue(self.actions(data)["margin_drop"].startswith("ข้าม"))


if __name__ == "__main__":
    unittest.main()
