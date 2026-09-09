---
status: current
last_verified: 2026-07-21
source_of_truth: [src/api/client.ts, src/api/index.ts, src/views/viewer/ExecutiveOverview.vue, src/views/viewer/ViewerReport.vue, src/composables/useServerTable.ts]
tags: [frontend, requests, idempotency, snapshots]
---

# Request Lifecycle

## Shared Client

`apiRequest` provides the common transport behavior:

- Same-origin cookies and JSON envelopes
- CSRF header for authenticated unsafe methods
- Per-request `AbortController` and timeout
- One retry by default for retryable safe reads
- No automatic retry for `POST`, `PUT`, `PATCH`, or `DELETE`
- `Retry-After` support and normalized `ApiError`
- Global unauthorized events for admin/viewer session recovery

An HTTP abort stops browser tracking only. It does not prove a backend report or remote SML query was cancelled.

## Mutation Safety

- Create idempotency keys at the user confirmation boundary, not during render or draft editing.
- Reuse the same key while recovering an unknown result.
- Prevent double submit with single-flight UI state.
- Do not retry a mutation automatically after timeout; refetch state first when the workflow supports reconciliation.

## Stale-response Safety

Long-lived pages that react to route, tenant, report, period, or run changes must:

1. Abort the previous HTTP request.
2. Increment or replace the local context generation.
3. Apply a response only if tenant, report, period, and generation still match.
4. Stop poll timers on terminal state, route change, and unmount.

## Snapshot and Refresh

- Initial overview/report rendering prefers an authorized snapshot.
- Fresh snapshots do not trigger SML work.
- Stale/missing data may expose explicit revalidation or refresh flows.
- Overview refresh replaces a complete result set atomically; partial output must be labelled and never mixed silently with another period.
- `runId`, `refreshId`, `snapshotRunId`, and `deliveryId` identify resources but are never authorization tokens.
- Delivery routes are read-only snapshot flows.
- Report detail filtering and exact page navigation call the stored-row query
  endpoint only. They never start a run, revalidate a snapshot, or contact SML.
  Filters are typed from the approved report presentation, unsafe technical
  columns are not offered, and every page response is guarded by tenant,
  report, run, and generation identity.

## Table Query Safety

- Text filter drafts do not append old and new results. Applying a filter resets
  to page one, aborts the prior request, and replaces the visible page atomically.
- Server-backed tables use exact totals and 25/50/100 page sizes. A concurrent
  deletion can clamp an empty page to the last valid page once; it must not
  create an unbounded request loop.
- POST query endpoints are read-only in business semantics but still use the
  normal authenticated CSRF transport because the HTTP method is unsafe.

Admin operational incident reads are `no-store`. Their acknowledge and
accepted-risk mutations use the shared Admin CSRF policy and optimistic version;
the UI refetches after conflicts rather than overwriting newer evidence.
Incident lifecycle text and action-required state come from the additive
`statusPresentation` contract so the list, detail page, and Telegram behavior do
not derive conflicting recovery claims from raw status or error codes.
The per-occurrence diagnosis is an Admin-only safe GET with abort/generation
guards. It reads persisted protocol metadata and matching-run baseline only;
opening it never starts a connection test, report run, SML query, or LINE work.
The view caches a diagnosis by occurrence ID for the current incident and drops
that cache when the incident context changes.
Report Run detail is also a read-only Admin request. It is fetched only when the
operator opens a row, aborts when the dialog closes, and uses a generation guard
so a slower previous selection cannot replace the active evidence. Neither
Report Run nor Incident detail starts JavaWS/SML work.

## LINE/LIFF Entry

- Preserve the intended route through LIFF login.
- Remove invitation/delivery references and transient OAuth parameters from browser history after exchange.
- A valid LINE session does not make an invalid explicit tenant or delivery route valid.
- Session expiry must clear sensitive viewer state before showing recovery UI.
