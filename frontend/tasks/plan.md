# Implementation Plan: Nextstep Dashboard Frontend

## Status

Proposed — planning only. No source code, commit, push or deployment starts until approval.

## Architecture Decisions

- Copy the real local Sakai Vue template as the starting point.
- Preserve Sakai shell and component vocabulary; remove demo/landing content in a dedicated increment.
- Separate admin and LIFF viewer layouts.
- Use same-origin cookie auth and `/api/v1`.
- Build navigation from the backend report catalog/permission response.

## Phase 0: Plan Approval

### Task 0.1 — Approve routes, UI boundaries and domain

**Acceptance:** Admin/viewer routes and HTTPS hostname are confirmed.

**Verification:** User approval before scaffold.

## Phase 1: Sakai Foundation

### Task 1.1 — Scaffold from Sakai Vue 5

- Copy required template source, assets and configuration.
- Preserve app shell and theme.
- Add project commands and lockfile.

**Verify:** install, lint and production build pass.

### Task 1.2 — Remove demo routes and define app routing

- Keep only required reusable layout/components.
- Add admin/viewer route groups and not-found/access-denied pages.

### Task 1.3 — Add API client and session contracts

- Same-origin client, credentials, timeouts and typed error mapping.
- Admin and viewer session stores remain separate.

### Checkpoint 1

- Sakai shell loads with final menu skeleton.
- Build/lint/tests pass with no demo route leakage.

## Phase 2: Admin Foundation

### Task 2.1 — Platform Admin sign-in and route guard

- Sakai login pattern, loading/error/rate-limit states and logout.

### Task 2.2 — Admin overview

- Operational counts/statuses without decorative dashboard clutter.

### Checkpoint 2

- Unauthenticated user cannot enter admin routes.
- Authenticated session survives reload through backend cookie.

## Phase 3: Store Configuration Vertical Slices

### Task 3.1 — Store list and create/edit

- DataTable, search/filters, form Dialog/page and status actions.

### Task 3.2 — SML connection setup

- Safe credential form, connection test and readiness states.

### Task 3.3 — Recipients and report permissions

- Recipient DataTable and permission Dialog for all 10 reports.

### Task 3.4 — Notification schedules

- Schedule table and create/edit Dialog for days/time/period/reports/recipients.

### Task 3.5 — LINE routing and delivery history

- Shared/tenant channel readiness, confirm test send and paginated history.

### Checkpoint 3

- Admin can configure one tenant end-to-end against backend integration environment.
- Empty, disabled and failure states are understandable.

## Phase 4: LIFF Viewer

### Task 4.1 — LIFF initialization and backend session exchange

- Login/loading/denied/expired states.
- Raw token to backend; no client-trusted user ID.

### Task 4.2 — Tenant and report navigation

- Tenant selector and permission-derived report menu.

### Task 4.3 — Shared report shell

- Period, freshness, quality and error/empty states.

### Task 4.4 — Sales and purchase views

### Task 4.5 — Gross profit views

### Task 4.6 — Stock views

### Task 4.7 — AR views

### Task 4.8 — Cash/bank views

Each report-family task includes responsive KPI and detail presentation plus fixture/component tests.

### Checkpoint 4

- All 10 report routes render authorized fixtures.
- Unauthorized menu and direct URL access fail correctly.

## Phase 5: Flex Preview and Operational UX

### Task 5.1 — Numeric Flex preview

- Display backend-rendered preview matching one-bubble design.

### Task 5.2 — Run/delivery/audit tables

- Pagination, filters and safe failures.

## Phase 6: Quality and Delivery

### Task 6.1 — Accessibility and responsive pass

- Keyboard, labels, focus, contrast and 360px LIFF viewer.

### Task 6.2 — Frontend tests and critical E2E flows

- Admin login/store/schedule and viewer access-control tests.

### Task 6.3 — Nginx production image

- SPA fallback, cache/security headers and `/api` proxy.

### Task 6.4 — GitHub Actions and GHCR

- Lint/test/build/audit gates then immutable image publish.

### Task 6.5 — Production browser verification

- Admin desktop/mobile, LIFF viewer, console/network and no horizontal overflow.

## Release Gate

- Tests/lint/build/audit pass.
- Browser console has no application errors.
- No token or secret appears in bundle/source map.
- All viewer authorization cases pass with backend.
- Sakai patterns remain the main component/layout system.

## Risks and Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Demo Sakai content leaks into product | Confusing/unprofessional admin | Remove demo routes in its own verified increment |
| UI hides unauthorized menu but API leaks data | Cross-tenant disclosure | Treat backend as authority; direct URL denial E2E tests |
| Ten reports make mobile navigation crowded | Poor LINE viewer UX | Permission-filtered menu and report-family routes |
| LIFF external-browser differences | Login failures | Explicit init/login/session states and real LINE testing |
| API contract drift | Broken releases | Shared OpenAPI examples and contract fixtures in CI |

