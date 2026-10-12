import os
import sqlite3
import tempfile
import time
import unittest
from unittest import mock

import actor

NOW = 1_800_000_000.0


class ActorTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        self.path = os.path.join(self.dir, "state.db")
        db = sqlite3.connect(self.path)
        db.execute("create table sessions (id text primary key, source text, user_id text, ended_at real)")
        db.execute("create table messages (id integer primary key autoincrement, session_id text, role text, finish_reason text, timestamp real)")
        db.commit()
        db.close()
        patcher = mock.patch.dict(os.environ, {"AIBCC_ACTING": "line"})
        patcher.start()
        self.addCleanup(patcher.stop)

    def session(self, sid, source, user, messages, ended=None):
        db = sqlite3.connect(self.path)
        db.execute("insert into sessions values (?,?,?,?)", (sid, source, user, ended))
        for age, role, reason in messages:
            db.execute("insert into messages (session_id, role, finish_reason, timestamp) values (?,?,?,?)", (sid, role, reason, NOW - age))
        db.commit()
        db.close()

    def headers(self):
        return actor.actor_headers(self.path, NOW)

    def test_the_one_line_conversation_being_answered_names_the_person(self):
        self.session("a", "line", "Uclerk", [(20, "user", None)])
        self.assertEqual(self.headers(), {"X-Acting-Line-User": "Uclerk"})

    def test_a_turn_in_the_middle_of_tools_is_still_being_answered(self):
        self.session("a", "line", "Uclerk", [(30, "user", None), (20, "assistant", "tool_calls"), (20, "tool", None)])
        self.assertEqual(self.headers(), {"X-Acting-Line-User": "Uclerk"})

    def test_conversations_that_are_finished_or_ended_or_old_are_not_counted(self):
        self.session("done", "line", "Uowner", [(60, "user", None), (50, "assistant", "stop"), (50, "session_meta", None)])
        self.session("ended", "line", "Uold", [(20, "user", None)], ended=NOW - 5)
        self.session("stale", "line", "Ustale", [(actor.IN_FLIGHT_SECONDS + 100, "user", None)])
        self.session("now", "line", "Uclerk", [(10, "user", None)])
        self.assertEqual(self.headers(), {"X-Acting-Line-User": "Uclerk"})

    def test_two_people_at_once_or_nobody_is_refused_instead_of_guessed(self):
        with self.assertRaises(actor.ActorRefused):
            self.headers()  # nobody is being answered
        self.session("a", "line", "Uone", [(20, "user", None)])
        self.session("b", "line", "Utwo", [(10, "user", None)])
        with self.assertRaises(actor.ActorRefused):
            self.headers()

    def test_telegram_and_the_api_server_stay_the_token_holders(self):
        self.session("t", "telegram", "12345", [(20, "user", None)])
        self.assertEqual(self.headers(), {})

    def test_a_telegram_conversation_and_a_line_one_together_are_refused(self):
        self.session("t", "telegram", "12345", [(20, "user", None)])
        self.session("l", "line", "Uclerk", [(10, "user", None)])
        with self.assertRaises(actor.ActorRefused):
            self.headers()

    def test_an_unknown_kind_of_conversation_or_a_line_one_without_a_person_is_refused(self):
        self.session("x", "discord", "1", [(20, "user", None)])
        with self.assertRaises(actor.ActorRefused):
            self.headers()

    def test_an_unreadable_store_is_refused_and_the_mode_off_asks_nothing(self):
        with self.assertRaises(actor.ActorRefused):
            actor.actor_headers(os.path.join(self.dir, "missing.db"), NOW)
        with mock.patch.dict(os.environ, {"AIBCC_ACTING": ""}):
            self.assertEqual(actor.actor_headers(os.path.join(self.dir, "missing.db"), NOW), {})
        with mock.patch.dict(os.environ, {"AIBCC_ACTING": "${AIBCC_ACTING}"}):
            self.assertEqual(actor.mode(), "")


if __name__ == "__main__":
    unittest.main()
