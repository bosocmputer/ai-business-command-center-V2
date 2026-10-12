"""Fetch, every morning, the reports an owner asks about first, so the first question of the day is answered at once.

Reports "as of today" (receivables, stock) and the rolling 180-day customer reports are keyed to the date, so each day's
first question would otherwise start a live fetch from the shop's system. Last month's reports are not kept for good
either: their snapshots go stale, so they are fetched every morning too. Sales of yesterday (a closed day, fresh for 24 hours)
and of today (an open day, refreshed by AI-BCC when asked after five minutes) come first: "ยอดขายเมื่อวาน" and "ยอดขายวันนี้" are what
owners ask first (14 fetches in all).

AI-BCC allows 10 live fetches an hour for a shop, and that budget is shared with the owner's own questions. This starts at
most MAX_PER_HOUR (7) fetches in any hour, so a question in the morning still finds fetches to spare, and the rest wait for
the hour to pass (all 14 are done in about an hour and a half). It asks in order, waits for each, and retries a refused one
every five minutes until the budget frees, for at most MAX_SECONDS (two hours). It prints one JSON line (counts only) at the end.
Run by entrypoint.sh at 07:30 Bangkok. Environment: AIBCC_URL, AIBCC_TOKEN.
"""
import json
import os
import time
import urllib.error
import urllib.request
from datetime import date, datetime, timedelta, timezone

BASE = os.environ.get("AIBCC_URL", "http://api:8080").rstrip("/") + "/api/v1/agent"
TOKEN = os.environ.get("AIBCC_TOKEN", "")
MAX_SECONDS = int(os.environ.get("PREWARM_MAX_SECONDS", "7200"))
MAX_PER_HOUR = int(os.environ.get("PREWARM_MAX_PER_HOUR", "7"))
POLL = float(os.environ.get("PREWARM_POLL_SECONDS", "10"))
RETRY_BUDGET = float(os.environ.get("PREWARM_BUDGET_RETRY_SECONDS", "300"))
ALWAYS = ["ar_aging", "customer_rfm", "purchase_frequency", "stock_balance", "stock_reorder", "ar_customer_movement"]
LAST_MONTH = ["sales_goods_services", "ar_debt_receipt", "purchase_goods_payables", "cash_bank_receipts", "cash_bank_payments", "gross_profit_by_product"]


def get(path):
    request = urllib.request.Request(BASE + path, headers={"Authorization": "Bearer " + TOKEN})
    try:
        return json.load(urllib.request.urlopen(request, timeout=60))
    except urllib.error.HTTPError as error:
        return {"status": "HTTP_%d" % error.code}
    except (urllib.error.URLError, TimeoutError, ValueError):
        return {"status": "UNREACHABLE"}


def day_jobs(allowed, today):
    """Sales of yesterday and of today, the first things an owner asks."""
    if "sales_goods_services" not in allowed:
        return []
    yesterday = (today - timedelta(days=1)).isoformat()
    day = lambda value: f"/reports/sales_goods_services?dateFrom={value}&dateTo={value}"
    return [("sales_goods_services@yesterday", day(yesterday)), ("sales_goods_services@today", day(today.isoformat()))]


def month_range(today):
    end = today.replace(day=1) - timedelta(days=1)
    return end.replace(day=1).isoformat(), end.isoformat()


def pace(starts, deadline):
    """Wait, if need be, until fewer than MAX_PER_HOUR live fetches were started in the last hour. False when the deadline comes first."""
    while True:
        now = time.monotonic()
        recent = [moment for moment in starts if now - moment < 3600]
        if len(recent) < MAX_PER_HOUR:
            return True
        wait = recent[0] + 3600 - now + 5
        if now + wait >= deadline:
            return False
        time.sleep(wait)


def warm(path, deadline, starts=None):
    """READY, or the last status seen. Waits while preparing, and while the live-fetch budget is used up, until the deadline.
    A first answer of PREPARING means a live fetch was started: its moment is added to starts."""
    first = True
    while True:
        data = get(path)
        status = data.get("status")
        if first and status == "PREPARING" and starts is not None:
            starts.append(time.monotonic())
        first = False
        if status == "READY" or time.monotonic() >= deadline:
            return status
        if status == "PREPARING":
            time.sleep(POLL)
        elif status == "UNAVAILABLE":
            time.sleep(min(RETRY_BUDGET, max(0.0, deadline - time.monotonic())))
        else:
            return status


def main():
    started = time.monotonic()
    deadline = started + MAX_SECONDS
    context = get("/context")
    if "reports" not in context:
        print(json.dumps({"prewarm": "skipped", "reason": context.get("status")}))
        return
    allowed = {r["key"] for r in context["reports"]}
    today = date.fromisoformat(context["today"])
    first, last = month_range(today)
    jobs = day_jobs(allowed, today)
    jobs += [(key, f"/reports/{key}") for key in ALWAYS if key in allowed]
    jobs += [(f"{key}@last_month", f"/reports/{key}?dateFrom={first}&dateTo={last}") for key in LAST_MONTH if key in allowed]
    results, starts = {}, []
    for key, path in jobs:
        if not pace(starts, deadline):
            results[key] = "NOT_TRIED"
            continue
        results[key] = warm(path, deadline, starts)
    ready = sum(1 for status in results.values() if status == "READY")
    print(json.dumps({"prewarm": "done", "ready": ready, "not_ready": {k: v for k, v in results.items() if v != "READY"},
                      "seconds": round(time.monotonic() - started), "at": datetime.now(timezone.utc).isoformat(timespec="seconds")}), flush=True)


if __name__ == "__main__":
    main()
