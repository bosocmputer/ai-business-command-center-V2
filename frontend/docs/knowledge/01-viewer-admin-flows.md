---
status: current
last_verified: 2026-08-03
source_of_truth: [src/router/index.ts, src/utils/viewerRouting.ts, src/views/viewer/ViewerShell.vue, src/views/admin/ScheduleEditor.vue, src/views/admin/TenantDetail.vue, src/views/admin/ReportRuns.vue, src/views/admin/OperationalIncidents.vue, src/views/admin/OperationalIncidentDetail.vue, src/components/operations/IncidentDiagnosisPanel.vue, src/utils/incidentDiagnosis.ts, src/composables/useServerTable.ts, src/utils/adminTableFilters.ts]
tags: [frontend, viewer, admin, routing]
---

# Viewer and Admin Flows

## Generated Route Inventory

This block is generated from `src/router/index.ts`. Do not edit it manually.

<!-- BEGIN GENERATED: ROUTE_INVENTORY -->
| Path | Route name | Surface | Page title |
| --- | --- | --- | --- |
| `/` | `—` | Public | — |
| `/admin/login` | `admin-login` | Admin | — |
| `/admin` | `—` | Admin | — |
| `/admin` | `admin-dashboard` | Admin | ภาพรวมระบบ |
| `/admin/password` | `admin-password` | Admin | ตั้งรหัสผ่านใหม่ |
| `/admin/tenants` | `admin-tenants` | Admin | ร้านค้า |
| `/admin/tenants/:tenantId/recipients/:recipientId/permissions` | `admin-recipient-permissions` | Admin | กำหนดสิทธิ์รายงาน |
| `/admin/tenants/:tenantId/schedules/new` | `admin-schedule-new` | Admin | เพิ่มตารางส่งรายงาน |
| `/admin/tenants/:tenantId/schedules/:scheduleId/edit` | `admin-schedule-edit` | Admin | แก้ไขตารางส่งรายงาน |
| `/admin/tenants/:tenantId` | `admin-tenant-detail` | Admin | รายละเอียดร้าน |
| `/admin/report-runs` | `admin-report-runs` | Admin | การสร้างรายงาน |
| `/admin/deliveries` | `admin-deliveries` | Admin | การส่ง LINE |
| `/admin/audit` | `admin-audit` | Admin | ประวัติการใช้งาน |
| `/admin/operational-incidents` | `admin-operational-incidents` | Admin | เหตุสำคัญ |
| `/admin/operational-incidents/:incidentId` | `admin-operational-incident` | Admin | รายละเอียดเหตุสำคัญ |
| `/app` | `—` | Viewer | — |
| `/app` | `viewer-home` | Viewer | — |
| `/app/invite` | `viewer-invite` | Viewer | — |
| `/app/tenant/:tenantId` | `viewer-overview` | Viewer | — |
| `/app/tenant/:tenantId/delivery/:deliveryId` | `viewer-delivery` | Viewer | — |
| `/app/tenant/:tenantId/delivery/:deliveryId/report/:reportKey` | `viewer-delivery-report` | Viewer | — |
| `/app/tenant/:tenantId/report/:reportKey` | `viewer-report` | Viewer | — |
| `/:pathMatch(.*)*` | `not-found` | Public | — |
<!-- END GENERATED: ROUTE_INVENTORY -->

## Admin

The router protects every `/admin` child route with the admin session guard. Main workflows are:

- Tenant list and tenant detail
- SML connection and safe connection test
- Recipient invitation, status, report permissions, and permission dependencies
- LINE schedule creation/editing, preview, activation, pause, test send, archive, and restore
- Global report run, LINE delivery, and audit views
- Operational incident list/detail with safe references, evidence timeline, acknowledge, and accepted-risk closure

The App shell polls the open P1 count at most once per minute while the tab is
visible. The incident detail page may resolve authenticated tenant names, but
the copy-for-Codex summary contains only safe operational fields. Admin cannot
manually mark an incident recovered: Acknowledge stops reminders, and accepted
risk is a separate closure with a required reason.

The SML connection tab distinguishes saved readiness from an unsaved draft. It
shows the standard JavaWS endpoint resolved from the Base URL, the latest
persisted test state and timestamp, and explains that the test is a read-only
`select 1` with no report or LINE side effect. A guarded `Retry-After` response
becomes a visible countdown and disabled test action, not a generic failure;
the UI must never encourage a retry while the Server protects a potentially
in-flight JavaWS request.

Report Run history shows a persisted Thai failure summary without loading one
detail request per table row. `ดูสาเหตุและหลักฐาน` lazily loads the selected run
and separates confirmed stage/transport evidence from possible checks. It must
not claim that a customer Server is down or a Firewall blocked traffic without
direct evidence, and opening the detail must never test SML automatically.
The `เข้าคิวเมื่อ` and `เสร็จเมื่อ` columns retain Bangkok time and include
milliseconds so Admin can compare closely spaced report executions.

