---
status: current
last_verified: 2026-10-08
source_of_truth: [internal/auth/session.go, internal/sml/endpoint.go, internal/sml/config_service.go, internal/database/sml_test_coordinator.go, internal/retention/worker.go, internal/sentinel/service.go, internal/database/sentinel_store.go, internal/database/sentinel_subject_store.go, internal/failure/catalog.go, deploy/RUNBOOK.md]
tags: [backend, security, operations, retention]
---

# Security and Operations

## Authentication and Authorization

- Admin and Viewer use separate secure, httpOnly, same-site sessions and CSRF cookies for unsafe requests.
- LINE identity is verified server-side before a viewer session is issued.
- Every viewer resource rechecks tenant membership and report permission.
- Viewer stored-row query POSTs require the Viewer CSRF check even though they
  are read-only queries, then re-authorize tenant, report, run, and row expiry.
- Row filter columns/operators are selected from the backend report catalog;
  client-supplied value types are ignored and SQL values remain parameterized.
- Invitation/delivery references are opaque entry references, not authorization tokens.
- Explicit tenant/delivery mismatch fails closed without revealing whether another tenant resource exists.

## SML Boundary

- Tenant SML credentials are encrypted at rest with the configured master key/key ID.
- Endpoint policy validates schemes, hosts/addresses, ports, and redirect behavior before JavaWS access.
- Only approved read-only query plans are sent; never add write-back or schema changes to customer SML.
- JavaWS responses are size/row bounded and validated before dashboards or rows publish.
- Logs use safe error codes and operational timing, never endpoint credentials, SQL, raw rows, or KPI values.

## Retention

- Detail rows follow their per-run expiry (normally 24 hours for viewer detail work).
- Dashboard snapshots/generations and scheduled report payload fields are retained/scrubbed at the 90-day policy boundary.
- Delivery/audit/history records use the 365-day policy boundary where their own expiry/status permits deletion.
- Expired access links and sessions are removed in bounded retention batches.
- Retention updates generation heads before deleting expired generations so stale pointers are not served.

## Production Operations

- `deploy/RUNBOOK.md` is the operational source for preflight, backup, migration, health, smoke, rollback, and key rotation.
- Never deploy from these knowledge notes or assume a branch/image is live.
- Verify current image digests, worker heartbeats, queue state, feature flags, and next schedules from the runtime before an operational change.
- Migration changes require a backup and rollback/compatibility analysis.
- Admin DataTable queries are read-only, CSRF-protected POST requests with typed
  filter allowlists, fixed page sizes, a two-second database statement timeout,
  and count/page reads from one repeatable-read snapshot. Viewer stored-row
  queries use the same fail-closed pattern with a three-second timeout and never
  invoke JavaWS, create a Report Run, or send LINE.
- Release maintenance opens the external window before application mutation,
  records the internal window by an exact UUID, and suppresses PostgreSQL
  command tags so successful inserts cannot be misclassified as failures.

## SML endpoint policy (changed 2026-10-09)

`SML_ALLOW_PUBLIC_ENDPOINTS=true` on the V2 server, by the owner's decision, so a shop whose JavaWS is on a public address can be connected (the second shop is). Private ranges in `SML_ALLOWED_CIDRS` still work. What still holds: only an admin can set an endpoint; the host name is resolved and checked on every request; loopback, link-local, metadata and other always-blocked addresses are refused whatever the setting; only `http`/`https`, no user info, no query. What it widens: an admin can point a shop at any public host, so keep the admin password strong. To go back to a single named host, set `SML_ALLOW_PUBLIC_ENDPOINTS=false` and `SML_ALLOWED_HOSTS=<host>`.

## Master data copy (assistant search)

- `master_items` holds a copy of a shop's customers, suppliers and items: code, name, phone number (people), unit and supplier code (items), active flag. No business figures. `master_sync` records the last good copy, the last try and a short error code.
- The worker (`MASTER_SYNC_ENABLED=true`, off by default) copies each kind once a day after 06:30 shop time, only for shops whose SML connection is READY and whose assistant has a live token, using three fixed SELECTs through the read-only SML client. A failed try is retried after 30 minutes and leaves the last good copy and its time untouched. An empty result never replaces an existing copy. Rows that disappear from SML disappear from the copy at the next copy.
- This is personal data (names, phone numbers) copied into the AI-BCC database: it is covered by the same backups, is removed with the shop (`on delete cascade`), is never returned to a token that may not see names (customers and suppliers), and the call log keeps the kind of search only, never the words. Add it to the shop agreement's list of what AI-BCC stores.
- `GET /api/v1/agent/search?kind=&q=` (tool `search_master`) returns at most ten matches. Words must all appear in the name, code or phone digits.

