# AI-Business Command-Center V2

Fork ของ [nextstep-dashboard](https://github.com/bosocmputer) (backend Go + frontend Vue 3/PrimeVue) ปรับมาใช้เป็น AI-Business Command-Center เวอร์ชัน 2

- `backend/` — Go (chi + pgx), เชื่อม SML ERP ผ่าน JavaWS (protocol ตรงกับ AI-BCC เดิม ยืนยันแล้ว)
- `frontend/` — Vue 3 + Vite + PrimeVue

## สถานะ

จุดเริ่มต้น: copy โค้ดจาก nextstep-dashboard-backend/frontend มาทั้งชุด (ยังไม่ปรับอะไร)

งานที่เหลือ (ตามแผน):
- ตั้งค่า tenant เดียว = สินทวีคอนกรีต
- สลับ LINE Flex renderer (`backend/internal/line/flex.go`, `presentation.go`) ให้เป็นฟอร์แมตเดิมของ AI-Business Command-Center
- ปิด/ตัดส่วนที่เกินความจำเป็นสำหรับร้านเดียว (sentinel + Telegram alerting, retention policy เดิม)
- ตรวจสอบ/ปรับ SML connection config ให้ตรงกับร้านจริง
