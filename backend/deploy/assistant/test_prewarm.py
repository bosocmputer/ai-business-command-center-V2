import json
import unittest
from datetime import date
from unittest import mock

import prewarm

CONTEXT = {"today": "2026-10-12", "reports": [{"key": key} for key in prewarm.ALWAYS + prewarm.LAST_MONTH]}


class PrewarmTests(unittest.TestCase):
    def test_yesterday_and_today_sales_come_first_and_are_single_days(self):
        jobs = prewarm.day_jobs({"sales_goods_services"}, date(2026, 10, 12))
        self.assertEqual([name for name, _ in jobs], ["sales_goods_services@yesterday", "sales_goods_services@today"])
        self.assertIn("dateFrom=2026-10-11&dateTo=2026-10-11", jobs[0][1])
        self.assertIn("dateFrom=2026-10-12&dateTo=2026-10-12", jobs[1][1])
        self.assertEqual(prewarm.day_jobs({"ar_aging"}, date(2026, 10, 12)), [], "a shop that may not read sales has no sales job")

    def run_main(self, answers):
        asked, out = [], []

        def get(path):
            asked.append(path)
            return answers(path)

        with mock.patch.object(prewarm, "get", get), mock.patch.object(prewarm.time, "sleep") as sleeping, mock.patch("builtins.print", lambda text, **kw: out.append(text)):
            prewarm.main()
        return asked, json.loads(out[-1]), sleeping

    def test_the_morning_list_has_14_fetches_with_sales_first_and_none_repeated_under_one_name(self):
        def answers(path):
            return CONTEXT if path == "/context" else {"status": "READY"}
        asked, result, _ = self.run_main(answers)
        reports = [path for path in asked if path != "/context"]
        self.assertEqual(len(reports), 14)
        self.assertIn("dateFrom=2026-10-11&dateTo=2026-10-11", reports[0])
        self.assertIn("dateFrom=2026-10-12&dateTo=2026-10-12", reports[1])
        self.assertEqual(result["ready"], 14, result)

    def test_no_more_than_seven_live_fetches_are_started_in_an_hour_so_the_owner_keeps_some(self):
        states = {}

        def answers(path):
            if path == "/context":
                return CONTEXT
            states[path] = states.get(path, 0) + 1
            return {"status": "PREPARING"} if states[path] == 1 else {"status": "READY"}  # every report needs a live fetch

        clock = [1000.0]
        with mock.patch.object(prewarm.time, "monotonic", lambda: clock[0]):
            def sleep(seconds):
                clock[0] += seconds
            started = []
            real_warm = prewarm.warm

            def watching_warm(path, deadline, starts=None):
                before = len(starts)
                result = real_warm(path, deadline, starts)
                if len(starts) > before:
                    started.append(clock[0])
                return result

            with mock.patch.object(prewarm, "warm", watching_warm):
                asked, result, _ = self.run_main_with_sleep(answers, sleep)
        self.assertEqual(result["ready"], 14, result)
        for index, moment in enumerate(started):
            in_hour = [other for other in started[: index + 1] if moment - other < 3600]
            self.assertLessEqual(len(in_hour), prewarm.MAX_PER_HOUR, "more than the cap in one hour")

    def run_main_with_sleep(self, answers, sleep):
        asked, out = [], []

        def get(path):
            asked.append(path)
            return answers(path)

        with mock.patch.object(prewarm, "get", get), mock.patch.object(prewarm.time, "sleep", sleep), mock.patch("builtins.print", lambda text, **kw: out.append(text)):
            prewarm.main()
        return asked, json.loads(out[-1]), None

    def test_when_the_deadline_comes_first_the_rest_are_reported_not_tried(self):
        starts = [1000.0] * prewarm.MAX_PER_HOUR
        with mock.patch.object(prewarm.time, "monotonic", return_value=1010.0):
            self.assertFalse(prewarm.pace(starts, 1010.0 + 600))
            self.assertTrue(prewarm.pace([], 5000.0))


if __name__ == "__main__":
    unittest.main()