## Assistant live lookups

- `GET /api/v1/agent/lookup/{kind}?code=` (tool `lookup`; kinds `customer_balance`, `customer_recent_sales`, `item_stock`) reads the shop's SML live for one known record. Off by default (`AGENT_LIVE_LOOKUPS_ENABLED`); runs in the API process with a narrow SML client (25 s, 5,000 rows).
- Safety: fixed SELECT text; the code must exist in `master_items` for the tenant and kind and is rendered as a quoted literal; at most two statements per question; 2 at once, 30 real reads per tenant per hour (in memory, resets on restart), answers kept 5 minutes; each lookup needs a report permission (receivable aging, sales, or stock balance/reorder) and, for customers, a token that sees names; otherwise the uniform NO_DATA answer.
- `agent_calls` records `tool=lookup` and the kind as `report_key`; never the code, name or figures.

## Assistant drafts

- `POST /api/v1/agent/drafts/collection` (tool `draft_collection`) returns a payment-reminder text for one customer among the ten owing the most past their due date. The text is built in `internal/agent/draft.go` from the receivable report's `overdue_debtors` and `overdue_debtor_days` charts (same customers, same order) and the assistant-only list `agent_overdue_documents` (up to five oldest overdue documents per customer: number, due date, balance, days; customer names ride in point labels). Keys starting `agent_` are hidden from the web pages and are not returned by the assistant's report tool. The model never writes the numbers.
- It writes nothing anywhere and sends nothing. `agent_calls` records `tool=draft` with the report key and an outcome only: no text, no customer name.
- Needs the receivable report permission (otherwise the uniform NO_DATA answer) and a token that may see names (otherwise a plain refusal). A customer outside the top ten, or a name that matches several, returns the candidate names instead of a draft.
- Only documents up to 365 days past their due date (`report.AgingChaseableDays`) are chased; the receivable report reports older overdue debts apart as old debts (`stale_overdue_amount`, `stale_overdue_debtors`, `stale_overdue_debtor_days`) and the draft tool answers `NOT_CHASEABLE` for them. A reminder says it counts only the recent part.
- `POST /api/v1/agent/drafts/purchase-order` (tool `draft_purchase_order`) writes a purchase list from the assistant-only chart `agent_reorder_items` of the reorder report (up to 20 items, most short first). Needs the `stock_reorder` permission (otherwise the uniform NO_DATA answer). Nothing is sent or written; the call log records `tool=draft`, `report_key=stock_reorder`.
- Reminder tone: friendly or formal. No deadline, fee or legal wording is ever generated.

## Assistant alerts

- The owner sets alerts by talking to the assistant (`alerts`, `alert_set` tools = `GET/PUT /api/v1/agent/alerts`). Rules live in `agent_alert_rules`, firings in `agent_alert_events` (figures and fixed Thai text, no names, kept 365 days). The worker's `alertLoop` (`internal/alert`) runs every five minutes; a rule is checked once a day, from 08:15 to 12:15 shop time.
- Off by default. `AGENT_ALERTS_ENABLED=true` alone is a dry run (events are recorded as `DRY_RUN`, nothing is sent); `AGENT_ALERT_DRY_RUN=false` also needs `AGENT_ALERT_WEBHOOK_URL`, a secret of 32+ characters and routes, or the worker refuses to start. Control it with `deploy/assistant/alerts.sh status|dry|live|off|test`.
- The worker signs each alert (`X-Webhook-Signature-V2` over `<unix seconds>.<body>`) and posts it to the assistant's webhook on the internal `agent` network, which the worker joins for this; the assistant relays the text to Telegram unchanged. Delivery errors are kept as short codes (`UNREACHABLE`, `NO_ROUTE`, `HTTP_500`), never as response bodies. A failed delivery is retried for up to six hours and five tries.
- An alert is written as fixed Thai text by AI-BCC from its own figures; the model does not touch it. It carries no customer names.
- `receipts_drop` and `margin_drop` (added 2026-10-08) watch cash received and the gross margin of yesterday against the same weekday a week earlier (a percent fall, and a fall in percentage points). A day the shop was probably closed (no documents, or no net sales for the margin rule) never raises a drop alert, and the digest says "no sales, shop may be closed" instead of a 100% fall. A unit word only counts when it belongs to the rule ("5 บาท" is refused for a margin rule).
- `morning_digest` (added 2026-10-08) is a fifth rule that is a switch, not a threshold (stored threshold is the placeholder `1`, whatever the owner said is ignored). Once a day it sends one message with yesterday's sales against the same weekday a week before, overdue receivables and the reorder count, each only if the recipient may read that report (a hidden report leaves no trace, not even in the suggested follow-up question). If any permitted figure is not fresh yet the whole digest waits for the next 5-minute tick inside the same window; if none may be read the rule is marked `NO_ACCESS`. It has no cooldown, and the unique (rule, day) record keeps it to one per day. The "data as of" line shows the oldest figure used.

