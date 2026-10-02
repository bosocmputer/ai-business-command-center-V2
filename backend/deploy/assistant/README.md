# Owner-facing assistant (Hermes) for one shop

An overlay on `compose.local.yml`. It runs one locked-down Hermes per shop that answers the owner's questions with
numbers it fetches from AI-BCC's Agent API (ADR 0001) and nothing else. Findings that led to these settings:
`spikes/hermes/FINDINGS.md`.

## What it can and cannot do

| | |
|---|---|
| Data source | AI-BCC `api` over the internal network `agent`, with the shop's own Agent API token |
| Internet | none directly; only hosts in `ASSISTANT_EGRESS_ALLOW` (default `openrouter.ai`) through `assistant-egress`, which logs host and decision (`docker logs`) |
| Built-in tools | none: terminal, files, web, browser, code execution, skills, memory, cron are off on every channel (`platform_toolsets`) |
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

Off, on purpose: **skills the agent writes for itself** (a self-written procedure could outrank the numeric rules in
`SOUL.md`; none are expected, and `memory_guard.py` reports any that appear), **context compression and title
generation** (their model calls are not bound by the provider pin), and the built-in tools (terminal, files, web,
browser, code, cron, delegation), which let an assistant for numbers reach beyond numbers. Each can be reviewed one at a
time; the order worth trying is skills (after memory has run for a few weeks), then scheduled messages.

## Updating

Nothing updates itself: the image is pinned by digest, and the update check Hermes tries on its own is refused by
the egress proxy. To update: `./assistant/backup.sh`; change the digest in `compose.assistant.yml`; `up -d`; run the
32-question set and the stale-figure test (`spikes/hermes`) and `assistant/live_check.py`; if anything regresses,
put the old digest back and, if the new version had migrated the store, restore the backup.

## Changing things

- **Model**: change `model.default` and `provider_routing.only` together, to a provider listed as zero-data-retention
  for that model (`https://openrouter.ai/api/v1/endpoints/zdr`), then run the 32-question set and the stale-figure test
  from `spikes/hermes` before use. A model that does its own arithmetic or copies codes wrongly fails them.
- **Persona**: `SOUL.md`. It is a copy of `spikes/hermes/SOUL.md`; change both.
- **Another shop**: copy the `assistant` service under a new name with its own `assistant_data` volume, its own
  env file and its own token. Never share a token or a data volume between shops.
- **Telegram** (step 5): add `TELEGRAM_BOT_TOKEN` to `hermes.env` and `api.telegram.org` to `ASSISTANT_EGRESS_ALLOW`.

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
