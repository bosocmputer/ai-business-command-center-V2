---
status: current
last_verified: 2026-09-30
source_of_truth: [internal/worker/report_worker.go, internal/database/report_store.go, internal/database/schedule_execution_store.go, internal/notification/worker.go, internal/delivery/worker.go, internal/failure/catalog.go]
tags: [backend, reports, queue, line]
---

# Report and LINE Pipeline

## Projections

- `SUMMARY` serves dashboard overview, background refresh, and scheduled LINE work. Aggregate SQL returns bounded KPI/trend/ranking data and does not write raw `report_run_rows`.
- `DETAIL` is created for explicit report-detail work and may retain pageable rows until their per-run expiry.
- Summary query results are capped by the query plan; totals cover the source set while only bounded presentation rows cross JavaWS.
- `QueryPlanFingerprint` covers normalized SQL, projection, dashboard builder source, formatter source, and contract versions to invalidate incompatible cache output automatically.

## Scheduled Flow

```text
Due schedule
  -> resolve effective period per report
  -> materialize immutable notification report positions
  -> enqueue SUMMARY report runs at schedule priority
  -> report worker validates JavaWS output and builds dashboards
  -> notification worker requires the complete report set
  -> render one bounded Flex bubble per eligible recipient
  -> publish delivery/outbox records
  -> delivery worker pushes to LINE with lease-safe status changes
```

`Work.Partial` causes `REPORT_SET_INCOMPLETE` before recipient selection or rendering. No LINE delivery or outbox payload is created from an incomplete report set. `ALL_REPORTS_FAILED` and `NO_ELIGIBLE_RECIPIENTS` remain distinct failure causes.

## Safe JavaWS Result Diagnostics

Failure Evidence V3 extends the bounded JavaWS protocol metadata for
`SML_RESULT_INVALID`. It records the decompressed XML byte count, a fixed parser
classification, the approximate parser byte offset, complete rows decoded, and
whether `ResultSet` was observed. This evidence is persisted with the report run
and may be shown to authenticated Admins without contacting JavaWS again.

The diagnostic must never retain SQL, XML or SOAP bodies, row/field names,
field values, KPI values, credentials, tokens, or customer identifiers. Existing
V1/V2 evidence remains readable; only failures with the complete parser
diagnostic are promoted to V3.

Notification occurrences are classified at materialization time: due schedules
write `SCHEDULED`, manual test sends write `TEST`, and historical rows remain
`UNKNOWN`. Sentinel may alert terminal `SCHEDULED` failures but must never infer
that an old or test occurrence was a scheduled customer delivery.

## Queue and Lease Safety

- Default priorities are Schedule 100, Viewer Dashboard 90, and Background FAST/STANDARD/HEAVY 30/25/20.
- An active/recent compatible request may be joined where the store explicitly permits it; schedule occurrences retain their own materialization.
- Chunked heavy reports receive a ten-minute total execution window while every JavaWS query remains bounded by the report's per-query timeout. Direct heavy reports retain their five-minute total window.
- `report_execution_modes` (migration 000034) holds DIRECT or CHUNKED per tenant and report; a missing row is DIRECT. `HEAVY_CHUNK_ENABLED` stays the master switch and `HEAVY_CHUNK_TENANT_REPORTS` only seeds the table. Only `ChunkSafe` reports can be CHUNKED.
- In DIRECT mode a size signal (`SML_RESPONSE_TOO_LARGE`, `SML_ZIP_TOO_LARGE`, or `SML_RESULT_INVALID` with `XML_MALFORMED`, `ROW_LIMIT_EXCEEDED` or `FIELD_VALUE_TOO_LARGE`) switches the report to CHUNKED, audits `REPORT_MODE_AUTO_SWITCHED` and requeues the same run. A failure after chunking is never switched back or retried this way.
- `SML_TIMEOUT` after the request was sent keeps its unknown remote state and is still not retried. It only counts toward a switch: two in a row plus a trivial probe query that succeeds. A dead shop is not a size problem.
- Admin API `/api/v1/admin/tenants/{tenantId}/report-modes` lists, sets and measures modes (`internal/executionmode`). Measuring counts the chunk units per chunkable report, refuses while a run is active for the shop, and only applies a mode to reports nobody has decided yet (`ChunkUnitThreshold` = 3,000 is a starting guess).
- A lease-lost worker cannot publish a result.
- Recovery marks abandoned work with safe codes and protects incomplete notification sets.
- Browser cancellation only stops tracking; only queued work is safely cancellable through the report API.

## JavaWS Failure Semantics

- Failure before sending a request may receive one bounded retry.
- Timeout/reset after request send has unknown remote state, is not automatically retried, and opens tenant protection before another query starts.
- Repeated connection failures contribute to tenant/host circuit state.
- Admin/Viewer errors expose safe codes and request IDs, not SQL or rows.
- A terminal Report failure persists sanitized `FailureEvidence` atomically with
  the run state. It records the stage and transport phase known by the Worker at
  failure time; no later reader infers those facts from an error code.
- `BEFORE_REQUEST_SENT`, `REQUEST_SENT_RESULT_UNKNOWN`, and `RESPONSE_STARTED`
  remain distinct. Unknown remote state must not be presented as a stopped
  customer query or as safe to retry immediately.
- The shared failure catalog is the source for Thai Admin/Telegram wording and
  next checks. Unknown codes use a generic Thai fallback and never raw error
  text.
- Evidence version 2 attaches an opaque `NXR-...` request reference to the
  JavaWS HTTP request and persists only bounded protocol metadata: request and
  retry counts, transport timestamps, HTTP/content metadata, SOAP/Base64/ZIP
  validation, response byte counts/hash, and observed admission concurrency.
  It never persists SQL, SOAP/response bodies, rows, or KPI values.
- Admin attribution distinguishes a customer JavaWS/network response from
  Nextstep report build, storage, queue, notification, LINE, database, and
  capacity stages. A claim that no abnormal Nextstep load signal was found
  requires at least five exact successful samples for the same tenant, report,
  projection, query fingerprint, connection version, and resolved period.

## Viewer Snapshot Flow

- An exact authorized snapshot is returned before revalidation decisions.
- Fresh cache produces no JavaWS call.
- Revalidation and generation-cache behavior is feature-flagged; inspect configuration before asserting it is active.
- Delivery context always uses the report runs attached to that immutable occurrence and never revalidates automatically.
- Viewer report tables can filter and exact-page existing `report_run_rows` by
  catalog-approved text, identifier, date, or number columns. The query reads
  only an authorized succeeded run before row expiry, keeps identifiers exact,
  supports PostgreSQL scientific numeric strings, returns stable row ordinals,
  and never starts SML work. Date ranges are inclusive in Bangkok business-date
  semantics and Global Search is limited to catalog-approved columns.

## Reopening a report without fetching again

- The tenant-wide exact snapshot lookup matches only SUMMARY runs (by query plan fingerprint), so a viewer's own DETAIL run was never found again and every reopen needed a fetch from SML.
- `ReportService.ExactSnapshot` now also looks for the viewer's own finished DETAIL run of the same period (`ReportStore.GetOwnDetailSnapshotForPeriod`, restricted to `source = DASHBOARD` and the requester, because its rows are readable by that viewer only) and returns whichever was collected more recently, the own run on a tie. Freshness is still judged by the refresh policy, so an old run shows as stale and the viewer can refresh.
- A drill-down needs rows. When the snapshot found is a summary, the page starts a detail run itself.
