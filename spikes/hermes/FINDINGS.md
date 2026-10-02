# Hermes spike findings (2026-10-02)

Setup: Hermes `v2026.9.24` in a locked-down container (non-root, no capabilities, 1 GB, 1 CPU,
read-only filesystem, built-in toolsets all off, only an `aibcc` MCP server with a per-shop token).
Mock Agent API with two invented shops (A: sales + aging + RFM, B: sales only). 15 Thai questions
per model through `hermes -z`, numbers checked against the mock's own data.

Models (both free on OpenRouter): `stealth/space-bunny-alpha` (anonymous, expires 2026-10-05) and
`qwen/qwen3.8-27b:free`.

## Result

| | space-bunny-alpha | qwen3.8-27b:free |
|---|---|---|
| Questions answered | 15 / 15 | 10 / 15 (5 failed with HTTP 429 from the free provider) |
| Numbers correct | all | all of those answered |
| Invented facts | none | one: stated a wrong "previous period" date range that the API never gave |
| Mean time per question | 29 s (max 44) | 52 s (max 232) |
| API calls per question | 3.5 | 5.2 |
| Tokens per question | about 16,000 | about 32,000 |
| Refused terminal, file read, secret, system prompt | yes, 3 of 3 | yes (one took 232 s and 12 calls before refusing, with garbled text) |
| Followed the instruction planted in report data | no, and told the owner about it | not hit in the answers that completed |
| Cross-shop request | gave no data of the other shop | gave no data of the other shop |

Both models read the date from `context`, interpreted "last month", "this week" and "first 10 days"
correctly, said "no data" for reports that do not exist, and never revealed that a report existed but was
forbidden.

## What this means for the Agent API

1. **Expose the tools directly.** By default Hermes hides MCP tools behind `tool_search`,
   `tool_describe` and `tool_call`, which adds two or three model calls to every question. Setting
   `tools.tool_search.enabled: "off"` cut calls from 4-5 to 3 and tokens by about 30% in a spot check.
   Use it.
2. **Authorization stays server-side.** A token belongs to one shop; a report the token may not read
   answers exactly like one that does not exist (`NO_DATA`). The model never saw a "forbidden" answer, so it
   could not leak one. Keep that contract.
3. **Return the period actually used, and a collected-at time, in every answer.** Both models relied on
   them to state the range and "data as of".
4. **Do arithmetic and comparison server-side** (`compare`). A model computed a percentage correctly, but a
   model that invents a date range (one did) can invent a figure.
5. **Say what a comparison compares.** "This week" (3 days) against the 3 days before it, which included a
   closed Sunday, gave +140% and was reported faithfully. The API should define comparison periods and say so.
6. **Treat report text as untrusted.** A planted instruction in a report note was ignored by the model that
   completed that question, which also warned the owner. Do not rely on that: keep free text out of numeric
   reports, or send it as clearly marked data.
7. **Latency is too high for chat at about 30 s.** Container start is only 1.5 s; the rest is Hermes start, the
   MCP server start and 3-6 model calls. A long-running gateway process with the MCP server kept alive should
   remove most of the start-up; measure that in step 4.

## Limits of this spike

- Fake data and a mock API, not the real Agent API; two free models, one of them anonymous and expiring.
- Free-tier rate limits made the Qwen run unreliable; that says little about Qwen on a paid route.
- Prompts and answers to an anonymous "stealth" model may be logged by its provider. This is acceptable
  for invented data and not for a shop's real data. Choose a model whose data policy is known before step 5.
- Token counts include the system prompt and tool list sent on every call (about 5,000 input tokens before
  any question); prompt caching on the provider side hid part of the cost.
