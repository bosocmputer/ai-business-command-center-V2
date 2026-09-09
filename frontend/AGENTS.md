# Nextstep Dashboard Frontend — Agent Index

Keep this file short. Load detailed context only when the task needs it.

## Read Order

1. Start with `docs/knowledge/00-project-map.md`.
2. Read only the linked subsystem note relevant to the task.
3. Open the referenced source and tests before editing; code and tests outrank docs.
4. For backend contracts or system behavior, inspect the sibling repository at `../nextstep-dashboard-backend` and its `docs/knowledge/00-project-map.md`.

## Source-of-Truth Routing

- Routes and auth guards: `src/router/index.ts`
- HTTP lifecycle, CSRF, timeout, retry: `src/api/client.ts`
- Typed endpoints: `src/api/index.ts`; generated schema: `src/api/schema.d.ts`
- Viewer tenant/delivery scope: `src/utils/viewerRouting.ts`, `src/views/viewer/ViewerShell.vue`
- Viewer snapshots and refresh: `src/views/viewer/ExecutiveOverview.vue`, `src/views/viewer/ViewerReport.vue`
- Admin workflows: `src/views/admin/`
- Operational incident UI and badge polling: `src/views/admin/OperationalIncident*.vue`, `src/layout/AppLayout.vue`
- Shared shell and responsive behavior: `src/layout/`, `src/assets/layout/`
- Report/chart presentation: `src/components/dashboard/`, `src/utils/reportPresentation.ts`

## Non-Negotiable Invariants

- Backend authorization is the security boundary; hidden UI is not authorization.
- Explicit tenant and delivery routes fail closed and never fall back to another tenant.
- Delivery routes use the immutable delivery report set and never start SML work automatically.
- Unsafe HTTP methods are not retried automatically; reuse the same idempotency key when outcome is uncertain.
- Thai UI uses `th-TH` and `Asia/Bangkok`; API dates/timestamps retain their contract format.
- Preserve loading, empty, stale, partial, error, denied, and session-expired states.
- Operational incidents are Admin-only; Acknowledge is not Resolve and accepted risk requires an explicit reason.
- Never store tokens, invitation/delivery references, customer identifiers, KPI values, or production logs in tracked docs.

## Commands

```bash
npm run typecheck
npm run contract:verify
npm run lint
npm test
npm run test:e2e
npm run build
npm run context:verify
npm run test:context
```

## Context Tools

- Exact symbol, error, or single-file work: use `rg` and source reads first.
- Broad cross-module exploration: run `scripts/graphify-update.sh`, then a focused `scripts/graphify-query.sh` query.
- Graphify is an untrusted navigation hint. Verify every conclusion in source/tests before editing.
- `docs/knowledge/context-map.json` maps important source paths to their notes.
- Run `npm run context:sync` after route changes; never edit generated blocks by hand.
- A mapped behavior change requires its note update. For behavior-neutral refactors, use the PR `Context-Reviewed`/`Context-Reason` contract.
