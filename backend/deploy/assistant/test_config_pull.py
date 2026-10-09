import io
import json
import os
import stat
import tempfile
import unittest
from contextlib import redirect_stdout
from unittest import mock

import config_pull as pull

TEMPLATE = """model:
  provider: openrouter
  default: google/gemini-3.1-flash-lite

provider_routing:
  only: [google-vertex]
  data_collection: deny
  require_parameters: true
"""

ENABLED = {
    "configVersion": 7, "enabled": True, "shopName": "x", "lineMode": "NONE",
    "model": {"key": "deepseek-v4.1-flash", "modelId": "deepseek/deepseek-v4.1-flash", "providers": ["deepinfra", "together"], "dataCollectionDeny": True},
    "secrets": {"openrouterKey": "sk-or-v1-abc'def$x", "telegramBotToken": "123456:AAA"},
}


class PullTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.mkdtemp()
        template = os.path.join(self.dir, "template.yaml")
        open(template, "w").write(TEMPLATE)
        patcher = mock.patch.multiple(pull, DATA=self.dir, TEMPLATE=template)
        patcher.start()
        self.addCleanup(patcher.stop)
        self.log = mock.patch.object(pull, "log")
        self.log.start()
        self.addCleanup(self.log.stop)

    def run_start(self, answer):
        out = io.StringIO()
        with mock.patch.object(pull, "fetch", return_value=answer), mock.patch.object(pull, "post_status") as post, redirect_stdout(out):
            code = pull.start()
        return code, out.getvalue(), post

    def test_the_model_and_the_pinned_providers_are_put_into_the_yaml(self):
        code, _, _ = self.run_start((200, '"v7-true"', ENABLED))
        text = open(os.path.join(self.dir, "config.yaml")).read()
        self.assertEqual(code, 0)
        self.assertIn("  default: deepseek/deepseek-v4.1-flash", text)
        self.assertIn("  only: [deepinfra, together]", text)
        self.assertIn("data_collection: deny", text)

    def test_secrets_are_quoted_for_the_shell_and_unused_ones_are_unset(self):
        _, out, _ = self.run_start((200, '"v7-true"', ENABLED))
        self.assertIn("export OPENROUTER_API_KEY='sk-or-v1-abc'\"'\"'def$x'", out)  # a quote and a dollar sign cannot break out
        self.assertIn("unset LINE_CHANNEL_SECRET", out)
        self.assertIn("unset LINE_CHANNEL_ACCESS_TOKEN", out)

    def test_a_shop_that_must_not_run_gets_every_secret_unset_and_the_template_untouched(self):
        off = {"configVersion": 8, "enabled": False, "reason": "EXPIRED"}
        code, out, post = self.run_start((200, '"v8-false"', off))
        self.assertEqual(code, 0)
        self.assertNotIn("export", out)
        self.assertEqual(out.count("unset"), 4)
        self.assertEqual(open(os.path.join(self.dir, "config.yaml")).read(), TEMPLATE)
        post.assert_called_once()

    def test_a_test_model_has_no_provider_pin_and_allows_collection(self):
        free = dict(ENABLED, model={"key": "openrouter-free", "modelId": "openrouter/free", "providers": [], "dataCollectionDeny": False})
        self.run_start((200, '"v9-true"', free))
        text = open(os.path.join(self.dir, "config.yaml")).read()
        self.assertIn("default: openrouter/free", text)
        self.assertIn("# no provider pin", text)
        self.assertIn("data_collection: allow", text)

    def test_the_copy_is_private_and_used_when_ai_bcc_cannot_be_reached(self):
        self.run_start((200, '"v7-true"', ENABLED))
        mode = stat.S_IMODE(os.stat(os.path.join(self.dir, "assistant-config.json")).st_mode)
        self.assertEqual(mode, 0o600)
        code, out, post = self.run_start((0, None, None))
        self.assertEqual(code, 0)
        self.assertIn("export OPENROUTER_API_KEY", out)
        post.assert_not_called()  # a copy is not reported as the current version

    def test_without_an_answer_and_without_a_copy_the_old_way_is_kept(self):
        code, out, _ = self.run_start((0, None, None))
        self.assertEqual((code, out), (pull.EXIT_NO_CONFIG, ""))
        self.assertFalse(os.path.exists(os.path.join(self.dir, "config.yaml")))

    def test_a_refusal_from_ai_bcc_does_not_replace_the_copy(self):
        self.run_start((200, '"v7-true"', ENABLED))
        code, out, _ = self.run_start((401, None, None))
        self.assertEqual(code, 0)
        self.assertIn("export OPENROUTER_API_KEY", out)

    def test_changed_is_true_only_for_a_new_tag_with_a_body(self):
        self.run_start((200, '"v7-true"', ENABLED))
        with mock.patch.object(pull, "fetch", return_value=(304, '"v7-true"', None)):
            self.assertEqual(pull.changed(), 1)
        with mock.patch.object(pull, "fetch", return_value=(0, None, None)):
            self.assertEqual(pull.changed(), 1)
        with mock.patch.object(pull, "fetch", return_value=(200, '"v8-true"', ENABLED)):
            self.assertEqual(pull.changed(), 0)

    def test_check_compares_by_hash_and_never_prints_a_secret(self):
        self.run_start((200, '"v7-true"', ENABLED))
        env = {"OPENROUTER_API_KEY": "sk-or-v1-abc'def$x", "TELEGRAM_BOT_TOKEN": "other"}
        out = io.StringIO()
        with mock.patch.dict(os.environ, env, clear=False), mock.patch.object(pull, "fetch", return_value=(200, '"v7-true"', ENABLED)), redirect_stdout(out):
            code = pull.check()
        report = json.loads(out.getvalue())
        self.assertEqual(code, 1)
        self.assertEqual(report["variables"]["OPENROUTER_API_KEY"], "same")
        self.assertEqual(report["variables"]["TELEGRAM_BOT_TOKEN"], "different")
        self.assertNotIn("sk-or", out.getvalue())
        self.assertNotIn("other", out.getvalue())


if __name__ == "__main__":
    unittest.main()
