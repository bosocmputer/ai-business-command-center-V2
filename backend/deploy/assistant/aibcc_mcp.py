"""MCP shim: the four tools the assistant may use against the AI-BCC Agent API (/api/v1/agent/*).

The token comes from the environment of this process, never from the model. The shim adds no logic and no
permission of its own: it forwards the call and returns what AI-BCC answered, refusals included.
"""
import json
import os
import urllib.error
import urllib.parse
import urllib.request

try:
    from mcp.server.mcpserver import MCPServer as FastMCP  # the SDK bundled with Hermes
except ImportError:  # older SDK
    from mcp.server.fastmcp import FastMCP

BASE = os.environ.get("AIBCC_URL", "http://api:8080").rstrip("/") + "/api/v1/agent"
TOKEN = os.environ.get("AIBCC_TOKEN", "")
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


@mcp.tool()
def context() -> str:
    """Shop name, today's date, the reports this shop can ask about (key, period mode, whether compare works) and notes. Call this first."""
    return call("/context")


@mcp.tool()
def get_report(report_key: str, date_from: str = "", date_to: str = "") -> str:
    """Fetch one report as numbers. report_key comes from context(). Dates are YYYY-MM-DD.
    DATE_RANGE reports use date_from..date_to (default: this month so far). AS_OF_DATE reports use date_to (default: today)."""
    return call("/reports/" + urllib.parse.quote(report_key, safe=""), {"dateFrom": date_from, "dateTo": date_to})


@mcp.tool()
def compare(report_key: str, metric: str, a_from: str, a_to: str, b_from: str, b_to: str) -> str:
    """Compare one number (metric = a kpi key from get_report, e.g. total_amount) between period A and period B.
    The server computes the difference and percent: never compute them yourself. Only for reports with supportsCompare."""
    return call("/compare", {"reportKey": report_key, "metric": metric, "aFrom": a_from, "aTo": a_to, "bFrom": b_from, "bTo": b_to})


@mcp.tool()
def latest_delivery(report_key: str) -> str:
    """The report exactly as the owner last received it on the morning card (what the owner has already seen)."""
    return call("/deliveries/latest", {"reportKey": report_key})


if __name__ == "__main__":
    mcp.run()
