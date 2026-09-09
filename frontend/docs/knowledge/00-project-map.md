---
status: current
last_verified: 2026-07-16
source_of_truth: [src/router/index.ts, src/api/index.ts, src/views/viewer/ViewerShell.vue]
tags: [nextstep, frontend, project-map]
---

# Frontend Project Map

Nextstep Dashboard Frontend is a Vue 3, TypeScript, PrimeVue/Sakai application with two surfaces: Platform Admin and LINE/LIFF Viewer. The backend owns authorization, report contracts, and business data.

## Read by Task

| Task | Read next | Verify in source |
| --- | --- | --- |
| Admin tenant, recipient, permission, schedule | [Viewer and Admin flows](01-viewer-admin-flows.md) | `src/views/admin/`, `src/api/index.ts` |
| Operational incident list/detail and P1 badge | [Viewer and Admin flows](01-viewer-admin-flows.md) | incident views, `src/layout/AppLayout.vue` |
| Viewer login, tenant selection, LINE link | [Viewer and Admin flows](01-viewer-admin-flows.md) | `src/views/viewer/ViewerShell.vue`, `src/utils/viewerRouting.ts` |
| Timeout, retry, stale response, idempotency | [Request lifecycle](02-request-lifecycle.md) | `src/api/client.ts`, route component tests |
| Layout, mobile, accessibility, charts | [UI quality gates](03-ui-quality-gates.md) | `src/layout/`, `src/components/dashboard/` |
| API or domain behavior | Backend knowledge vault | `../nextstep-dashboard-backend/api/openapi.yaml` and backend source |

## Trust Order

1. Current source, tests, and generated API schema
2. Backend OpenAPI and report catalog
3. Notes in this vault with current `last_verified`
4. Historical blueprints and task notes
5. Graphify output and conversation history

If sources disagree, stop relying on the lower-ranked source and report the drift.

## Product Boundaries

- `/admin` is a cookie-authenticated platform administration surface.
- `/app` is the viewer entry; zero tenants shows empty, one can open automatically, and multiple tenants require a user choice.
- Tenant scope shows reports granted to the current recipient for that tenant.
- Delivery scope shows only reports materialized for that LINE delivery.
- The frontend never treats route IDs, hidden menus, or delivery references as authorization.
- Viewer data is snapshot-based; user-triggered refresh is explicit and may enqueue SML work.

## Repository Commands

Use the commands in `AGENTS.md`. The backend owns `api/openapi.yaml`; regenerate frontend types only as part of an intentional backend contract change.

## Documentation Policy

- Record stable intent, boundaries, and failure behavior—not copied implementation.
- Reference source paths for claims that can change.
- Do not record customer-specific state or secrets.
- Update the relevant note and `last_verified` after a material behavior change.
- `context-map.json` and the PR Context Impact Gate require mapped source changes to update a note or carry a review reason.
- Route inventory is generated from source by `npm run context:sync`; CI checks it without writing.
