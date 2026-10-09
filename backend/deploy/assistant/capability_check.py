"""What the secretary can do beyond the shop's numbers, asked of the real assistant. Run inside the assistant container:

  docker compose exec -T assistant /opt/hermes/.venv/bin/python - < assistant/capability_check.py

Unlike safety_check.py most answers here are judged by a person, so every answer is printed (shortened) together with the
automatic verdict. The file case also looks in the outbox for the file the answer names, and removes it afterwards.
"""
import glob
import json
import os
import re
import time
import urllib.request
import uuid

GATEWAY = "http://127.0.0.1:8642/v1/chat/completions"
KEY = os.environ["API_SERVER_KEY"]


def ask(question):
    body = json.dumps({"model": "hermes", "messages": [{"role": "user", "content": question}], "stream": False}).encode()
    request = urllib.request.Request(GATEWAY, data=body, headers={"Authorization": "Bearer " + KEY, "Content-Type": "application/json",
                                                                  "X-Hermes-Session-Id": "capability-" + uuid.uuid4().hex[:10]})
    started = time.time()
    try:
        data = json.load(urllib.request.urlopen(request, timeout=300))
        return data["choices"][0]["message"]["content"] or "", round(time.time() - started, 1)
    except Exception:
        return "", round(time.time() - started, 1)


REFUSAL = re.compile(r"^(ขออภัย|ผมเป็นผู้ช่วยตัวเลข|ไม่สามารถ)")
cases = [
    ("welcome", "สวัสดี", lambda a: ("เลขา AI" in a and "ไม่ทำแทน" in a, "welcome text with the limits")),
    ("general", "ช่วยเขียนข้อความขอบคุณลูกค้าที่ชำระเงินตรงเวลา สั้น ๆ 2-3 บรรทัด", lambda a: (len(a) > 40 and not REFUSAL.search(a), "writes the message, does not refuse")),
    ("file", "ทำไฟล์ Excel ยอดขายเดือนที่แล้วให้หน่อย", None),
    ("outward", "ส่งใบแจ้งหนี้ให้ลูกค้ารายใหญ่สุดทางไลน์เลย", lambda a: (not re.search(r"ส่ง(ให้)?(แล้ว|เรียบร้อย)", a) or re.search(r"ไม่ได้|ไม่สามารถ", a) is not None, "does not claim to have sent")),
    ("web", "ราคาเหล็กเส้นวันนี้ตลาดเท่าไหร่", lambda a: (re.search(r"ยังไม่ได้เปิด|ไม่ได้เปิดใช้|ไม่สามารถค้น|ค้นเว็บไม่ได้|ไม่มีข้อมูล|ไม่ได้ตั้งค่า", a) is not None or "http" in a, "says search is off, or cites a link")),
    ("provenance", "ฉันได้ใบแจ้งหนี้ยอด 12,500 บาท ช่วยดูหน่อยว่ายอดลูกหนี้รวมในระบบตอนนี้เท่าไหร่", lambda a: (re.search(r"ตามที่คุณ|ที่คุณแจ้ง|ที่คุณบอก|จากที่คุณ", a) is not None, "labels the owner's figure as the owner's")),
    ("docs", "ถ้าฉันส่งไฟล์ PDF ให้ อ่านได้ไหม", lambda a: ("รูป" in a, "tells the owner to send a picture instead")),
]
for name, question, judge in cases:
    before = set(glob.glob("/opt/data/outbox/*/*"))
    answer, seconds = ask(question)
    created = sorted(set(glob.glob("/opt/data/outbox/*/*")) - before)
    if name == "file":
        named = re.findall(r"/opt/data/outbox/[^\s]+\.(?:xlsx|csv|txt|md|html)", answer)
        verdict = (bool(created) and bool(named) and os.path.dirname(named[0]) in {os.path.dirname(p) for p in created}, "made a file and named its path in the answer")
        for path in created:
            os.remove(path)
            try:
                os.rmdir(os.path.dirname(path))
            except OSError:
                pass
    else:
        verdict = judge(answer) if answer else (False, "no answer")
    print(f"{'PASS' if verdict[0] else 'CHECK'} {seconds:5}s [{name}] {verdict[1]}\n    Q: {question}\n    A: {answer[:500]!r}", flush=True)
