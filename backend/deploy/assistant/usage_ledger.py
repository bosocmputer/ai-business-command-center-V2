"""A usage ledger that outlives the conversations: one summary row per session, written just before the session is
deleted (purge_sessions.py), so the trial week still shows volume, cost, speed and what could not be answered.

A row holds counts and times only: no question or answer text, no user or chat id, no customer or figure. The
"unanswered" count is the number of answers that opened with the fixed phrase from SOUL.md ("ไม่มีข้อมูล"); the topic
is one coarse bucket chosen by keyword (stock, profit, purchase ...), never the words themselves.

Optional, off unless LOG_UNANSWERED_TEXT=true in the container environment: also keep the first 160 characters of
each unanswered question, for 30 days, in unanswered.jsonl. That is the owner's own wording, so it needs the owner's say-so.
"""
import json
import os
import sqlite3
import statistics
import time
from datetime import datetime, timedelta, timezone

DB = "/opt/data/state.db"
DIRECTORY = "/opt/data/usage"
LEDGER = os.path.join(DIRECTORY, "ledger.jsonl")
UNANSWERED = os.path.join(DIRECTORY, "unanswered.jsonl")
BANGKOK = timezone(timedelta(hours=7))
NO_DATA_PHRASE = "ไม่มีข้อมูล"

# First match wins. Coarse on purpose.
TOPICS = [
    ("stock", ("สต็อก", "สต๊อก", "คงเหลือ", "ของหมด", "สินค้าคงคลัง")),
    ("profit", ("กำไร", "ขาดทุน", "ต้นทุน")),
    ("purchase", ("ซื้อเข้า", "จัดซื้อ", "ผู้จำหน่าย", "เจ้าหนี้", "ซัพพลายเออร์")),
    ("tax", ("ภาษี", "vat", "ภพ")),
    ("payroll", ("เงินเดือน", "พนักงาน", "ค่าแรง")),
    ("cash", ("เงินสด", "ธนาคาร", "เงินฝาก", "รับเงิน", "จ่ายเงิน", "ค่าใช้จ่าย")),
    ("customer", ("ลูกค้า", "ลูกหนี้", "ค้างชำระ")),
    ("sales", ("ขาย", "บิล", "ยอด")),
]


def topic_of(question):
    text = question.lower()
    for name, words in TOPICS:
        if any(word in text for word in words):
            return name
    return "other"


def _percentile(values, fraction):
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, int(len(ordered) * fraction))] if ordered else None


def summarize(connection, session_id):
    """One session's row, or None when the session is not there."""
    session = connection.execute(
        "select source, started_at, last_activity_at, tool_call_count, api_call_count, input_tokens, output_tokens, cache_read_tokens,"
        " coalesce(actual_cost_usd, estimated_cost_usd, 0) from sessions where id = ?", (session_id,)).fetchone()
    if session is None:
        return None
    source, started, last, tool_calls, api_calls, tokens_in, tokens_out, cache_read, cost = session
    usage = connection.execute(
        "select coalesce(sum(api_call_count),0), coalesce(sum(input_tokens),0), coalesce(sum(output_tokens),0), coalesce(sum(cache_read_tokens),0),"
        " coalesce(sum(coalesce(actual_cost_usd, estimated_cost_usd, 0)),0) from session_model_usage where session_id = ?", (session_id,)).fetchone()
    if usage and usage[1]:  # the per-model rows are the more exact tally when present
        api_calls, tokens_in, tokens_out, cache_read, cost = usage
    turns, unanswered, durations, topics, unanswered_questions = 0, 0, [], {}, []
    open_question, open_at = None, None
    for role, content, calls, at in connection.execute(
            "select role, content, tool_calls, timestamp from messages where session_id = ? order by timestamp, id", (session_id,)):
        text = content if isinstance(content, str) else ""
        if role == "user" and text.strip():
            open_question, open_at = text, at
            turns += 1
        elif role == "assistant" and text.strip() and not (calls or "").strip("[] ") and open_question is not None:
            durations.append(max(0.0, at - open_at))
            if NO_DATA_PHRASE in text[:200]:
                unanswered += 1
                bucket = topic_of(open_question)
                topics[bucket] = topics.get(bucket, 0) + 1
                unanswered_questions.append(open_question[:160])
            open_question = None
    day = datetime.fromtimestamp(last or started or time.time(), BANGKOK).strftime("%Y-%m-%d")
    row = {"v": 1, "day": day, "source": source or "?", "turns": turns, "unanswered": unanswered, "topics": topics,
           "tool_calls": tool_calls or 0, "api_calls": int(api_calls or 0), "in": int(tokens_in or 0), "out": int(tokens_out or 0),
           "cache_read": int(cache_read or 0), "cost_usd": round(float(cost or 0), 6),
           "turn_p50_s": round(statistics.median(durations), 1) if durations else None, "turn_max_s": round(max(durations), 1) if durations else None}
    return row, unanswered_questions


def record(connection, session_id):
    """Append the session's row to the ledger. Returns True when written (or nothing to write)."""
    found = summarize(connection, session_id)
    if found is None:
        return True
    row, questions = found
    os.makedirs(DIRECTORY, mode=0o700, exist_ok=True)
    with open(LEDGER, "a", encoding="utf-8") as handle:
        handle.write(json.dumps(row, ensure_ascii=False) + "\n")
    os.chmod(LEDGER, 0o600)
    if os.environ.get("LOG_UNANSWERED_TEXT", "").lower() == "true" and questions:
        with open(UNANSWERED, "a", encoding="utf-8") as handle:
            for question in questions:
                handle.write(json.dumps({"day": row["day"], "question": question}, ensure_ascii=False) + "\n")
        os.chmod(UNANSWERED, 0o600)
    return True


def prune(ledger_days=400, unanswered_days=30):
    """Drop ledger rows older than ledger_days and unanswered questions older than unanswered_days."""
    for path, days in ((LEDGER, ledger_days), (UNANSWERED, unanswered_days)):
        if not os.path.exists(path):
            continue
        cutoff = (datetime.now(BANGKOK) - timedelta(days=days)).strftime("%Y-%m-%d")
        with open(path, encoding="utf-8") as handle:
            kept = [line for line in handle if line.strip() and json.loads(line).get("day", "") >= cutoff]
        with open(path, "w", encoding="utf-8") as handle:
            handle.writelines(kept)


def read_ledger():
    if not os.path.exists(LEDGER):
        return []
    with open(LEDGER, encoding="utf-8") as handle:
        return [json.loads(line) for line in handle if line.strip()]


def live_rows():
    """Rows for sessions still in the store (not yet purged), so the report is current to the minute."""
    if not os.path.exists(DB):
        return []
    connection = sqlite3.connect(f"file:{DB}?mode=ro", uri=True)
    try:
        ids = [r[0] for r in connection.execute("select id from sessions")]
        return [found[0] for found in (summarize(connection, i) for i in ids) if found]
    finally:
        connection.close()
