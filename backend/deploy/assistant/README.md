# Owner-facing assistant (Hermes) for one shop

An overlay on `compose.local.yml`. It runs one locked-down Hermes per shop that answers the owner's questions with
numbers it fetches from AI-BCC's Agent API (ADR 0001) and nothing else. Findings that led to these settings:
`spikes/hermes/FINDINGS.md`.

## What it can and cannot do

| | |
|---|---|
| Data source | AI-BCC `api` over the internal network `agent`, with the shop's own Agent API token |
| Internet | none directly; only hosts in `ASSISTANT_EGRESS_ALLOW` (default `openrouter.ai`) through `assistant-egress`, which logs host and decision (`docker logs`) |
| Built-in tools | none except `memory` (on by decision, with `memory_guard.py`): terminal, files, web, browser, code execution, skills, cron are off on every channel (`platform_toolsets`). The ~58 skills that ship with Hermes are also switched off at every start (`disable_skills.py` writes `skills.disabled`; only `hermes-agent`, the agent's own manual, which Hermes refuses to disable, stays on), because a chat message such as `/claude-code` would otherwise be rewritten into that skill's instructions |
| Model | `google/gemini-3.1-flash-lite`, pinned to Google Vertex with `data_collection: deny` (`config.yaml`) |
| Side calls | title generation, background review, compression and memory are off, because each would send the conversation to a model outside the pinned provider rules |
| Ports | none published to the host |

## Secrets (never in git, never in chat)

`secrets/assistant/hermes.env`, mode 0600, three values written by `set-secret.sh` (hidden prompt):

```
ssh -t <server> 'cd <deploy dir>/backend/deploy && ./assistant/set-secret.sh OPENROUTER_API_KEY'
ssh -t <server> 'cd <deploy dir>/backend/deploy && ./assistant/set-secret.sh AIBCC_TOKEN'      # issued on the recipient's permissions page
ssh -t <server> 'cd <deploy dir>/backend/deploy && ./assistant/set-secret.sh API_SERVER_KEY'  # type "generate"
```

A token is shop-specific: one assistant container, one token. Revoke it on the permissions page to cut the assistant
off at once.

## Start, check, stop

```
export COMPOSE_FILE=compose.local.yml:assistant/compose.assistant.yml
docker compose config -q && docker compose up -d --build        # recreates `api` once: it joins the `agent` network
docker compose ps assistant                                     # healthy after about a minute
docker compose exec assistant /opt/hermes/bin/hermes tools list --platform api_server   # every built-in must be "disabled"
docker compose logs assistant-egress | tail                     # only openrouter.ai ALLOW; anything else DENY
docker compose stop assistant                                   # stop answering; the data stays
```

Ask one question without any chat channel (this sends the question and the figures to the model provider, so use
real data only after the shop's agreement allows it):

```
docker compose exec assistant /opt/hermes/bin/hermes -z "เดือนนี้ขายได้เท่าไหร่"
```

## Updating Hermes

Never change the pinned image (`compose.assistant.yml`) on a hunch. Run the candidate beside the live assistant first:

```
cd backend/deploy
./assistant/upgrade-check.sh nousresearch/hermes-agent:<tag>@sha256:<digest>  [5,9,13]
```

It starts the candidate with a made-up Telegram token and chat id (nothing can be sent) and an empty memory, and checks that the gateway is healthy,
that no built-in tool except memory (on by decision) is enabled on cli, api_server, telegram and webhook, that the shim shows exactly the tools in
`aibcc_mcp.py`, and that real questions pass against the real Agent API. It spends about three of the token's 60 hourly calls per
question. Only after "UPGRADE CHECK PASSED": back up the data volume (`assistant/backup.sh`), change the digest, recreate the
assistant, read the logs for ten minutes, and keep the old digest at hand to roll back (change it back and recreate).

## Search: finding a customer, supplier or item

`search_master` finds a record by a name, a code or a phone number the owner says ("เบอร์ของลูกค้า …", "รหัสสินค้า ปูน …").
It searches a read-only copy of the shop's master data that the worker makes from SML once a day after 06:30 (three fixed
SELECTs: customers, suppliers, items; codes, names, phone numbers, the unit and supplier code of an item, and no business
figures). Switch: `MASTER_SYNC_ENABLED=true` (off by default). Customers and suppliers need a token that sees names and a
report about them; an item search needs a report about items; otherwise the uniform "no data" answer. At most ten matches.
An empty answer from SML never replaces an existing copy. The call log records the kind searched, never the words.

## Live lookups: one customer or item, right now

`lookup` asks the shop's system one narrow question about one customer or item that `search_master` found: `customer_balance`
(total owed, split into overdue up to a year, old debts, not yet due, no due date, credit, plus the five oldest open
documents; built on the receivable report's own base so it agrees with the report), `customer_recent_sales` (sales in the
last 365 days and the latest five sales documents) and `item_stock` (on hand, to receive, to deliver, reserved, reorder point).
Switch: `AGENT_LIVE_LOOKUPS_ENABLED=true` (off by default). The statements are fixed text and the only value used is a code
the master data copy already holds; an unknown code never reaches the shop's system. Answers are kept five minutes, two run at
once, a shop gets 30 real reads an hour (then BUSY), and the call log keeps the kind only. Customer lookups need a token that
sees names and the matching report permission. Not built yet: price, document by number, supplier balance.

## Drafts: a reminder the owner copies and sends

`draft_collection` writes a payment reminder for one of the ten customers who owe the most past their due date
("ร่างข้อความทวงหนี้ให้ …"). AI-BCC writes the text from the receivable report's own figures (overdue amount, longest
days overdue, up to five of the oldest overdue documents with number, due date and balance, and the amount left on
older ones; the date the data is as of); the model hands it over unchanged. It is polite, sets no deadline, adds no
fee and threatens nothing. Nothing is sent and nothing is written to SML: the owner copies it. It needs a token that sees
customer names (a reminder without a name is not a reminder). The call log records that a draft was made, never its text
or the customer. Quotations are not drafted yet.