Incident list defaults to active episodes and leads with Thai cause, area to
inspect, active impact, and local time rather than raw codes. Continuous
conditions say `ตรวจพบต่อเนื่อง` instead of displaying probe counts. The detail
page leads with the backend-provided lifecycle state, the affected subject, the
sanitized JavaWS URL, current LINE impact, and the next Admin action. A single
tenant uses a compact action card; multiple tenants use the bounded server-side
occurrence table. Cause breakdown, causal chain, timeline, and technical fields
remain available in one collapsed evidence section instead of competing with the
current status. The page loads bounded occurrences separately from the incident
summary and may show a sanitized historical/current JavaWS URL only to an authenticated Admin.
Opening that URL is explicitly different from the guarded Server Dashboard
connection test; neither action runs automatically and a successful test never
marks the scheduled incident recovered. The timeline may show tenant/report
context and a causal chain, while technical fields remain collapsed.
The occurrence diagnosis is fetched lazily from persisted evidence and never
contacts JavaWS. It leads with the failure area and investigation owner, then
shows protocol/request evidence, LINE containment, and the next Admin action.
Single-occurrence incidents load one diagnosis automatically; multi-occurrence
incidents load only the row the Admin opens. The customer clipboard may include
the sanitized tenant/Base URL and opaque request reference supplied by the
backend; it is a separate action from the redacted Codex clipboard.
Copy-for-Codex excludes tenant names, tenant IDs, endpoints, KPI, SQL,
credentials, request references, response hashes, and raw payloads.

Permission and schedule selection are different concepts:

- Permission = reports a recipient may open in the tenant dashboard.
- Schedule report set = reports included in one LINE occurrence.
- Every selected recipient must have permission for every report selected by an active schedule.

## Admin Table Navigation

- Admin history tables use typed server-side filtering, exact totals, and
  numbered 25/50/100-row pagination. Column values remain draft state until
  Apply; applying or clearing resets to page one and aborts obsolete requests.
- Recipient tables return an exact filtered total for PrimeVue pagination. Search,
  status, and permission filters are evaluated before paging, while selections
  outside the visible page remain intact. The recipient endpoint keeps its
  bounded flat query contract, so the shared table envelope must be adapted at
  the view boundary rather than forwarded verbatim.
- Schedule tables filter by name, lifecycle status, and archived visibility on
  the server. Selecting `ARCHIVED` explicitly enables archived rows.
- Small catalog/picker tables use local PrimeVue filtering and 25/50/100-row
  pagination because their full bounded catalogs are already in browser memory.
- All PrimeVue tables follow the shared Sakai filter-menu/header interaction.
  Numeric, selection, and action columns keep their semantic alignment; action
  and selection columns are never filterable.
- Every table keeps numeric/action/select alignment semantics and must preserve
  loading, empty, denied, and error states at mobile and desktop widths.
- Required array fields from table APIs are normalized defensively before
  rendering; one malformed collection must not crash the whole tenant detail
  page while the backend contract is being repaired.

## Viewer Entry

`ViewerShell.vue` initializes the viewer session, resolves invitation or delivery entry references, loads accessible tenants, and establishes navigation scope.

### `/app`

- Zero accessible tenants: show an empty state.
- One accessible tenant: may open it automatically.
- Multiple accessible tenants: show the chooser; do not select the first tenant silently.
- A tenant membership with no report permissions remains visible with an explanatory state.

### Tenant scope

Routes:

```text
/app/tenant/{tenantId}
/app/tenant/{tenantId}/report/{reportKey}
```

The menu is built from the current tenant report permissions. An explicit malformed or unauthorized tenant route becomes unavailable and must not display stale data from a previous tenant.

### Delivery scope

Routes:

```text
/app/tenant/{tenantId}/delivery/{deliveryId}
/app/tenant/{tenantId}/delivery/{deliveryId}/report/{reportKey}
```

The menu is built only from `DeliveryContext.reports`, preserving delivery order. It must not flash the wider tenant menu while context loads. A report absent from the delivery is rejected even when the recipient may open it in tenant scope. `ไป Dashboard ร้าน` exits delivery scope explicitly.

## Fail-closed Rules

- Clear previous tenant name, menus, KPI, charts, and delivery context before validating a new explicit route.
- Abort obsolete requests and use a generation/current-context guard before applying responses.
- Use the same generic unavailable message for missing and unauthorized resources.
- Recovery returns to `/app` only after the user requests it.
- Delivery routes load existing snapshots and never start report runs or revalidation automatically.
