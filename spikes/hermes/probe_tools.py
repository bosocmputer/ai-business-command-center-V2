"""Exercise the MCP shim directly, without a model: what each shop's token can and cannot read."""
import asyncio, json, os, sys
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

async def run(token, label):
    params = StdioServerParameters(command="/opt/hermes/.venv/bin/python", args=["/spike/aibcc_mcp.py"], env={"AIBCC_URL": "http://aibcc-mock:8099", "AIBCC_TOKEN": token})
    async with stdio_client(params) as (read, write):
        async with ClientSession(read, write) as session:
            await session.initialize()
            tools = await session.list_tools()
            print(label, "tools:", [t.name for t in tools.tools])
            for name, args in [("context", {}), ("get_report", {"report_key": "sales_goods_services", "date_from": "2026-09-01", "date_to": "2026-09-30"}),
                               ("get_report", {"report_key": "ar_aging"}), ("get_report", {"report_key": "stock_balance"}),
                               ("get_report", {"report_key": "sales_goods_services", "date_from": "2026-09-30", "date_to": "2026-09-01"})]:
                result = await session.call_tool(name, args)
                text = result.content[0].text
                print(" ", name, args.get("report_key", ""), "->", text[:230].replace("\n", " "))

asyncio.run(run(os.environ["TOKEN_A"], "shop A"))
asyncio.run(run(os.environ["TOKEN_B"], "shop B"))