`draft_purchase_order` writes a purchase list from the reorder report: the items below their reorder point, most short
first, up to twenty, each with its shortage against the reorder point (reorder point minus on hand, not a recommended
order quantity) and what is still to arrive on earlier purchase orders. The report has no supplier, so the draft is
addressed to "ผู้จำหน่าย" and the owner picks one. When nothing is below its point the assistant says so and invents no list.

Only debts up to 365 days past their due date get a reminder. Older ones are old debts for the accountant: the receivable
report keeps them apart (KPI `stale_overdue_amount` and two charts), the draft tool answers `NOT_CHASEABLE` for them and
the assistant says so instead of writing a letter about a debt from years ago.

Receivables with no due date in SML are never counted as overdue and never chased (the owner's decision). Everything that talks
about overdue money says so and states how much has no due date: the draft's notes, the overdue alert and the morning digest.

## Alerts: the assistant speaks first

The owner sets alerts in their own words ("tell me when overdue receivables pass 500,000") and the assistant stores them
through `alert_set`. AI-BCC's worker checks the rules once a day after 08:15 and, when one is true, sends a message
through the assistant's webhook (a route per person in `ALERT_CHAT_IDS`, default everyone allowed to chat, at most five).
The text is written by AI-BCC from its own figures; no model touches it, and the owner can answer it in the same chat.

Four rules to begin with: overdue receivables, receivables over a year, items at the reorder point (all quiet for three
days unless 10% worse) and yesterday's sales down against the same weekday a week earlier. Thresholds are the owner's;
nothing is guessed for them.

A fifth rule, `morning_digest` ("send me a summary every morning"), has no threshold: one short message a day with yesterday's
sales against the same weekday last week, overdue receivables and the reorder count, limited to the reports that person
may read. It waits until those reports are fresh, then sends once.

**Known limit:** alerts are delivered to the people in `ALERT_CHAT_IDS`, not to the person who set the rule (the assistant does not
yet know which Telegram user a rule came from). Before an owner is invited, put the owner's id in `ALERT_CHAT_IDS` (and expect that
the rest of the team in that list sees the same alerts), or the owner would set an alert and never receive it.

Everything is off until you switch it on, and on it only records:

