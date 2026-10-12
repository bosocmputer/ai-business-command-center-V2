import os
import re
import shutil
import stat
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def read(path):
    with open(path, encoding="utf-8") as handle:
        return handle.read()


def block(text, header):
    """The lines of one service in a compose file: from its header to the next service header."""
    match = re.search(r"(?ms)^  " + re.escape(header) + r":\n(.*?)(?=^  [A-Za-z0-9_-]+:\n|^[a-z]|\Z)", text)
    assert match, header
    return match.group(1)


def env_keys(text):
    env = re.search(r"(?ms)^    environment:\n(.*?)(?=^    [a-z_]+:)", text)
    return set(re.findall(r"^      ([A-Z_]+):", env.group(1), re.M))


class ShopTemplateTests(unittest.TestCase):
    def setUp(self):
        self.base = read(os.path.join(HERE, "compose.assistant.yml"))
        self.template = read(os.path.join(HERE, "compose.shop.template.yml"))

    def test_the_second_assistant_has_the_same_settings_as_the_first_except_the_ones_that_must_differ(self):
        first, second = env_keys(block(self.base, "assistant")), env_keys(block(self.template, "assistant-__INST__"))
        self.assertEqual(first, second, "an environment setting was added to one assistant and not the other")
        self.assertEqual(env_keys(block(self.base, "assistant-egress")), env_keys(block(self.template, "assistant-__INST__-egress")))

    def test_it_never_reads_the_first_shops_secrets_or_shares_its_network_volume_or_proxy(self):
        mine = block(self.template, "assistant-__INST__")
        self.assertIn("hermes-__INST__.env", mine)
        self.assertNotRegex(self.template.replace("hermes-__INST__.env", ""), r"hermes\.env")
        self.assertNotRegex(self.template, r"(?m)^\s+networks:.*\b(agent|egress)\b[,\]]")
        self.assertNotIn("assistant_data:", self.template)
        self.assertNotIn("assistant-egress:3128", self.template)
        self.assertIn("ASSISTANT_CONFIG_PULL: \"on\"", mine)

    def test_the_generator_fills_every_placeholder_and_keeps_secrets_private_and_stable(self):
        root = tempfile.mkdtemp()
        os.makedirs(os.path.join(root, "assistant"))
        for name in ("add-shop.sh", "compose.assistant.yml", "compose.shop.template.yml"):
            shutil.copy(os.path.join(HERE, name), os.path.join(root, "assistant", name))
        run = lambda *args: subprocess.run(["sh", os.path.join(root, "assistant", "add-shop.sh"), *args], capture_output=True, text=True)
        done = run("shop2")
        self.assertEqual(done.returncode, 0, done.stderr)
        generated = read(os.path.join(root, "assistant", "compose.assistant-shop2.yml"))
        self.assertNotRegex(generated, r"__[A-Z]+__")
        self.assertIn("nousresearch/hermes-agent:v", generated)
        self.assertIn("assistant-shop2-egress:3128", generated)
        secrets = os.path.join(root, "secrets", "assistant", "hermes-shop2.env")
        self.assertEqual(stat.S_IMODE(os.stat(secrets).st_mode), 0o600)
        first = read(secrets)
        self.assertRegex(first, r"^API_SERVER_KEY=[0-9a-f]{48}\n$")
        self.assertNotIn(first.split("=")[1].strip(), done.stdout + done.stderr)
        run("shop2")
        self.assertEqual(read(secrets), first, "running it again must not change the key")
        for bad in ("Shop2", "-x", "x-", "a b", "a/b", "x" * 25):
            self.assertNotEqual(run(bad).returncode, 0, bad)


if __name__ == "__main__":
    unittest.main()
