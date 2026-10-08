"""MCP shim: the four tools the assistant may use against the AI-BCC Agent API (/api/v1/agent/*).

The token comes from the environment of this process, never from the model. The shim adds no logic and no
permission of its own: it forwards the call and returns what AI-BCC answered, refusals included.
"""
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request

try:
    from mcp.server.mcpserver import MCPServer as FastMCP  # the SDK bundled with Hermes
except ImportError:  # older SDK
    from mcp.server.fastmcp import FastMCP

BASE = os.environ.get("AIBCC_URL", "http://api:8080").rstrip("/") + "/api/v1/agent"
TOKEN = os.environ.get("AIBCC_TOKEN", "")
# A report that has to be fetched live from the shop's system takes 15 to 60 seconds. Instead of telling the owner to ask
# again, the tool waits for it and returns the finished report. AI-BCC does not count these status checks against the
# hourly call limit (the first 240 an hour), and Hermes allows a tool five minutes.
WAIT_SECONDS = float(os.environ.get("AIBCC_WAIT_SECONDS", "100"))
POLL_SECONDS = float(os.environ.get("AIBCC_POLL_SECONDS", "6"))
mcp = FastMCP("aibcc")


def call(path, query=None, method="GET", body=None):
    query = {k: v for k, v in (query or {}).items() if v}
    url = BASE + path + (("?" + urllib.parse.urlencode(query)) if query else "")
    headers = {"Authorization": f"Bearer {TOKEN}"}
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return response.read().decode("utf-8")
    except urllib.error.HTTPError as error:
        # AI-BCC answers a missing and a forbidden report with the same bytes; pass its answer on untouched.
        return error.read().decode("utf-8", "replace") or json.dumps({"status": "ERROR", "httpStatus": error.code})
    except (urllib.error.URLError, TimeoutError):
        return json.dumps({"status": "UNAVAILABLE", "message": "ติดต่อระบบรายงานไม่ได้ในขณะนี้ ลองถามใหม่ภายหลัง"}, ensure_ascii=False)


def is_preparing(body):
    try:
        return json.loads(body).get("status") == "PREPARING"
    except (ValueError, AttributeError):
        return False


def call_waiting(path, query=None, method="GET", payload=None):
    """Like call(), but while AI-BCC says the report is being prepared, ask again every few seconds, up to WAIT_SECONDS."""
    body = call(path, query, method, payload)
    deadline = time.monotonic() + WAIT_SECONDS
    while is_preparing(body) and time.monotonic() < deadline:
        time.sleep(max(0.0, min(POLL_SECONDS, deadline - time.monotonic())))
        body = call(path, query, method, payload)
    return body


@mcp.tool()
def context() -> str:
    """Shop name, today's date, the reports this shop can ask about (key, period mode, whether compare works) and notes. Call this first."""
    return call("/context")


@mcp.tool()
def get_report(report_key: str, date_from: str = "", date_to: str = "") -> str:
    """Fetch one report as numbers. report_key comes from context(). Dates are YYYY-MM-DD.
    DATE_RANGE reports use date_from..date_to (default: this month so far). AS_OF_DATE reports use date_to (default: today).
    If the report has to be fetched live this call waits up to about 100 seconds and returns the finished report: just wait."""
    return call_waiting("/reports/" + urllib.parse.quote(report_key, safe=""), {"dateFrom": date_from, "dateTo": date_to})


@mcp.tool()
def compare(report_key: str, metric: str, a_from: str, a_to: str, b_from: str, b_to: str) -> str:
    """Compare one number (metric = a kpi key from get_report, e.g. total_amount) between period A and period B.
    The server computes the difference and percent: never compute them yourself. Only for reports with supportsCompare.
    If a period has to be fetched live this call waits up to about 100 seconds and returns the finished comparison."""
    return call_waiting("/compare", {"reportKey": report_key, "metric": metric, "aFrom": a_from, "aTo": a_to, "bFrom": b_from, "bTo": b_to})