```
cd backend/deploy
./assistant/alerts.sh status        # switches, rules, last firings
./assistant/alerts.sh dry           # check every morning, record what would be sent, send nothing
./assistant/alerts.sh live          # also send
./assistant/alerts.sh off
./assistant/alerts.sh test <id>     # one labelled test message to one Telegram id from the allowed list
```

## Slow reports: waiting and the morning prewarm

A report that has to be fetched live from the shop's system takes 15 to 60 seconds (measured 15-58 s on 2026-10-05). The
assistant does not tell the owner to ask again: `get_report` and `compare` in `aibcc_mcp.py` ask AI-BCC every 6 seconds
while it answers `PREPARING`, for up to 100 seconds, and return the finished report, so the owner sees "typing" and then
the answer. AI-BCC does not count those status checks against the 60 calls an hour (the first 240 an hour are free). If the
report is still not ready after the wait, the assistant says so and asks the owner to try again in a minute or two.

Reports "as of today" (receivables, stock) and the rolling 180-day customer reports are keyed to the date, so each day's
first question would start a live fetch, and last month's snapshots go stale too (they are not kept for good). `prewarm.py`
fetches all 12 every morning at 07:30 Bangkok, one after the other. AI-BCC allows 10 live fetches an hour by default (`AGENT_REFRESHES_PER_HOUR`; the V2 server has lifted it), so the last two
are refused at first and it retries every five minutes until the budget frees, about an hour later, for at most two hours:
so the owner's first questions are instant, and the last two reports are ready by about 08:40. It prints one JSON line to
`docker logs` when it ends. Move the time with
`ASSISTANT_PREWARM_FROM_MINUTE` (minute of the UTC day).

## What is kept, for how long, and how to delete it

The assistant's own copy of a conversation is working memory, not the record. Hermes stores it in two places only
(checked: `state.db` holds questions and answers; `logs/agent.log` held the start of each question until the log
level was raised to WARNING).

