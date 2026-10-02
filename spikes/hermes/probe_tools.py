"""Exercise the MCP shim directly, without a model: what each shop's token can and cannot read."""
import asyncio, json, os
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

CASES = [
    ("context", {}),
    ("get_report", {"report_key": "sales_goods_services", "date_from": "2026-09-01", "date_to": "2026-09-30"}),
    ("get_report", {"report_key": "ar_aging"}),
    ("get_report", {"report_key": "customer_rfm"}),
    ("get_report", {"report_key": "stock_balance"}),
    ("get_report", {"report_key": "sales_goods_services", "date_from": "2026-09-30", "date_to": "2026-09-01"}),
    ("get_report", {"report_key": "sales_goods_services", "date_from": "2026-01-01", "date_to": "2026-01-31"}),
    ("compare", {"report_key": "sales_goods_services", "metric": "total_amount", "a_from": "2026-09-01", "a_to": "2026-09-30", "b_from": "2026-08-01", "b_to": "2026-08-31"}),
    ("latest_delivery", {"report_key": "sales_goods_services"}),
]


async def run(token, label):
    params = StdioServerParameters(command="/opt/hermes/.venv/bin/python", args=["/spike/aibcc_mcp.py"], env={"AIBCC_URL": "http://aibcc-mock:8099", "AIBCC_TOKEN": token})
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            print(label, "tools:", [t.name for t in (await session.list_tools()).tools])
            for name, args in CASES:
                text = (await session.call_tool(name, args)).content[0].text
                status = json.loads(text).get("status", "?")
                print(f"  {name:16} {args.get('report_key', ''):22} -> {status:12} {text[:110]!r}")


asyncio.run(run(os.environ["TOKEN_A"], "shop A"))
asyncio.run(run(os.environ["TOKEN_B"], "shop B"))
