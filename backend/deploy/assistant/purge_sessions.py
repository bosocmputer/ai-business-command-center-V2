"""Delete conversations from the assistant's store, open ones included.

`hermes sessions prune` only deletes sessions that have ended, and a chat session never ends, so on its own it
deletes nothing. This picks sessions by last activity (or by chat id, or all) and deletes each with Hermes' own
`sessions delete`, which also clears the search index. Run it with the gateway stopped: Hermes refuses to delete while
the gateway holds the store open, and this reports that refusal instead of hiding it.

  purge_sessions.py --idle-hours N | --all | --chat-id ID
Before a session is deleted its usage summary (counts and times, no text) is appended to the usage ledger; if that
fails the session is still deleted (a deletion request must not wait on statistics) and the failure is counted.
Prints one JSON line: matched / deleted / failed / ledger_failed. Exit status 1 if anything matched but was not deleted.
"""
import argparse
import json
import os
import sqlite3
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import usage_ledger  # noqa: E402

DB = "/opt/data/state.db"
HERMES = "/opt/hermes/bin/hermes"

parser = argparse.ArgumentParser()
group = parser.add_mutually_exclusive_group(required=True)
group.add_argument("--idle-hours", type=int)
group.add_argument("--all", action="store_true")
group.add_argument("--chat-id")
args = parser.parse_args()

try:
    store = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
except sqlite3.Error as error:
    print(json.dumps({"purge": "cannot open the store", "error": type(error).__name__}))
    sys.exit(1)
if args.all:
    rows = store.execute("select id from sessions").fetchall()
elif args.chat_id:
    rows = store.execute("select id from sessions where chat_id = ?", (args.chat_id,)).fetchall()
else:
    rows = store.execute("select id from sessions where last_activity_at < ?", (time.time() - args.idle_hours * 3600,)).fetchall()
store.close()

reader = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
deleted = failed = ledger_failed = 0
refused = False
for (session_id,) in rows:
    try:
        usage_ledger.record(reader, session_id)
    except Exception:  # noqa: BLE001 - statistics never block a deletion
        ledger_failed += 1
    done = subprocess.run([HERMES, "sessions", "delete", "--yes", session_id], capture_output=True, text=True)
    output = done.stdout + done.stderr
    if "Refusing" in output:
        refused = True
    if done.returncode == 0 and not refused:
        deleted += 1
    else:
        failed += 1
reader.close()
print(json.dumps({"purge": "refused: the store is in use" if refused else "done", "matched": len(rows), "deleted": deleted, "failed": failed, "ledger_failed": ledger_failed}))
sys.exit(1 if failed or refused else 0)