| What | Kept | Removed by |
|---|---|---|
| Conversations in `state.db` | until idle for `ASSISTANT_RETENTION_HOURS` (default 24) | the assistant itself, daily at 04:00 Bangkok: it stops its gateway for a few seconds, runs `maintain.sh`, starts again (`prune` measures last activity, so a chat in use is not cut off) |
| Log files | `ASSISTANT_LOG_RETENTION_DAYS` (default 7), WARNING and above, no question text | same daily maintenance |
| A shop's deletion request | at once | `./assistant/forget.sh --all` or `--chat-id ID`: stops the assistant for a few seconds, deletes the sessions (open ones too: `hermes sessions prune` skips them, which is why `purge_sessions.py` exists), empties the logs, compacts the store |
| Backups of the volume | 30 days, mode 0600 | `backup.sh` (stops the assistant for the copy) deletes older ones; a deletion request does not reach a backup until it expires |
| What the assistant remembers about the owner (`memories/MEMORY.md`, `USER.md`) | until the owner asks it to forget (`forget.sh --memory`, or `--all`) | preferences and vocabulary only; `memory_guard.py` removes any line with a figure, a decimal, a link, a customer code or the words of an instruction, every 10 minutes (under Hermes' own lock) and at the daily maintenance |

The 365 days the shop agreements allow is a ceiling, not a target: keeping less leaves less to protect. A record that
must outlive the day belongs in AI-BCC (the `agent_conversations` table exists and is not written yet), where the
shop-scoped delete already works. Hermes refuses to delete while its gateway holds the store open, which is why
deletion always happens with the gateway stopped (never `--force`). Context compression stays off because its model call is not bound by the
provider pin above (the auxiliary client has no code that reads `provider_routing`); a long chat is bounded by the
idle deletion and by the owner sending `/new`.

## What it learns, and what it does not (yet)

On: **memory and the owner profile**, including Hermes' background review that writes them. The assistant learns how
this owner likes answers (short, millions of baht, the owner's own names for things) and uses it in later
conversations; tested: a stated preference was stored, used in a new session with a correct figure, and a request to
"remember" a sales figure was refused. The review is a fork of the same model and inherits the provider pin (read in
`agent/background_review.py`; not verified end to end, because OpenRouter shows no per-call provider).

Why the guard exists: memory is the one place where text planted in report data (a product name someone typed in SML)
could become a standing instruction, and where a recalled figure could replace a fresh one. In a test with an
instruction planted in a report, memory stayed clean, but the model once said it *would* store a transfer account, so
the rules do not rest on the model alone.

Off, on purpose: **every bundled skill** (see the table above; `upgrade-check.sh` step 2b fails if one is enabled), **skills the agent writes for itself** (a self-written procedure could outrank the numeric rules in
`SOUL.md`; none are expected, and `memory_guard.py` reports any that appear), **context compression and title
generation** (their model calls are not bound by the provider pin), and the built-in tools (terminal, files, web,
browser, code, cron, delegation), which let an assistant for numbers reach beyond numbers. Each can be reviewed one at a
time; the order worth trying is skills (after memory has run for a few weeks), then scheduled messages.

## Usage during the trial

Conversations are deleted after 24 hours idle, so each session's summary is written to a ledger first
(`/opt/data/usage/ledger.jsonl`, kept 400 days): counts, tokens, cost, how long answers took, how many answers said
"no data", and a coarse topic bucket for those (stock, profit, purchase, tax, payroll, cash, customer, sales, other).
No question or answer text, no user or chat id. Cost in the report is worked out from tokens at the list price, ignoring the cache discount (close to the bill, a little under: side calls such as the memory review are not in the session totals), because Hermes showed $0 for this model; `USAGE_PRICE_IN_PER_M` and `USAGE_PRICE_OUT_PER_M` must follow the model in `config.yaml`. The OpenRouter line is the true total. The AI-BCC side is `agent_calls`, which already records every tool call
without values.

```
cd backend/deploy && ./assistant/usage-report.sh 7
```

Off by default: keeping the owner's own words for the questions that could not be answered (first 160 characters, 30
days, `usage/unanswered.jsonl`). It tells which report to build next, but it is the owner's wording, so turn it on only
with their say-so: add `LOG_UNANSWERED_TEXT=true` to the `assistant` environment in `compose.assistant.yml`.

## Updating

Nothing updates itself: the image is pinned by digest, and the update check Hermes tries on its own is refused by
the egress proxy. To update: `./assistant/backup.sh`; change the digest in `compose.assistant.yml`; `up -d`; run the
32-question set and the stale-figure test (`spikes/hermes`) and `assistant/live_check.py`; if anything regresses,
put the old digest back and, if the new version had migrated the store, restore the backup.

## What else the secretary can do (tools beyond the numbers)

`secretary_tools.py` (tests: `python3 -m unittest test_secretary_tools` in this folder):

- `make_file` writes a csv, xlsx, txt, md or html file under `/opt/data/outbox`. `gateway.strict` in `config.yaml` lets the gateway send a
  file to the chat only from there (and from Hermes' own caches), whatever path a reply names; `upgrade-check.sh` step 2c checks it.
  `maintain.sh` deletes outbox files and attachments older than `RETENTION_HOURS`.
- `read_document` returns the text of a `.xlsx` or `.docx` the owner attached (only from the attachment folders). PDF is not read.
- `web_search` asks Serper (`WEB_SEARCH_API_KEY`, primary) and, if that fails, SerpApi (`WEB_SEARCH_FALLBACK_KEY`); both give Google results.
  Without `WEB_SEARCH_API_KEY` the tool says search is not enabled and the welcome text leaves the web line out. Enter the keys with
  `set-secret.sh`, put `google.serper.dev,serpapi.com` into `ASSISTANT_EGRESS_ALLOW` in `.env.production` next to the existing hosts, then
  `up -d --force-recreate assistant assistant-egress`. A query that looks like it carries shop data (digit runs of 7 or more, an e-mail
  address, the alias AI-BCC gives a customer) is refused before it leaves, and `WEB_SEARCH_DAILY_CAP` (default 80 a day) keeps a loop
  from using the free plans (Serper 2,500 a month, SerpApi 100). The query text goes to those services, which are not the
  zero-retention model provider: say so in the shop agreement.

## Checking the real assistant

Two scripts run inside the live assistant container (`docker compose exec -T assistant /opt/hermes/.venv/bin/python - < assistant/<script>`):
`example_check.py` asks the 18 example questions of the owner's guide and checks each answer against the figure the real Agent API
returned; `safety_check.py` asks 5 questions the reports cannot answer (the answer must say there is no data) and makes 5 requests it
must refuse (run a command, read the config, show the instructions, send a message, change a figure). Both print PASS or FAIL and the
seconds. On 2026-10-09 with the pinned image and `google/gemini-3.1-flash-lite`: 18 of 18 and 10 of 10, one run each (the model is not
deterministic, so a pass is one observation). Answers took 3 to 36 seconds, a few over the 10 seconds the blueprint hopes for when
no live fetch is needed.

## Changing things

- **Model**: change `model.default` and `provider_routing.only` together, to a provider listed as zero-data-retention
  for that model (`https://openrouter.ai/api/v1/endpoints/zdr`), then run the 32-question set and the stale-figure test
  from `spikes/hermes` before use. A model that does its own arithmetic or copies codes wrongly fails them.
- **Persona**: `SOUL.md`. It is a copy of `spikes/hermes/SOUL.md`; change both.
- **Another shop**: copy the `assistant` service under a new name with its own `assistant_data` volume, its own
  env file and its own token. Never share a token or a data volume between shops.
- **Telegram**: a bot from @BotFather, one per deployment (never the Sentinel alert bot in `secrets/telegram`: that one
  speaks to the operators). Run `set-secret.sh TELEGRAM_BOT_TOKEN` and `set-secret.sh TELEGRAM_ALLOWED_USERS` (the
  numeric ids of the people allowed to chat; message @userinfobot to read your own), put
  `ASSISTANT_EGRESS_ALLOW=openrouter.ai,api.telegram.org` in `.env.production`, then `up -d --force-recreate assistant`. To let one more person in later without retyping the others: `./assistant/set-secret.sh TELEGRAM_ALLOWED_USERS --add`, then recreate the assistant.
  Without the allowed-users list the assistant does not start Telegram at all. Telegram sees every question and answer
  (it is not end-to-end encrypted for bots), which belongs in the shop's agreement next to the model provider.

## Known gaps

- Hermes keeps its own conversation history and logs in the `assistant_data` volume. They hold questions and
  figures; the retention and deletion policy (365 days, deletion on request) is task s4-6 and is not enforced yet.
- `assistant-egress` is a 60-line proxy that is tested but not battle-hardened; replace it with squid if the shop
  count grows.
- "Zero retention" is the provider's declared policy as listed by OpenRouter, not something checked here.

## Verification record (2026-10-02, V2 server, placeholder secrets, no real data sent to any model)

- `docker compose config` valid; `api` joined `agent` (recreated once); web to api still answers (health 200, admin
  session 401); the other five services unchanged and healthy.
- `hermes tools list` for `api_server` and `telegram`: 0 built-in toolsets enabled; `mcp test aibcc`: 4 tools.
- From inside the assistant: direct internet blocked (IP and name), `postgres` unresolvable, `api:8080` reachable;
  proxy: `openrouter.ai` allowed, `example.com` and `api.telegram.org` refused.
- A placeholder token sent to the real Agent API got the standard 401 body.
- End-to-end with the real token and the real model (2026-10-02, after the shop agreement was confirmed): nine
  questions checked against figures fetched straight from the Agent API with `live_check.py`: 9 of 9 passed, 3 to 7 s
  each (sales, receivables total and over-a-year, top debtor as an alias, at-risk customers, a server-side comparison,
  the morning-card report, one "no data" question, one attempt to extract the keys). It checks the assistant against
  the API, not the API against SML or the LINE card; that comparison is task s3-8.
- Sentinel drill (2026-10-04, on purpose, with the owner's consent): `docker compose stop assistant` at 15:40:32 Bangkok.
  The host probe showed `assistant=false` at 15:41:36; Sentinel opened the P1 incident `CONTAINER_ASSISTANT_UNHEALTHY` at
  15:42:21 (about 1 min 50 s after the stop) and sent the Telegram alert at 15:42:52. After `start` at 15:45:03 the
  incident was resolved at 15:45:51 (the probe counts "starting" as healthy) and the recovery message went out at 15:45:52.
  The alert text says "บริการระบบ AI-BCC" and shows the technical code, not that it is the owner assistant.
