"""Who is asking, for the assistant's calls to AI-BCC.

One assistant (one AI-BCC token) serves a shop, but in LINE several people may write to it, and each may read different reports. When
AIBCC_ACTING is "line" the shim does not leave that to the model: it looks at Hermes' own conversation store (state.db, written by Hermes from
the messages it received, never by the model) for the one conversation that is being answered right now, and tells AI-BCC which LINE user it is
(header X-Acting-Line-User). AI-BCC then answers with that person's own permissions.

A conversation is "being answered" while its last message is the person's (or a tool result), or the model's request to call a tool, and it is recent.
Telegram and the local API server stay the token holder's (the people allowed on Telegram are listed by hand). If it cannot be told who is asking
(nobody being answered, two people at once, an unknown kind of conversation, the store unreadable) the call is refused instead of answered with
someone else's permissions.
"""
import os
import sqlite3
import time

IN_FLIGHT_SECONDS = 600
MESSAGE_REFUSED = "ตอนนี้ระบบยืนยันไม่ได้ว่าใครเป็นคนถาม (อาจมีคนถามพร้อมกัน) ลองถามใหม่อีกครั้งในอีกสักครู่ครับ"
HEADER = "X-Acting-Line-User"
HOLDER_SOURCES = {"telegram", "api_server"}


class ActorRefused(Exception):
    """It cannot be told who is asking."""


def mode():
    value = os.environ.get("AIBCC_ACTING", "").strip()
    return value if value == "line" else ""  # an unset ${VAR} stays literal in the MCP environment; that is "off"


def database_path():
    home = os.environ.get("HERMES_HOME", "")
    if not home or home.startswith("${"):
        home = "/opt/data"
    return os.path.join(home, "state.db")


def answering_now(path, now):
    """(source, user_id) of every conversation that is being answered at this moment."""
    try:
        connection = sqlite3.connect(f"file:{path}?mode=ro", uri=True, timeout=2)
    except sqlite3.Error as error:
        raise ActorRefused from error
    try:
        rows = connection.execute(
            """select s.source, s.user_id, m.role, m.finish_reason
               from sessions s
               join messages m on m.id = (select x.id from messages x where x.session_id = s.id and x.role != 'session_meta'
                                          order by x.timestamp desc, x.id desc limit 1)
               where s.ended_at is null and m.timestamp > ?""",
            (now - IN_FLIGHT_SECONDS,),
        ).fetchall()
    except sqlite3.Error as error:
        raise ActorRefused from error
    finally:
        connection.close()
    return [(source, user) for source, user, role, reason in rows if role in ("user", "tool") or (role == "assistant" and reason == "tool_calls")]


def actor_headers(path=None, now=None):
    """The headers that say who is asking: {} for the token holder, the LINE user for a LINE conversation, ActorRefused otherwise."""
    if not mode():
        return {}
    active = answering_now(path or database_path(), now if now is not None else time.time())
    if len(active) != 1:
        raise ActorRefused
    source, user = active[0]
    if source in HOLDER_SOURCES:
        return {}
    if source == "line" and user:
        return {HEADER: user}
    raise ActorRefused
