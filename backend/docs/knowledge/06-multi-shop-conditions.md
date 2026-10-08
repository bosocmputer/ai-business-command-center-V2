---
status: current
last_verified: 2026-10-08
source_of_truth: [cmd/evidence, deploy/assistant/upgrade-check.sh, internal/agent/service.go]
tags: [operations, scaling, runbook]
---

# Conditions before going past three shops (blueprint step 7, item 5)

Each condition has either a tool or a stated reason it is not built. Status as of 2026-10-08, one shop in production.

| Condition | Status | Where |
|---|---|---|
| Hermes update policy: a candidate is tried beside the live assistant and the regression questions pass before the pin moves | **Built** | `assistant/upgrade-check.sh`, "Updating Hermes" in `assistant/README.md` |
| A cost brake per shop with a degraded mode | **Built (calls, not tokens)** | `AGENT_MONTHLY_CALL_BUDGET`, below |
| Evidence of a wrong number kept beyond 365 days | **Built** | `deploy/evidence.sh`, `cmd/evidence`, below |
| Runbook and a backup person | **Documents only**; the person is a decision for the owner | this file, `deploy/RUNBOOK.md`, `05-new-shop-onboarding.md` |
| A partial card when one report breaks (per-shop setting) | **Not built, on purpose** | below |
| Split machines at about eight containers | **Trigger defined**, nothing to build yet | below |
| A model abstraction layer | **Not built, on purpose** | below |

## Cost brake (`AGENT_MONTHLY_CALL_BUDGET`)

The most counted assistant calls a shop may make in its calendar month (shop timezone). 0 = no cap (the default). At 80% the
context call tells the assistant to say so; at the cap every call answers `429 BUDGET_USED` with a Thai message and `Retry-After`
until the first of next month. Alerts and the morning digest use no model and keep going. Refused calls are not counted. The cap
is global, not per shop: set it per deployment until a second shop needs a different number (then it becomes a column on the
tenant). The unit is calls because that is what AI-BCC can see; the model's own token cost follows the number of questions but is
measured separately by `assistant/usage-report.sh`. Pick the number from a real week of use: a question costs 2 to 4 calls.

## Evidence of a wrong number (`deploy/evidence.sh`)

Stored reports are deleted after 90 days and call logs after 365, which is before many disputes are settled. When someone says a
number was wrong, make a case:

```
./evidence.sh capture <tenant-id> 2026-10-01 2026-10-07 "sales of 6 Oct disagreed with the till" <your name>
./evidence.sh list <tenant-id>
./evidence.sh show <tenant-id> <case-id> > case.json     # figures, maybe names from a report's top lists
./evidence.sh delete <tenant-id> <case-id> <your name>
```

A case copies, for the shop and window (at most 93 days): the assistant's call log (metadata only), the stored reports those calls
and the cards were answered from with their figures, the cards sent and the alerts raised. It lives in `evidence_cases`, expires
after three years, is deleted with the shop and on request, and every capture and delete is in the audit log. State in the shop
agreement that such copies exist. Questions and answers themselves are not stored by AI-BCC (Hermes deletes conversations after
24 hours), so a case proves what the system said from its numbers, not the words of a chat.

## Partial card when one report breaks: not built

Today a card is sent only if every report in it is ready (`REPORT_SET_INCOMPLETE`), enforced in the worker and in the database
query. That is a deliberate guard: a smaller card that looks complete is worse than a late one. Changing it is a product decision
(which reports may be left out, how the card says so) and touches the atomic publish of a notification. Build it when a real shop
loses a morning card because of one broken report; then make it a per-shop setting that adds a visible "this report is missing"
line, never a silent omission.

## Splitting machines (about eight containers)

Today the stack is nine long-running containers (api, worker, frontend, postgres, sentinel, assistant, assistant-egress, plus the
three init jobs that exit). A second host is worth planning when one of these holds for a week: the machine status page shows RAM
available under 15% or disk over 80% (it was 82% on 2026-10-08, mostly image layers: prune old images before buying disk), the morning
report window (07:30 to 09:00) ends after 09:00 more than twice, or the assistant needs more than one Hermes process (one per shop is
the design). The first thing to move is the assistant with its egress proxy: it only needs the `agent` network and an address for
the api.

## Model abstraction layer: not built

The model is one setting in `assistant/config.yaml` (provider and model) and one secret. A layer that routes between models is
justified only when a second model is actually used (a cheaper one for simple questions, or a fallback). Until then it would be
code nobody exercises. The change that matters is already in place: the model is pinned to a zero-retention provider and a change
goes through `upgrade-check.sh` first.

## Runbook and backup person

`deploy/RUNBOOK.md` covers deploy, rollback, backups and power loss. What is missing is a named second person with access to the
server and the three secrets lists (`.env.production`, `secrets/assistant/hermes.env`, the Telegram bot): decide who, give them
access in a way that can be revoked, and have them run `./onboard-check.sh` and `./assistant/upgrade-check.sh` once so they have
seen them work.
