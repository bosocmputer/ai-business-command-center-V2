"""Fetch, every morning, the reports an owner asks about first, so the first question of the day is answered at once.

Reports "as of today" (receivables, stock) and the rolling 180-day customer reports are keyed to the date, so each day's
first question would otherwise start a live fetch from the shop's system. Last month's reports are fetched only when no
snapshot exists, which is normally only on the first day of a month.

AI-BCC allows 10 live fetches an hour; this asks in order and waits for each, retrying one that was refused for the budget
after five minutes. It prints one JSON line (counts only) when it ends and never exceeds MAX_SECONDS.
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
MAX_SECONDS = int(os.environ.get("PREWARM_MAX_SECONDS", "3600"))
POLL = float(os.environ.get("PREWARM_POLL_SECONDS", "10"))
RETRY_BUDGET = float(os.environ.get("PREWARM_BUDGET_RETRY_SECONDS", "300"))
ALWAYS = ["ar_aging", "stock_balance", "stock_reorder", "customer_rfm", "purchase_frequency", "ar_customer_movement"]
LAST_MONTH = ["sales_goods_services", "ar_debt_receipt", "purchase_goods_payables", "gross_profit_by_product", "cash_bank_receipts", "cash_bank_payments"]


def get(path):
    request = urllib.request.Request(BASE + path, headers={"Authorization": "Bearer " + TOKEN})
    try:
        return json.load(urllib.request.urlopen(request, timeout=60))
    except urllib.error.HTTPError as error:
        return {"status": "HTTP_%d" % error.code}
    except (urllib.error.URLError, TimeoutError, ValueError):
        return {"status": "UNREACHABLE"}


def month_range(today):
    end = today.replace(day=1) - timedelta(days=1)
    return end.replace(day=1).isoformat(), end.isoformat()


def warm(path, deadline):
    """READY, or the last status seen. Waits while preparing; retries once or twice when refused for the fetch budget."""
    refused = 0
    while True:
        data = get(path)
        status = data.get("status")
        if status == "READY" or time.monotonic() >= deadline:
            return status
        if status == "PREPARING":
            time.sleep(POLL)
        elif status == "UNAVAILABLE" and refused < 2:
            refused += 1
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
    jobs = [(key, f"/reports/{key}") for key in ALWAYS if key in allowed]
    jobs += [(key, f"/reports/{key}?dateFrom={first}&dateTo={last}") for key in LAST_MONTH if key in allowed]
    results = {}
    for key, path in jobs:
        results[key] = warm(path, deadline)
    ready = sum(1 for status in results.values() if status == "READY")
    print(json.dumps({"prewarm": "done", "ready": ready, "not_ready": {k: v for k, v in results.items() if v != "READY"},
                      "seconds": round(time.monotonic() - started), "at": datetime.now(timezone.utc).isoformat(timespec="seconds")}), flush=True)


if __name__ == "__main__":
    main()