@mcp.tool()
def latest_delivery(report_key: str) -> str:
    """The report exactly as the owner last received it on the morning card (what the owner has already seen)."""
    return call("/deliveries/latest", {"reportKey": report_key})


@mcp.tool()
def alerts() -> str:
    """The alerts the owner can ask for (a rule key, what it watches, its unit) and which are on with what threshold.
    Call this before changing an alert, and when the owner asks what alerts are set."""
    return call("/alerts")


@mcp.tool()
def alert_set(rule_key: str, threshold: str = "", enabled: bool = True) -> str:
    """Set, change or switch off ONE alert, only when the owner clearly asks for it in this conversation.
    rule_key is exactly one of: ar_overdue, ar_over_year, stock_reorder, sales_drop, morning_digest (the keys from alerts()). morning_digest is a daily summary switch: it takes no threshold (leave threshold empty, enabled=true to turn it on, enabled=false to turn it off). threshold is the number the owner said, as digits (baht, a count of items or a
    percent depending on the rule), digits only without a unit such as "%" or "baht"; leave it empty to switch a rule off (enabled=false) or back on with its old threshold.
    This changes only the owner's own alert settings. Read the result back to the owner."""
    result = call("/alerts/" + urllib.parse.quote(rule_key, safe=""), method="PUT", body={"threshold": threshold, "enabled": enabled})
    try:
        if json.loads(result).get("status") == "NO_DATA":
            # An unknown or unavailable rule answers like a missing report. The catalog itself is not secret, so say which
            # keys exist: a wrong guess then costs one more call instead of a wrong "no data" for the owner.
            listing = json.loads(call("/alerts"))
            keys = [item["rule"] for item in listing.get("alerts", []) if item.get("available")]
            return json.dumps({"status": "UNKNOWN_RULE", "message": "ไม่รู้จักชื่อกฎนี้ ใช้ rule_key ตัวใดตัวหนึ่งเท่านั้น แล้วเรียกใหม่", "validRuleKeys": keys}, ensure_ascii=False)
    except (ValueError, AttributeError, KeyError):
        pass
    return result


@mcp.tool()
def draft_collection(customer: str, tone: str = "friendly") -> str:
    """Write a payment reminder message for ONE customer who is overdue, only when the owner asks for one in this conversation.
    customer is the name or part of the name the owner said. It works for the ten customers owing the most past their due date.
    tone is "friendly" (default) or "formal" (only when the owner asks for a formal one). The server writes the message from its own
    figures: give the owner the "draft" text exactly as returned, never edit it and never add numbers, dates or contact details.
    The draft is not sent to anyone; the owner copies and sends it. If it has to be fetched live this call waits up to about 100 seconds."""
    return call_waiting("/drafts/collection", method="POST", payload={"customer": customer, "tone": tone})


@mcp.tool()
def draft_purchase_order() -> str:
    """Write a purchase list for the items that are below their reorder point, only when the owner asks for a purchase
    draft in this conversation. The server writes the text from its own figures: give the owner the "draft" text exactly as
    returned, never edit it and never add quantities, prices or suppliers. The quantity is the shortage, not a recommended
    order. The draft is not sent to anyone; the owner copies and sends it. If it has to be fetched live this call waits up to
    about 100 seconds."""
    return call_waiting("/drafts/purchase-order", method="POST", payload={})


@mcp.tool()
def search_master(kind: str, query: str) -> str:
    """Find a customer, supplier or item by a name, a code or a phone number the owner said. kind is customer, supplier or item.
    query is one to five words; every word must appear in the name, the code or the phone number. Returns at most ten matches
    (code, name, phone for people, unit for items) and how many were found. Use it to turn a name the owner says into the exact
    name or code before asking a report, and when the owner asks for a phone number or a code. Give the owner only what they asked
    for. It reads a copy made from the shop's system once a day (syncedAt says when)."""
    return call("/search", {"kind": kind, "q": query})


if __name__ == "__main__":
    mcp.run()
