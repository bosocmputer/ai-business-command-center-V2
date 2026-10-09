#!/usr/bin/env python3
"""Turns the output of `onboard-check.sh <tenant> json` into a one-file HTML report an owner can read: what is wrong with the
shop's own data, why it matters, and what to do about it. Counts only (the check prints no names or per-customer figures).

  ./onboard-check.sh <tenant> json | python3 health_report.py "ชื่อร้าน" > health.html
"""
import html
import json
import re
import sys
from datetime import datetime

# What each finding means to an owner. Keyed by the check's item key; the check's own title carries the shop's number.
ADVICE = {
    "ar_due_dates": ("ลูกหนี้ส่วนใหญ่ไม่มีวันครบกำหนด", "high",
                     "ระบบบอกไม่ได้ว่าใครเลยกำหนดจ่ายแล้ว ยอดเลยกำหนดจึงต่ำกว่าความจริง และเลขา AI ร่างข้อความทวงให้ไม่ได้",
                     "ให้ผู้ทำใบขายเครดิตกรอกจำนวนวันเครดิตทุกครั้ง และตั้งเครดิตวันมาตรฐานในข้อมูลลูกค้าแต่ละราย"),
    "reorder_points": ("ไม่มีสินค้าที่ตั้งจุดสั่งซื้อ", "medium",
                       "รายงานและการเตือน “สินค้าใกล้หมด” จะว่างเสมอ เพราะระบบไม่รู้ว่าเหลือเท่าไรถึงควรสั่ง",
                       "เริ่มจากสินค้าขายดี 20 อันดับ ตั้งจุดสั่งซื้อให้เท่ากับยอดขายประมาณ 1–2 สัปดาห์"),
    "negative_stock": ("มีสินค้าที่ยอดคงเหลือติดลบ", "high",
                       "มูลค่าสต็อกและต้นทุนขายคลาดเคลื่อน กำไรที่เห็นอาจไม่จริง ติดลบมักมาจากขายก่อนรับของเข้าระบบหรือรับของผิดรหัส",
                       "ให้ฝ่ายคลังตรวจรายการติดลบ รับของเข้าให้ครบ หรือปรับปรุงสต็อก แล้วตั้งเป็นกิจวัตรตรวจทุกสัปดาห์"),
    "item_suppliers": ("สินค้าไม่มีผู้จำหน่ายประจำ", "low",
                       "รายการสั่งซื้อที่เลขาร่างให้จัดกลุ่มตามผู้จำหน่ายไม่ได้",
                       "ใส่รหัสผู้จำหน่ายประจำในข้อมูลสินค้าที่สั่งบ่อย"),
    "prices": ("ไม่มีตารางราคาขาย", "low",
               "ถามเลขาว่าสินค้านี้ราคาเท่าไรไม่ได้",
               "บันทึกราคาขายมาตรฐานในระบบ หรือแจ้งราคาให้เลขาเป็นรายการ"),
    "customer_phones": ("เบอร์โทรลูกค้าไม่ครบ", "low",
                        "ค้นเบอร์โทรลูกค้าให้เจ้าของได้เฉพาะรายที่บันทึกไว้",
                        "ขอเบอร์เมื่อเปิดลูกค้าใหม่ และเติมเบอร์ของลูกค้าประจำ"),
}
ORDER = {"high": 0, "medium": 1, "low": 2}
LABEL = {"high": "ควรแก้ก่อน", "medium": "ควรแก้", "low": "แก้เมื่อสะดวก"}
COLOR = {"high": "#b3261e", "medium": "#a05a00", "low": "#4d6a8a"}


def render(result, shop):
    findings = [item for item in result.get("items", []) if item.get("area") == "ข้อมูลของร้าน" and item.get("key") in ADVICE]
    findings.sort(key=lambda item: ORDER[ADVICE[item["key"]][1]])
    stamp = datetime.fromisoformat(result["generatedAt"].replace("Z", "+00:00")).astimezone().strftime("%d/%m/%Y %H:%M")
    cards = []
    for item in findings:
        title, level, why, todo = ADVICE[item["key"]]
        cards.append(
            f'<section class="card"><span class="tag" style="background:{COLOR[level]}">{LABEL[level]}</span>'
            f"<h2>{html.escape(title)}</h2><p class=\"fact\">{html.escape(item['title'])}</p>"
            f"<p><b>ผลต่อร้าน:</b> {html.escape(why)}</p><p><b>วิธีแก้:</b> {html.escape(todo)}</p></section>")
    if not cards:
        cards.append('<section class="card"><h2>ไม่พบจุดที่ต้องแก้</h2><p>ข้อมูลที่ตรวจพร้อมใช้งาน</p></section>')
    return f"""<!doctype html><html lang="th"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>สุขภาพข้อมูลของร้าน {html.escape(shop)}</title>
<style>body{{font-family:Tahoma,sans-serif;margin:0;padding:24px 16px;background:#f6f7f9;color:#1d2530}}main{{max-width:760px;margin:0 auto}}
h1{{font-size:22px;margin:0 0 4px}}.sub{{color:#5b6673;margin:0 0 20px}}.card{{background:#fff;border:1px solid #dde2e8;border-radius:10px;padding:16px;margin:0 0 14px}}
.card h2{{font-size:17px;margin:8px 0}}.tag{{color:#fff;font-size:12px;padding:2px 8px;border-radius:99px}}.fact{{font-weight:bold}}p{{line-height:1.6;margin:6px 0}}
.note{{color:#5b6673;font-size:13px;margin-top:20px}}</style></head><body><main>
<h1>สุขภาพข้อมูลของร้าน {html.escape(shop)}</h1><p class="sub">ตรวจจากระบบ SML ของร้านเมื่อ {stamp} (อ่านอย่างเดียว ไม่แก้ข้อมูล)</p>
{''.join(cards)}
<p class="note">รายงานนี้นับจำนวนจากข้อมูลจริง ไม่ได้แสดงชื่อลูกค้าหรือยอดรายบุคคล ตัวเลขทางการเงินของร้านดูได้จากเลขา AI และรายงานในระบบ จุดที่แก้แล้วจะทำให้ตัวเลขและการเตือนของเลขา AI แม่นขึ้น</p>
</main></body></html>"""


if __name__ == "__main__":
    data = json.load(sys.stdin)
    sys.stdout.write(render(data, sys.argv[1] if len(sys.argv) > 1 else ""))
