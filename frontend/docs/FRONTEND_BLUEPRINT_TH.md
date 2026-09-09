---
status: historical
last_verified: 2026-07-15
source_of_truth: [src/router/index.ts, src/api/index.ts]
tags: [frontend, historical, blueprint]
---

# Nextstep Dashboard Frontend Blueprint

> Historical proposal: เอกสารนี้เก็บบริบทการออกแบบเริ่มต้นเท่านั้น หลาย route และพฤติกรรมไม่ตรงกับ Production ปัจจุบัน ห้ามใช้เป็น source of truth สำหรับการ implement ให้เริ่มที่ `docs/knowledge/00-project-map.md` และตรวจ source/tests จริง

## สถานะเอกสาร

Proposed — รออนุมัติก่อนเริ่ม implementation

เอกสารนี้เป็นแผน frontend ของระบบ Nextstep Dashboard โดยอ้างอิง system blueprint ใน repository backend ที่ `docs/BLUEPRINT_TH.md`

## 1. UX Boundaries

Frontend มีสองพื้นที่แยกกัน:

### Platform Admin

- ใช้ Sakai app shell, sidebar และ topbar
- เข้าได้เฉพาะ Platform Admin
- ออกแบบเป็น operations/admin UI ที่ compact และตรวจสอบง่าย
- ใช้ PrimeVue DataTable, Toolbar, Dialog, ConfirmDialog, Toast, Tag และ form controls

### Store Viewer

- เปิดจาก LINE ผ่าน LIFF
- Mobile-first และไม่มี admin shell
- Dashboard แสดงเฉพาะร้านและรายงานที่ API อนุญาต
- ไม่แสดง AI, recommendation หรือข้อความวิเคราะห์

## 2. Route Map

```text
/signin

/admin
/admin/stores
/admin/stores/new
/admin/stores/:tenantId
/admin/stores/:tenantId/sml
/admin/stores/:tenantId/recipients
/admin/stores/:tenantId/permissions
/admin/stores/:tenantId/schedules
/admin/stores/:tenantId/line
/admin/stores/:tenantId/history
/admin/system

/app
/app/stores/:tenantId
/app/stores/:tenantId/reports/:reportKey
/app/access/:deliveryRef
```

## 3. Admin Page Patterns

### Store List

- Sakai `card` + `Toolbar` + `DataTable`
- Search, status filter and expiration filter
- Columns: name, status, access end, SML readiness, LINE readiness, next schedule, latest delivery
- Row actions: view/edit, enable/disable with confirmation

### Store Create/Edit

- PrimeVue form layout using responsive 12-column grid
- Basic information, lifetime access and status
- Inline validation and explicit disabled-action reasons

### SML Connection

- Connection state Message/Tag
- Endpoint/database/credential form
- Test button with loading, success timestamp and safe failure message
- Credential fields never display stored values

### Recipients and Permissions

- Recipient DataTable with status and LINE identity profile
- Permission edit Dialog containing the 10 report checkboxes
- Pending, active, disabled and invalid-recipient states

### Schedules

- Schedule DataTable and focused create/edit Dialog
- Days, local time, period preset, reports and recipients
- Preview reports/recipients before save
- ConfirmDialog for disable/delete

### LINE Setup and History

- System shared OA vs tenant override status
- Test-send action requires confirmation
- Run and delivery tables show safe errors and timestamps

## 4. Viewer Dashboard

### LIFF Bootstrap

- Initialize LIFF on every viewer entry
- Obtain raw ID/access token and exchange it for a backend session
- Never send decoded profile/user ID as proof of identity
- Show explicit login, not-authorized, expired-link and service-unavailable states

### Dashboard Home

- Store selector only when the user has more than one tenant
- Report navigation generated from backend permission response
- Latest data timestamp and period always visible
- Summary tiles use PrimeVue/Sakai tokens without inventing a second design system

### Report Detail

- Shared report shell for title, period, freshness and status
- Report-specific KPI section
- Responsive table/list appropriate to each report
- Mobile layout avoids wide raw tables when a stacked row is clearer
- Empty/stale/error states never imply fresh data

## 5. Ten Report Navigation

1. ขายสินค้าและบริการ
2. ซื้อสินค้า/ตั้งหนี้
3. กำไรขั้นต้นตามสินค้า
4. กำไรขั้นต้นตามลูกหนี้
5. สต็อกคงเหลือ
6. สินค้าถึงจุดสั่งซื้อ
7. ความเคลื่อนไหวลูกหนี้
8. รับชำระหนี้
9. รับเงิน
10. จ่ายเงิน

Menu rendering is authorization-aware but hiding a menu is not a security boundary. Backend authorization remains mandatory.

## 6. API Client Contract

- Same-origin `/api/v1`
- Cookie-based sessions with credentials enabled
- One typed error envelope
- Abort/timeouts for page requests
- Loading, empty, success, stale and error states represented explicitly
- No auth tokens in localStorage

## 7. Stack

- Vue 3.4+
- Sakai Vue 5.0.0 local template source
- PrimeVue 4.5+, PrimeIcons 7 and Tailwind PrimeUI
- Vue Router
- Pinia only for authenticated session/global catalog state
- Vitest + Vue Test Utils
- Playwright or browser smoke for critical flows
- Nginx static image and `/api` reverse proxy

## 8. Testing Strategy

- Unit: permission menu mapping, period labels, currency/number formatting and API state mapping
- Component: login, store form, schedule form, permissions dialog and error states
- Contract: frontend fixtures match backend OpenAPI examples
- E2E: admin login, create store, configure schedule and viewer authorization denial
- Runtime: desktop/mobile visual check, no console errors and no horizontal overflow

## 9. Accessibility and Responsive Rules

- Keyboard-accessible dialogs/forms
- Correct labels and validation messages
- Visible focus indicators
- Sufficient contrast through Sakai theme tokens
- Mobile viewer target width starts at 360px
- Admin DataTables use responsive scroll only when a stacked alternative is unsuitable

## 10. Acceptance Criteria

- Admin pages use Sakai/PrimeVue patterns as the primary structure
- Platform Admin auth guard blocks every `/admin` route
- Viewer cannot render a report absent from backend permissions
- All 10 authorized reports have a usable summary/detail route
- Empty, stale and failed data are visually distinct
- Mobile LIFF flow has no horizontal overflow
- Frontend build, lint and tests pass in CI
- Production image serves frontend at port 6324 and proxies API same-origin

## 11. Implementation Gate

Frontend scaffold starts only after the shared Blueprint and both repository plans are approved.
