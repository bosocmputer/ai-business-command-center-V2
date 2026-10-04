"""Usage per day from the ledger plus the sessions still in the store. Counts and times only; prints no conversation text.

  usage_report.py [--days N] [--openrouter]
--openrouter also prints what OpenRouter says this key has spent (uses OPENROUTER_API_KEY from the environment).
"""
import json
import os
import statistics
import sys
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import usage_ledger  # noqa: E402

days = int(sys.argv[sys.argv.index("--days") + 1]) if "--days" in sys.argv else 7
rows = usage_ledger.read_ledger() + usage_ledger.live_rows()
by_day = {}
for row in rows:
    by_day.setdefault(row["day"], []).append(row)

print(f"{'วัน':10} {'เซสชัน':>7} {'คำถาม':>6} {'ไม่มีข้อมูล':>10} {'เรียก API':>9} {'token เข้า':>10} {'token ออก':>9} {'USD':>8} {'เวลากลาง':>8} {'ช้าสุด':>7}")
total = {"sessions": 0, "turns": 0, "unanswered": 0, "tool_calls": 0, "in": 0, "out": 0, "cost": 0.0}
topics = {}
for day in sorted(by_day)[-days:]:
    group = by_day[day]
    p50s = [r["turn_p50_s"] for r in group if r["turn_p50_s"] is not None]
    maxes = [r["turn_max_s"] for r in group if r["turn_max_s"] is not None]
    turns = sum(r["turns"] for r in group)
    print(f"{day:10} {len(group):7} {turns:6} {sum(r['unanswered'] for r in group):10} {sum(r['tool_calls'] for r in group):9} "
          f"{sum(r['in'] for r in group):10} {sum(r['out'] for r in group):9} {sum(r['cost_usd'] for r in group):8.4f} "
          f"{(statistics.median(p50s) if p50s else 0):8.1f} {(max(maxes) if maxes else 0):7.1f}")
    total["sessions"] += len(group); total["turns"] += turns; total["unanswered"] += sum(r["unanswered"] for r in group)
    total["tool_calls"] += sum(r["tool_calls"] for r in group); total["in"] += sum(r["in"] for r in group)
    total["out"] += sum(r["out"] for r in group); total["cost"] += sum(r["cost_usd"] for r in group)
    for r in group:
        for name, count in r["topics"].items():
            topics[name] = topics.get(name, 0) + count
print(f"รวม {days} วันล่าสุด: {total['sessions']} เซสชัน · {total['turns']} คำถาม · ตอบว่าไม่มีข้อมูล {total['unanswered']} · ค่าใช้จ่าย ${total['cost']:.4f}"
      + (f" (เฉลี่ย ${total['cost'] / total['turns']:.4f}/คำถาม)" if total["turns"] else ""))
if topics:
    print("หัวข้อที่ตอบว่าไม่มีข้อมูล (หมวดกว้าง ไม่ใช่ข้อความ):", ", ".join(f"{k}={v}" for k, v in sorted(topics.items(), key=lambda kv: -kv[1])))
if os.path.exists(usage_ledger.UNANSWERED):
    print("เก็บข้อความคำถามที่ตอบไม่ได้ไว้: เปิดอยู่ (ดู", usage_ledger.UNANSWERED + ")")
if "--openrouter" in sys.argv and os.environ.get("OPENROUTER_API_KEY"):
    request = urllib.request.Request("https://openrouter.ai/api/v1/key", headers={"Authorization": "Bearer " + os.environ["OPENROUTER_API_KEY"]})
    data = json.load(urllib.request.urlopen(request, timeout=15))["data"]
    print(f"OpenRouter: ใช้ไปทั้งหมด ${data['usage']:.4f} (วันนี้ ${data.get('usage_daily', 0):.4f}) จากวงเงิน ${data['limit']}")
