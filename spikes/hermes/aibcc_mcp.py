"""MCP shim: gives Hermes the tools it may use against the (mock) AI-BCC Agent API.

The token comes from the environment of this process, never from the model.
"""
import json
import os
import urllib.error
import urllib.parse
import urllib.request

try:
    from mcp.server.mcpserver import MCPServer as FastMCP  # newer SDK, the one bundled with Hermes
except ImportError:  # older SDK
    from mcp.server.fastmcp import FastMCP

BASE = os.environ.get("AIBCC_URL", "http://aibcc-mock:8099")
TOKEN = os.environ.get("AIBCC_TOKEN", "")
mcp = FastMCP("aibcc")


def call(path, query=None):
    url = BASE + path + (("?" + urllib.parse.urlencode(query)) if query else "")
    request = urllib.request.Request(url, headers={"Authorization": f"Bearer {TOKEN}"})
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            return json.dumps(json.loads(response.read()), ensure_ascii=False)
    except urllib.error.HTTPError as error:
        # The shop is told "no data" whether the report is missing or not permitted.
        body = error.read().decode("utf-8", "replace")
        return json.dumps({"ok": False, "status": error.code, "detail": json.loads(body) if body else {}}, ensure_ascii=False)


@mcp.tool()
def context() -> str:
    """Shop name, today's date, and the reports this shop can ask about. Call this first."""
    return call("/v1/context")


@mcp.tool()
def get_report(report_key: str, date_from: str = "", date_to: str = "") -> str:
    """Fetch one report as numbers. report_key is one of the keys from context(). Dates are YYYY-MM-DD.
    Date-range reports use date_from..date_to (default: this month to today). As-of reports use date_to."""
    query = {k: v for k, v in {"dateFrom": date_from, "dateTo": date_to}.items() if v}
    return call("/v1/reports/" + urllib.parse.quote(report_key, safe=""), query)


if __name__ == "__main__":
    mcp.run()
