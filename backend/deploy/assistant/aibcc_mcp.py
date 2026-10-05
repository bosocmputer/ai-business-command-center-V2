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


def call(path, query=None):
    query = {k: v for k, v in (query or {}).items() if v}
    url = BASE + path + (("?" + urllib.parse.urlencode(query)) if query else "")
    request = urllib.request.Request(url, headers={"Authorization": f"Bearer {TOKEN}"})
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


def call_waiting(path, query=None):
    """Like call(), but while AI-BCC says the report is being prepared, ask again every few seconds, up to WAIT_SECONDS."""
    body = call(path, query)
    deadline = time.monotonic() + WAIT_SECONDS
    while is_preparing(body) and time.monotonic() < deadline:
        time.sleep(max(0.0, min(POLL_SECONDS, deadline - time.monotonic())))
        body = call(path, query)
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


if __name__ == "__main__":
    mcp.run()
