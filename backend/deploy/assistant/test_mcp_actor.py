import http.server
import importlib
import json
import os
import sqlite3
import sys
import tempfile
import threading
import types
import unittest
from unittest import mock

import actor

NOW = 1_800_000_000.0


class FakeServer:
    def __init__(self, *args, **kwargs):
        pass

    def tool(self, *args, **kwargs):
        return lambda function: function

    def run(self, *args, **kwargs):
        pass


def install_fake_mcp():
    for name in ("mcp", "mcp.server", "mcp.server.mcpserver", "mcp.server.fastmcp"):
        sys.modules.setdefault(name, types.ModuleType(name))
    sys.modules["mcp.server.mcpserver"].MCPServer = FakeServer
    sys.modules["mcp.server.fastmcp"].FastMCP = FakeServer


class Capture(http.server.BaseHTTPRequestHandler):
    seen = []

    def do_GET(self):
        Capture.seen.append(dict(self.headers))
        body = json.dumps({"status": "READY"}).encode()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


class McpActorTests(unittest.TestCase):
    def setUp(self):
        install_fake_mcp()
        self.server = http.server.HTTPServer(("127.0.0.1", 0), Capture)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.shutdown)
        self.addCleanup(self.server.server_close)
        Capture.seen = []
        self.dir = tempfile.mkdtemp()
        self.db = os.path.join(self.dir, "state.db")
        db = sqlite3.connect(self.db)
        db.execute("create table sessions (id text primary key, source text, user_id text, ended_at real)")
        db.execute("create table messages (id integer primary key autoincrement, session_id text, role text, finish_reason text, timestamp real)")
        db.commit()
        db.close()
        env = {"AIBCC_URL": f"http://127.0.0.1:{self.server.server_port}", "AIBCC_TOKEN": "abcc_test", "AIBCC_ACTING": "line", "HERMES_HOME": self.dir}
        patcher = mock.patch.dict(os.environ, env)
        patcher.start()
        self.addCleanup(patcher.stop)
        sys.modules.pop("aibcc_mcp", None)
        self.shim = importlib.import_module("aibcc_mcp")
        clock = mock.patch.object(actor.time, "time", return_value=NOW)
        clock.start()
        self.addCleanup(clock.stop)

    def session(self, sid, source, user, role="user"):
        db = sqlite3.connect(self.db)
        db.execute("insert into sessions values (?,?,?,NULL)", (sid, source, user))
        db.execute("insert into messages (session_id, role, finish_reason, timestamp) values (?,?,?,?)", (sid, role, None, NOW - 5))
        db.commit()
        db.close()

    def test_a_call_made_while_a_line_person_is_being_answered_carries_who_they_are_and_the_token(self):
        self.session("a", "line", "Uclerk")
        answer = self.shim.call("/context")
        self.assertEqual(json.loads(answer)["status"], "READY")
        self.assertEqual(Capture.seen[0].get("X-Acting-Line-User"), "Uclerk")
        self.assertEqual(Capture.seen[0].get("Authorization"), "Bearer abcc_test")

    def test_a_call_that_cannot_be_attributed_is_not_sent_at_all(self):
        self.session("a", "line", "Uone")
        self.session("b", "line", "Utwo")
        answer = json.loads(self.shim.call("/context"))
        self.assertEqual(answer["status"], "UNAVAILABLE")
        self.assertEqual(answer["message"], actor.MESSAGE_REFUSED)
        self.assertEqual(Capture.seen, [])

    def test_telegram_calls_carry_no_person_and_the_mode_off_changes_nothing(self):
        self.session("t", "telegram", "12345")
        self.shim.call("/context")
        self.assertNotIn("X-Acting-Line-User", Capture.seen[0])
        with mock.patch.dict(os.environ, {"AIBCC_ACTING": ""}):
            self.shim.call("/context")
        self.assertNotIn("X-Acting-Line-User", Capture.seen[1])


if __name__ == "__main__":
    unittest.main()
