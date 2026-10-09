"""Read back what the assistant was asked and what it answered, to review real use. Run inside the assistant container:

  docker compose exec -T assistant /opt/hermes/.venv/bin/python - [minutes] [source] < assistant/review_sessions.py

minutes (default 120): only conversations that had a message in that time. source (default telegram): the channel.
For each question it prints the time, the question, the tools called with the status each answered, the files sent and the
answer (shortened). Tool results are not printed, only their status word. This reads the conversation store of the operator's
own test accounts; once owners use the assistant it holds their questions too, so run it only for a review the shop agreed to.
"""
import json
import re
import sqlite3
import sys
import time
from datetime import datetime, timedelta, timezone

MINUTES = int(sys.argv[1]) if len(sys.argv) > 1 and sys.argv[1].isdigit() else 120
SOURCE = sys.argv[2] if len(sys.argv) > 2 else "telegram"
BANGKOK = timezone(timedelta(hours=7))
db = sqlite3.connect("file:/opt/data/state.db?mode=ro", uri=True)
since = time.time() - MINUTES * 60


def text_of(content):
    if content is None:
        return ""
    try:
        data = json.loads(content)
    except (TypeError, ValueError):
        return str(content)
    if isinstance(data, list):
        parts = []
        for part in data:
            if isinstance(part, dict):
                parts.append(part.get("text") or ("[รูปภาพ]" if "image" in str(part.get("type", "")) else ""))
        return " ".join(p for p in parts if p)
    return str(data) if not isinstance(data, str) else data


def status_of(content):
    match = re.search(r'"status"\s*:\s*"([A-Z_]+)"', str(content or ""))
    return match.group(1) if match else "?"


sessions = db.execute("select id from sessions where source = ? and id in (select session_id from messages group by session_id having max(timestamp) >= ?) order by id", (SOURCE, since)).fetchall()
count = 0
for (session_id,) in sessions:
    messages = db.execute("select role, content, tool_calls, tool_name, timestamp from messages where session_id = ? order by id", (session_id,)).fetchall()
    question, calls, files = None, [], []
    for role, content, tool_calls, tool_name, timestamp in messages:
        if role == "user":
            if question is not None:
                pass
            question, calls, files = (text_of(content), timestamp), [], []
        elif role == "assistant" and tool_calls:
            try:
                for call in json.loads(tool_calls):
                    calls.append((call.get("function", {}).get("name") or call.get("name") or "?"))
            except (TypeError, ValueError):
                pass
        elif role == "tool":
            calls = [f"{c}" for c in calls]
            status = status_of(content)
            calls.append(f"→{status}")
            if tool_name == "mcp__aibcc__make_file" or "make_file" in str(tool_name):
                match = re.search(r'"path"\s*:\s*"([^"]+)"', str(content))
                if match:
                    files.append(match.group(1))
        elif role == "assistant" and content and question is not None and question[1] >= since:
            count += 1
            when = datetime.fromtimestamp(question[1], BANGKOK).strftime("%d %H:%M")
            print(f"--- {when} Q: {question[0][:300]!r}")
            print(f"    tools: {' '.join(calls) if calls else '(none)'}" + (f" files: {files}" if files else ""))
            print(f"    A: {text_of(content)[:700]!r}")
            question = None
print(f"({count} answers in the last {MINUTES} minutes)")
