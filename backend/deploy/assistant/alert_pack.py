"""The standard set of alerts for a new owner, so the secretary speaks first instead of waiting to be asked. Run inside the assistant
container (it uses the container's own token, so it sets alerts for exactly the person that token belongs to):

  docker compose exec -T assistant /opt/hermes/.venv/bin/python /assistant/alert_pack.py            show the plan, change nothing
  docker compose exec -T assistant /opt/hermes/.venv/bin/python /assistant/alert_pack.py --apply    set the rules that are missing

Only rules the person may use and has not already set (no threshold chosen, not switched on) are added, so an owner's own choices are
never overwritten. Nothing is sent by this script: the daily check and the alert switch (alerts.sh dry|live) decide what is sent.
stock_reorder is left out unless asked for with --with-stock, because a shop with no reorder points would never hear from it.
"""
import json
import sys

STANDARD = [
    # (rule, threshold, why this default)
    ("morning_digest", "", "สรุปเช้าทุกวัน (ไม่มีเกณฑ์)"),
    ("sales_drop", "30", "ยอดขายเมื่อวานตกเกิน 30% จากวันเดียวกันของสัปดาห์ก่อน"),
    ("receipts_drop", "30", "เงินเข้าเมื่อวานตกเกิน 30% จากวันเดียวกันของสัปดาห์ก่อน"),
    ("ar_overdue", "100000", "ลูกหนี้เลยกำหนดรวมเกิน 100,000 บาท"),
    ("ar_over_year", "100000", "ลูกหนี้ค้างเกิน 1 ปีรวมเกิน 100,000 บาท"),
    ("margin_drop", "5", "อัตรากำไรขั้นต้นเมื่อวานลดเกิน 5 จุด"),
]
WITH_STOCK = ("stock_reorder", "1", "มีสินค้าถึงจุดสั่งซื้อตั้งแต่ 1 รายการ")


def plan(alerts, with_stock=False):
    """[(rule, threshold, why, action)] where action is 'set' or a reason it is skipped. alerts is the answer of GET /alerts."""
    state = {item["rule"]: item for item in alerts.get("alerts", [])}
    wanted = STANDARD + ([WITH_STOCK] if with_stock else [])
    result = []
    for rule, threshold, why in wanted:
        item = state.get(rule)
        if item is None or not item.get("available"):
            result.append((rule, threshold, why, "ข้าม: ผู้ใช้นี้ไม่มีสิทธิ์ดูรายงานของเตือนนี้"))
        elif item.get("enabled") or item.get("threshold") not in (None, ""):
            result.append((rule, threshold, why, "ข้าม: เจ้าของตั้งไว้แล้ว ไม่แตะ"))
        else:
            result.append((rule, threshold, why, "set"))
    return result


def main(argv):
    sys.path.insert(0, "/assistant")
    import aibcc_mcp
    apply = "--apply" in argv
    alerts = json.loads(aibcc_mcp.call("/alerts"))
    if alerts.get("status") != "READY":
        print("อ่านรายการเตือนไม่ได้:", alerts.get("message") or alerts.get("status"))
        return 1
    steps = plan(alerts, "--with-stock" in argv)
    for rule, threshold, why, action in steps:
        print(f"{'ตั้ง' if action == 'set' else '    '} {rule:15} {threshold:>7}  {why}  {'' if action == 'set' else '(' + action + ')'}")
        if apply and action == "set":
            answer = json.loads(aibcc_mcp.call("/alerts/" + rule, method="PUT", body={"threshold": threshold, "enabled": True}))
            print("     ->", answer.get("status") or answer.get("message") or answer)
    if not apply:
        print("\nนี่คือแผน ยังไม่ได้เปลี่ยนอะไร ใส่ --apply เพื่อตั้งจริง (การส่งจริงยังขึ้นกับ alerts.sh dry|live)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