## Container logs

- Every service in the compose files writes Docker `json-file` logs capped at 20 MB x 5 files (100 MB). Docker keeps logs forever by default, and a worker in a sibling deployment once filled a disk with 27.8 GB of repeated error lines.
- Worker lanes (`cmd/worker`, `runLane`) wait 1 s, 2 s, 4 s ... up to 30 s between consecutive errors and return to 1 s after a success or an idle pass, so a lost database cannot produce a tight error loop. Retention and LINE quota loops already wait a minute after an error.

## Nextstep Sentinel

- Sentinel is a separate process and database pool. Report, Notification, and Delivery transactions never call the incident writer, so monitoring failure cannot roll back business work.
- Terminal Report, Notification, and Delivery scans use durable source state,
  bounded batches, per-source cursors, a five-minute overlap, deduplication, and
  backlog-aware advancement so the 500-row limit cannot skip later events.
  Historical notification rows remain `UNKNOWN` and do not generate alerts.
- P1 Telegram delivery is disabled in `off`/`observe` modes. `send` requires root-owned token/chat files and must follow the runbook preflight and observation window.
- Incidents use root-cause/severity families, five-minute episodes, and per-subject lifecycle state. Discrete bursts send at most OPEN plus one UPDATE; continuous probes do not inflate occurrence/version on every heartbeat. Acknowledge stops reminders; only system evidence resolves every active subject. Manual closure is `CLOSED_ACCEPTED` with a reason.
- The host probe also reports the owner-facing assistant container when it exists (`containers.assistant`, optional): absent means it is not deployed and raises nothing; present and false raises the usual container incident (`CONTAINER_ASSISTANT_UNHEALTHY`, P1) after two consecutive bad probes (the probe file is written once a minute; Sentinel reads it every 30 seconds but counts each file once, since 2026-10-09), and the watchdog status lists `ASSISTANT_CONTAINER_UNHEALTHY`. The probe script counts a container that is still starting as healthy, so a planned restart does not alert; an assistant that never becomes healthy does. Stopping the assistant on purpose will alert after two minutes.
- The emergency database alert lane stores only a safe reference and timestamps in a protected volume. Host probe and monitor heartbeat files are bounded, schema-checked, and contain no customer data.
- Admin incident APIs are Admin-only, CSRF-protected for mutations, and `no-store`. Telegram tenant context is disabled by default. The `private_chat` mode is enabled only after `getChat` verifies the exact private destination; it may carry a sanitized tenant name and JavaWS Base URL for tenant-scoped P1 alerts. Failed or non-private verification redacts that context without blocking the P1.
- The Admin-only occurrence diagnosis endpoint is read-only and uses at most
  three fixed PostgreSQL queries. It never contacts JavaWS. Customer copy may
  contain the sanitized tenant name/Base URL and an opaque request reference;
  Codex copy must exclude those customer-identifying fields. Neither copy may
  contain SQL, raw responses, hashes, credentials, rows, or KPI values.
- Incident events copy sanitized failure evidence and LINE/report impact at
  observation time, so the 365-day incident record remains useful after source
  Report or Notification retention. The occurrence resolver may match a
  connection version to a sanitized historical audit URL, or explicitly label a
  current-only fallback. URL userinfo, query, and fragment are removed.
- The authenticated Incident list includes at most two tenant-name examples per
  incident from the same bounded SQL query; Codex clipboard output never
  includes those names. Telegram loads at most five complete tenant/URL pairs
  with one bounded best-effort query only when verified private mode is active.
  Incident detail returns at most 200 newest events
  to keep the response bounded below its payload budget.
- A linked `REPORT_SET_INCOMPLETE` is downstream impact of its failed report and
  is suppressed as a second P1 alert. If no root report is provable inside the
  aggregation window, the notification failure remains eligible as a standalone
  incident rather than being hidden.
