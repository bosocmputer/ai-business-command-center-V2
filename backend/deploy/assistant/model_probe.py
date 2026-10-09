"""One measurable question put to the assistant that is running a given model, scored by this script and not by a person. Run inside the
throwaway container that model-probe.sh starts (never in the shop's own assistant):

  python /assistant/model_probe.py <label>

The question needs three tools (the sales report, the receivables report, a live document lookup), so one answer shows whether the model
calls tools correctly, copies numbers exactly, says "not found" when something is not found instead of inventing it, gives the source and
time, and answers in Thai. The right numbers are fetched from AI-BCC just before and just after the question (the shop's data moves, so
either value counts). Output is one JSON line with counts, numbers and times only: no answer text and no shop name.
"""
import json
import os
import re
import sqlite3
import sys
import time
import urllib.request
import uuid

sys.path.insert(0, "/assistant")
import aibcc_mcp  # noqa: E402

QUESTION = ("ขอสรุป 3 ข้อสั้น ๆ 1) ยอดขายสุทธิรวม VAT วันที่ 1-8 ตุลาคม 2569 เท่าไร "
            "2) ตอนนี้ลูกหนี้ค้างรวมเท่าไร และเลยกำหนดเท่าไร "
            "3) ใบขายเลขที่ ZZ-TEST-99999 มีในระบบไหม "
            "ตอบเป็นข้อ ๆ บอกที่มาและข้อมูล ณ เวลาไหนของแต่ละข้อ")
GATEWAY = "http://127.0.0.1:8642/v1/chat/completions"
KEY = os.environ["API_SERVER_KEY"]


def truth():
    sales = json.loads(aibcc_mcp.call("/reports/sales_goods_services", {"dateFrom": "2026-10-01", "dateTo": "2026-10-08"}))
    ar = json.loads(aibcc_mcp.call("/reports/ar_aging"))
    values = {item["key"]: item["value"] for item in (sales.get("kpis", []) + ar.get("kpis", []))}
    return {"sales": values.get("total_amount"), "ar_total": values.get("total_balance"), "ar_overdue": values.get("overdue_amount")}


def openrouter_usage():
    request = urllib.request.Request("https://openrouter.ai/api/v1/key", headers={"Authorization": "Bearer " + os.environ["OPENROUTER_API_KEY"]})
    proxy = os.environ.get("HTTPS_PROXY")
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({"https": proxy})) if proxy else urllib.request.build_opener()
    return float(json.load(opener.open(request, timeout=20))["data"]["usage"])


def numbers_in(text):
    return [float(token.replace(",", "")) for token in re.findall(r"\d[\d,]*\.?\d*", text) if token.replace(",", "").replace(".", "").isdigit()]


def has(text, expected):
    if expected is None:
        return False
    target = float(expected)
    return any(abs(value - target) < 1.0 for value in numbers_in(text))


def main(label):
    before, usage_before = truth(), openrouter_usage()
    session = "probe-" + uuid.uuid4().hex[:8]
    body = json.dumps({"model": "hermes", "messages": [{"role": "user", "content": QUESTION}], "stream": False}).encode()
    request = urllib.request.Request(GATEWAY, data=body, headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json", "X-Hermes-Session-Id": session})
    started, answer, error = time.time(), "", ""
    try:
        answer = json.load(urllib.request.urlopen(request, timeout=420))["choices"][0]["message"]["content"] or ""
    except Exception as failure:  # noqa: BLE001 - reported as a failed probe, never a trace
        error = type(failure).__name__
    seconds = round(time.time() - started, 1)
    after, usage_after = truth(), openrouter_usage()
    tools, tokens = [], {}
    try:
        connection = sqlite3.connect("/opt/data/state.db")
        row = connection.execute("select id, input_tokens, output_tokens, cache_read_tokens, api_call_count, tool_call_count, coalesce(actual_cost_usd, estimated_cost_usd, 0) from sessions order by started_at desc limit 1").fetchone()
        if row:
            tokens = {"input": row[1], "output": row[2], "cache_read": row[3], "api_calls": row[4], "tool_calls": row[5], "reported_cost_usd": row[6]}
            for (calls,) in connection.execute("select tool_calls from messages where session_id = ? and tool_calls is not null", (row[0],)):
                for call in json.loads(calls) if calls else []:
                    function = call.get("function", call)
                    tools.append(function.get("name", "?") + (":" + str(json.loads(function.get("arguments") or "{}").get("kind", "")) if function.get("name", "").endswith("lookup") else ""))
    except Exception:  # noqa: BLE001
        pass
    thai = len(re.findall(r"[ก-๙]", answer))
    letters = len(re.findall(r"[A-Za-zก-๙]", answer)) or 1
    not_found = bool(re.search(r"ไม่พบ|ไม่มี(?:ใบ|เอกสาร|เลข|ใน)", answer)) and not re.search(r"(?<!ไม่)พบเอกสาร|มีใบขายเลขที่\s*ZZ", answer)
    checks = {
        "sales": has(answer, before["sales"]) or has(answer, after["sales"]),
        "ar_total": has(answer, before["ar_total"]) or has(answer, after["ar_total"]),
        "ar_overdue": has(answer, before["ar_overdue"]) or has(answer, after["ar_overdue"]),
        "not_found_said": not_found,
        "lookup_tool_used": any(name.endswith("lookup:document") for name in tools),
        "source_and_time": bool(re.search(r"ณ|\d{1,2}[:.]\d{2}\s*น", answer)) and bool(re.search(r"รายงาน|ระบบ", answer)),
        "thai_answer": thai / letters >= 0.6 and 80 <= len(answer) <= 1500,
    }
    print(json.dumps({"label": label, "error": error, "seconds": seconds, "answer_chars": len(answer), "score": sum(checks.values()), "of": len(checks),
                      "checks": checks, "tools": tools, "tokens": tokens, "openrouter_usd": round(usage_after - usage_before, 5)}, ensure_ascii=False))


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "probe")
