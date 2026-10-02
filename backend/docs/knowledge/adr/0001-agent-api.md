---
status: current
last_verified: 2026-10-02
source_of_truth: [internal/viewer/report_service.go, internal/database/snapshot_store.go, internal/report/dashboard.go, internal/database/migrations/000031_recipient_ai_chat.sql]
tags: [adr, agent, security, api]
---

# ADR: Agent API for the owner-facing assistant

## Status

Accepted and implemented (migration 000041). Written after the Hermes spike (`spikes/hermes/FINDINGS.md`). Decisions taken by
the project owner on 2026-10-02: customer names visible for the pilot shop (token setting, default masked), the assistant may start
background fetches within the hourly budget, `compare` is in the first batch, internal network only (Telegram first), 90-day tokens.

## Context

An assistant (Hermes, one process per shop) should answer an owner's questions about their numbers in chat. AI-BCC stays the only
trusted source of numbers and the only place that decides who may see what. The assistant never reaches SML.

Evidence from the spike (fake data, two free models, 15 Thai questions):

- Tools must come from AI-BCC only; with every built-in toolset off, the model could not run commands or read files.
- A report the caller may not read must answer exactly like one that does not exist; the model then cannot leak that it exists.
- Both models relied on the period and the collected-at time in each answer; one invented a "previous period" range the API never gave.
- Comparisons computed by a model are a risk; a "this week" against the previous equal-length period gave a misleading +140%.
- Exposing the tools directly (not behind Hermes' `tool_search`) saved about 30% of tokens and one to two model calls.
- Report text can carry instructions (planted in the test); names and notes written by shop staff are untrusted data.

Existing parts to reuse: the per-recipient "talk to the AI assistant" switch (`recipient_report_permissions` stays the permission source,
migration 000031 holds the switch), summary snapshots and `ReportStore.RevalidateSnapshot` (returns a fresh snapshot or enqueues a bounded
background summary run with schedule and SML-circuit guards), and the dashboard JSON that cards and web already use.

## Options

1. **Read-only HTTP API under `/api/v1/agent/*` with its own bearer token per recipient and tenant, backed by snapshots** (chosen).
2. Give Hermes a viewer session. Rejected: a session is a browser identity with CSRF and cookie rules, long-lived and broad; it cannot be scoped
   to "assistant only" or rate-limited separately.
3. Let Hermes query SML or the database. Rejected: breaks the single source of numbers and the read-only guarantee, and every shop's schema differs.

## Decision

Option 1. The contract:

### Identity and authorization

- `Authorization: Bearer <token>`. A token is 32 random bytes, stored only as a hash, bound to one tenant and one recipient, with an expiry
  (default 90 days) and a last-used time. Issued and revoked by an admin only; issuing a new one revokes the old.
- Every call re-checks: token active and unexpired, recipient `ACTIVE`, membership `ACTIVE`, tenant `ACTIVE` and before `access_ends_at`,
  and the recipient's AI switch is on. Any failure answers the same `401 UNAUTHORIZED` body.
- Report permission is checked per report from `recipient_report_permissions`. Not permitted, unknown, or no data all answer `404 NO_DATA`
  with an identical body (a test compares the bytes).
- The routes are served on the internal network only in this step and are blocked by the public proxy. A flag `AGENT_API_ENABLED` (default off)
  turns the whole surface off; turning it off must not affect cards or the web.

### Read tools (all `GET`)

| Route | Purpose |
|---|---|
| `/context` | shop name, timezone, today (Bangkok), the reports this caller may read with their period modes, remaining call budget |
| `/reports/{reportKey}?dateFrom&dateTo` | the report's KPIs and chart data for a period |
| `/compare?reportKey&metric&aFrom&aTo&bFrom&bTo` | the difference between two periods, computed on the server |
| `/deliveries/latest?reportKey` | what the owner's last card said for that report |

Report response (`status` is `READY`, `PREPARING` or `UNAVAILABLE`):

```
{ "reportKey", "label", "status",
  "period": { "dateFrom", "dateTo", "mode" },      // the period actually used, always returned
  "collectedAt", "freshness": "FRESH|STALE",       // from the snapshot, never "now"
  "kpis": [{ "key", "label", "unit", "value" }],
  "visualizations": [{ "key", "title", "intent", "unit", "categories", "series" }],
  "warnings": [],                                   // text written by AI-BCC only
  "retryAfterSeconds": 0 }
```

- The numbers are the stored dashboard of a snapshot, the same figures as the card and web page. No detail rows are returned in this step.
- Read path: use an exact snapshot for the period; if it is missing or stale, call `RevalidateSnapshot` (existing guards) and answer
  `PREPARING` with a retry time, or the stale snapshot marked `STALE`. The agent never triggers SML work outside that path.
- `compare` accepts only reports where `ComparisonSupported`, takes both periods from snapshots, and returns
  `{ metric, a:{period,value}, b:{period,value}, delta, percent, basis, warnings }`. It states the day counts of both periods and warns when
  they differ or when a period is partial, so the assistant never subtracts or divides.
- Errors: `401 UNAUTHORIZED`, `404 NO_DATA`, `422 INVALID_PERIOD` (one generic message), `429 RATE_LIMITED` with `Retry-After`,
  `503 AGENT_DISABLED`. No response names a permission, a tenant or another shop.

### Redaction

Names of customers and suppliers are masked by default as stable per-tenant aliases (for example `ลูกค้า-7F3A`). The level is a property of the
token set by an admin, never a request parameter, because a model could ask for more. Names can be turned on per tenant once the data-egress
decision (customer names to the model provider) is made.

### Limits and audit

- 60 calls per hour per token; at most 3 refresh enqueues per hour per tenant through this API.
- `agent_calls` records each call: token, tenant, recipient, tool, report key, period, outcome, duration, snapshot run id. No values, names or
  question text. Retention 365 days like audit.
- Issue and revoke are written to `audit_logs` as admin actions.

### Storage

Migration adds `agent_tokens` and `agent_calls` and a flag in config. Question and answer logging, alert rules and the master-data
copy are later steps.

## Consequences

- Positive: one source of numbers; a stolen token reads only that recipient's permitted reports, only through snapshots, and can be revoked at
  once; the assistant can fail without affecting reports; the spike harness becomes the acceptance test (point it at this API).
- Negative: stale or missing snapshots make the assistant answer "preparing" for rarely opened reports until they are fetched; the new reports
  (aging, RFM, purchase frequency) have no scheduled summary run, so the first question for each triggers a background fetch. `RevalidateSnapshot` is called directly by the agent service,
  so the viewer-facing snapshot-first and stale-revalidation flags do not need to be enabled for this API.
- Masked names make some answers harder to act on ("who owes the most" returns an alias) until names are allowed.
- Rollout: flag off, deploy migration, issue a token for the project owner's recipient only, run the spike battery against it.
  Rollback: set `AGENT_API_ENABLED=false` and revoke tokens; nothing else depends on it.

## Regret Check

The `PREPARING` state is the part most likely to hurt: if owners ask about reports nobody schedules, every first question feels slow.
Signal: more than one in five agent calls ending `PREPARING` in the weekly review of `agent_calls`; then add a background refresh for the
reports owners actually ask about. A second risk is relying on a model's refusals; the design assumes the model can be fooled and leaves
authorization, redaction and limits on the server.