- Telegram uses Thai local time and lifecycle-specific wording. SML P1 messages
  state only whether JavaWS is unavailable or reachable again; RECOVERY omits
  the earlier cause/impact repetition and uses the evidence-backed resolved
  timestamp. In verified private mode, OPEN, REMINDER, UPDATE, and RECOVERY may
  include the sanitized URL matching the failure connection version; a
  current-only fallback is labelled explicitly. Acknowledge only stops
  reminders; recovery still requires system evidence for each subject.
- Non-SML Telegram alerts use the confirmed failure stage to name the affected
  Nextstep/LINE/platform area and the team that should investigate. Evidence V1
  may identify a confirmed stage, but only Evidence V2 with an exact baseline
  may make a statement about whether abnormal Nextstep load was observed.
- Admin JavaWS investigation separates opening a sanitized URL in the operator's
  browser from a guarded Server Dashboard test. The test uses fixed `select 1`,
  shares report admission limits, is single-flight with cooldown, yields to an
  active/nearby Schedule, and never resolves an incident. An uncertain remote
  outcome opens the tenant circuit and is not retried automatically.
- Backup policy is `PRE_MIGRATION_ONLY`: the release checks pending migrations,
  then creates a checksummed backup and performs an isolated restore verification
  only when a migration is pending. There are no daily/offsite/monthly stale P2s;
  the two newest verified pre-migration backup sets are retained.

## Living Context Gate

- `docs/knowledge/context-map.json` maps production-sensitive source paths to the notes that must be reviewed.
- `make context-verify` validates map/schema/path/marker safety and checks the generated report catalog without writing.
- Pull requests changing mapped source must update a mapped note or include auditable `Context-Reviewed` and `Context-Reason` lines.
- Graphify remains local-only and is never installed or executed by CI.

## Incident Documentation

Use the sanitized incident template. Record safe error codes, time windows, affected subsystem, evidence sources, containment, fix, and regression tests. Do not copy customer identifiers, payloads, tokens, SQL, KPI values, or full logs into Git.

## Viewer open events

- `report_view_events` (migration 000035) records that a recipient opened a card button, a report page, the executive overview, or asked for fresh data. It exists to measure real use before more is built.
- It stores internal recipient and tenant ids, the event kind, an optional report key and delivery id, and a timestamp. It never stores LINE user ids, names, report values or request bodies.
- Recording is best effort and asynchronous: a failure is logged by category and never fails the viewer's request. The same viewer doing the same thing within one minute is stored once.
- Events are deleted with their recipient or tenant, and by the retention worker after 365 days (`viewEvents` in the retention log line).

## Agent API (owner-facing assistant)

- Routes under `/api/v1/agent/*` serve the assistant: `context`, `reports/{key}`, `compare`, `deliveries/latest`. All are `GET`, read-only, and answer from stored snapshots; the assistant never reaches SML. They are on the internal network only: the frontend proxy answers `404` for `/api/v1/agent/` so nothing reaches them through the public address. `AGENT_API_ENABLED` (default `false`) switches the whole surface off; off answers `503 AGENT_DISABLED` and does not touch cards or the web.
- A token is `abcc_` plus 32 random bytes, stored only as an HMAC hash in `agent_tokens`, bound to one recipient in one tenant, valid 90 days. One live token per recipient and tenant: issuing a new one revokes the old in the same transaction. Admin issues, shows state and revokes at `/api/v1/admin/tenants/{t}/recipients/{r}/agent-token`; the token is in the issue response once and never repeated. Issue and revoke are audited as admin actions.
- Every call re-checks the token (live, unexpired), the tenant (active, inside its access period), the recipient and membership (active) and the recipient's assistant switch. Every failure answers the same `401` body, so a caller cannot tell a revoked token from an expired one or a switched-off assistant.
- Report permission comes from `recipient_report_permissions`. A report the token may not read, an unknown key and a missing report answer with the same status and the same bytes (`404 NO_DATA`); a test compares them. Agent errors are written without a request id for that reason.
- Customer and supplier names are masked as stable per-tenant aliases (`ลูกค้า-7F3A`) unless the token was issued with `namesVisible`. The level belongs to the token, never to a request parameter. The visualizations that carry such names are listed in `report.PersonNameVisualizations`.
- Limits: 60 answered calls per hour per token (refusals for rate do not count), 10 different reports or periods fetched in the background per hour per tenant (asking again for the same one is not a new fetch). A missing or stale snapshot goes through `ReportStore.RevalidateSnapshot`; the answer is `PREPARING` with a retry time, or the stale numbers marked `STALE`.
- `agent_calls` logs token, tool, report, period, outcome, duration and snapshot run id, with no values, names or question text; retention 365 days (`agentCalls` in the retention log line).
- `compare` computes the difference on the server and warns about different period lengths, unfinished periods and overlap. The assistant is told not to do arithmetic.

