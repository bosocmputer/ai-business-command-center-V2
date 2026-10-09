# AI-Business Command-Center V2

เลขา AI ประจำร้านสำหรับร้านที่ใช้ SML ERP: ส่งรายงานเช้าทาง LINE, ตอบคำถามเรื่องตัวเลขทาง Telegram, เตือนเรื่องที่ต้องรู้, ร่างข้อความให้เจ้าของคัดลอกไปส่งเอง
หลักการ: **AI-BCC เป็นแหล่งตัวเลขเดียว** ผู้ช่วยเรียบเรียงเป็นภาษาคน ไม่คำนวณเอง · อ่านจาก SML อย่างเดียว ไม่เขียนกลับ · ไม่ส่งข้อความหรือทำรายการแทนคน

Fork ของ nextstep-dashboard

- `backend/` Go (chi + pgx) + PostgreSQL 16, เชื่อม SML ผ่าน JavaWS แบบอ่านอย่างเดียว
- `frontend/` Vue 3 + Vite + PrimeVue (หน้า admin และหน้าผู้รับ)
- `backend/deploy/assistant/` ผู้ช่วย (Hermes) พร้อมคำสั่งดูแล · `spikes/hermes/` ผลทดลองและชุดทดสอบจำลอง

## สถานะ (8 ต.ค. 2569)

ใช้งานจริงกับร้านแรก 1 ร้าน ยังไม่เปิดให้เจ้าของร้านใช้ผู้ช่วย (ทดสอบกับทีมผ่าน Telegram)

| ส่วน | สถานะ |
|---|---|
| รายงาน 13 ตัว + การ์ด LINE เช้า | ใช้งานจริง |
| Agent API (context, report, compare, delivery, alerts, search, lookup, drafts) | ใช้งานจริง ปิดโดยค่าเริ่มต้น เปิดด้วย `AGENT_API_ENABLED` |
| ผู้ช่วยทาง Telegram (โมเดลตรึงที่ผู้ให้บริการไม่เก็บข้อมูล, ไม่มีเครื่องมือในตัวนอกจากความจำ) | ใช้งานกับ 2 บัญชีทดสอบ |
| เตือน 6 เรื่อง + สรุปเช้า ตั้งด้วยคำพูด ส่งเป็นข้อความสำเร็จรูปไม่ผ่านโมเดล | ใช้งาน (ปลายทางเฉพาะผู้ทดสอบ) |
| ร่างข้อความทวงหนี้ / ร่างรายการสั่งซื้อ | ใช้งาน ร่างเท่านั้น ไม่ส่งเอง |
| ค้นลูกค้า/ผู้จำหน่าย/สินค้า (สำเนาข้อมูลหลักวันละครั้ง) และข้อมูลสดรายลูกค้า/รายสินค้า | ใช้งาน |
| เครื่องมือเปิดร้านใหม่ (`onboard-check`), เก็บหลักฐานเลขผิด (`evidence`), ตรวจก่อนอัปเดต Hermes (`upgrade-check`) | ใช้งาน |
| ยอดขายสุทธิ (ใบขาย + ใบเพิ่มหนี้ − ใบรับคืน/ลดหนี้) พร้อมส่วนประกอบในรายงานขาย | ใช้งานตั้งแต่ 2026-10-09 |
| ร้านที่ 2 (เชื่อมกับสำเนาฐานข้อมูลที่ใช้ทดสอบ ตรวจตัวเลข 6 รายงานตรงข้อมูลดิบแล้ว) | ทดสอบอยู่ ยังไม่เปิดใช้จริง |
| LINE ถามตอบ, ปิดระบบเดิม | ยังไม่ทำ |

## เอกสาร

เริ่มที่ [backend/docs/knowledge/00-project-map.md](backend/docs/knowledge/00-project-map.md)

- สถาปัตยกรรมและความปลอดภัย: `backend/docs/knowledge/01..04`
- เปิดร้านใหม่: [05-new-shop-onboarding.md](backend/docs/knowledge/05-new-shop-onboarding.md)
- เงื่อนไขก่อนขยายเกิน 3 ร้าน: [06-multi-shop-conditions.md](backend/docs/knowledge/06-multi-shop-conditions.md)
- การตัดสินใจของ Agent API: [adr/0001-agent-api.md](backend/docs/knowledge/adr/0001-agent-api.md)
- ผู้ช่วย: [backend/deploy/assistant/README.md](backend/deploy/assistant/README.md) · ติดตั้งและกู้คืน: [backend/deploy/RUNBOOK.md](backend/deploy/RUNBOOK.md)
- นิยามตัวเลขและรหัสเอกสาร SML: `backend/docs/sml/`

## สวิตช์ที่ปิดไว้โดยค่าเริ่มต้น

`AGENT_API_ENABLED` · `AGENT_ALERTS_ENABLED` (+ `AGENT_ALERT_DRY_RUN=true` บันทึกอย่างเดียวจนกว่าจะสั่งส่งจริง) · `MASTER_SYNC_ENABLED` ·
`AGENT_LIVE_LOOKUPS_ENABLED` · `AGENT_MONTHLY_CALL_BUDGET` (0 = ไม่จำกัด) ดูค่าและเหตุผลใน `backend/deploy/compose.local.yml`

ที่เก็บนี้เป็นสาธารณะ: ห้ามใส่ชื่อร้าน ตัวเลขธุรกิจ รหัสผ่านหรือโทเคน
