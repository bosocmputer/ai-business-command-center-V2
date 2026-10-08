---
status: current
last_verified: 2026-10-08
source_of_truth: [cmd/onboarding-check, internal/onboarding, deploy/onboard-check.sh]
tags: [onboarding, runbook]
---

# Opening a new shop

Goal (blueprint step 7): our part takes at most 30 minutes and waiting on the shop at most 3 days, without changing code. This is the
order that gets there. Steps marked **tool** have a command; the rest are screens that already exist.

## Ask the shop first (this is the waiting part)

1. A **read-only JavaWS** login for their SML (endpoint URL, database name, user, password) that our server can reach. Nothing that writes.
2. The **owner's name and LINE** (for the report cards) and, for the assistant, the owner's **Telegram** (the bot token is ours, the
   owner only messages it; their numeric id is added with `assistant/set-secret.sh TELEGRAM_ALLOWED_USERS --add`).
3. The **agreement** covers: reports and assistant answers sent to the model provider (zero retention), customer and supplier **names
   and phone numbers stored** in AI-BCC (the master data copy), questions and answers kept 365 days, deletion on request.
4. Who decides (owner or accountant) and who signs the number definitions (step 7 below).

## Our part

1. Create the shop (admin → shops), timezone `Asia/Bangkok`, access end date.
2. Add the SML connection and press **test**; it must say READY.
3. **tool** `./onboard-check.sh <tenant-id>`: runs every approved report query once on their SML (summary form, last 7 days), lists
   document types they use that no report counts, and reports data gaps. Fix every **ไม่ผ่าน** (FAIL) before going on; read every
   **ระวัง** (WARN) with the owner. A FAIL on a report usually means a table or field this shop does not have; do not "fix" it in the
   shop's data, change the query in code for everyone.
4. Measure report sizes (shop page → report modes → measure). A slow report in the check (WARN, 20 s or more) must be measured so it
   switches to chunked mode before the first morning card.
5. Add recipients (invitation link), set their report permissions, create the schedule, send a test card.
6. Assistant (optional): turn on "talk to the assistant" for the recipient, issue the token, set the Hermes secrets, run
   `assistant/example_check.py` (use `EXAMPLE_ONLY` to save the hourly quota). The master data copy appears at 06:30 next morning.
7. Number definitions: give the owner `docs/sml/numbers-dictionary.md` (what counts as sales, overdue and so on) and keep their
   answer. Read them the WARN lines from step 3, which are the limits of **their** data (for example receivables with no due date are never
   counted as overdue and never chased).
8. Run `./onboard-check.sh <tenant-id>` once more; keep the output (counts only) with the shop's file.

## What the check does not do

- It does not map a shop's own document types into the reports. It shows which types are used and not counted so a person decides;
  a configurable mapping per shop is built only when a real second shop needs it.
- It does not measure or change report modes (step 4) and does not touch the schedule or the assistant.
- It does not run the detail form of the reports, only the summary form the system uses for snapshots.

## Rollback

Nothing the check does is written. To take a shop out: pause its schedules, revoke its assistant token (admin → recipient), and set
the shop inactive. The master data copy is removed with the shop.