- Assistant skills (2026-10-09): the built-in tools are limited to memory on every channel, which also keeps the skills index out of the model's prompt (Hermes adds it only when a skills tool exists). A skill can still be loaded by a message that starts with its name as a slash command, so `assistant/disable_skills.py` lists every skill on disk into `skills.disabled` at each container start; `upgrade-check.sh` step 2b verifies that nothing but `hermes-agent` is enabled.
- Document lookup (2026-10-09): `GET /api/v1/agent/lookup/document?code=<document number>` (tool `lookup` kind `document`) finds one document by the number written on it among sales, debit notes, returns (codes 44, 46, 48), purchases (12) and debt receipts (239). The number is checked for its shape (3 to 40 characters of letters, digits, `.`, `_`, `/`, `-`; no `..`) and goes into three fixed read-only statements as a quoted literal; no master record is needed. Only the families the recipient has a report for are searched (a recipient with none sees a missing report), a name column is blanked for a token that may not see names, and the call log keeps the kind only. The answer is the document header (type, date, party, total including VAT, status, cash or credit, reference, due date), its first twelve lines with the count, and what was paid against it. Quotations, sale orders, delivery notes and cash book documents are not searched.
- Report export (2026-10-09): `GET /api/v1/agent/exports/{reportKey}?dateFrom=&dateTo=&cursor=` (tool `export`, MCP tool `export_report`) gives the detail rows of a report the recipient may read, 500 rows a page, up to 20,000 rows, with the Thai headings the report page uses (`internal/agent/export_columns.go` mirrors `frontend/src/utils/reportPresentation.ts`, internal columns left out). It goes through the same path as the web page, `viewer.ReportService`: the permission check is the page's own, the rows are the recipient's own finished DETAIL run for exactly that period (found again with `OwnDetailRun`, so a second ask does not touch SML) or one started the way the page starts it (idempotency key per recipient, report, period and hour, so asking again while it works joins the same run and answers PREPARING). A token that may not see names gets the usual codes in the name columns (`exportNameColumns`). A report the recipient may not read answers like a missing report. One export is one counted call, whatever the pages (only the first page is recorded); the call log keeps the report and period only (migration 000047 adds the tool name). The shim follows the pages and `secretary_tools.export_file` writes an xlsx (sheet "ข้อมูล" with a bold, frozen, filterable header, numbers as numbers, dates as dates; sheet "คำอธิบาย" with report, period, "ข้อมูล ณ", row count, notes and column totals) or a csv. Column totals are worked out by the tool, never by the model. The cap and its notice are enforced by the server (cursor carries the count) and the file refuses over 15 MB. The assistant's rule in SOUL.md: lists of the shop's own data are always `export_report`; `make_file` never carries shop figures.


- Assistant settings per shop (9 Oct 2026): the model, the shop's OpenRouter key and its Telegram and LINE secrets are kept in `tenant_assistant_settings` (the operator's shared LINE channel in `assistant_global_settings`, what the assistant runs in `tenant_assistant_status`), sealed with the same box as the SML credentials and bound to the shop and the field, so a value sealed for one shop does not open as another's. No admin API returns a secret (a secret is "set" or "not set"; the OpenRouter key shows its last four characters). Setting or clearing one needs the signed-in admin's password again (`AdminService.ConfirmPassword`, five wrong passwords lock that check for 15 minutes, counted apart from login), and the format is checked before the password so a typo does not use an attempt. Changes are audited with the field name and version, never the value (the shared channel's with no shop). The shop's assistant reads its own settings, secrets included, with its own token (`GET /api/v1/agent/assistant-config`, `Cache-Control: no-store`, ETag, not a counted call and not logged); a shop that is off, past its end date, without a key, or whose secret cannot be opened gets `enabled: false` and nothing else. A change of the shared LINE channel moves every shop's version. Rule for the assistant chat: a recipient may be given "คุยกับเลขา" only if the reports they may read cover those of every other recipient who holds an active assistant token in the shop (the assistant answers with the token holder's permissions); it is checked when the chat is switched on, not when permissions change later. The secrets travel in plain text from the api to the assistant over the internal `agent` network: separate that network per shop before a second shop's assistant runs.
