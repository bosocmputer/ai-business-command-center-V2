import os
import re
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))


def block(text, name):
    """The indented lines of a top-level section of config.yaml."""
    match = re.search(r"(?m)^" + re.escape(name) + r":\s*\n((?:[ \t]+.*\n|\s*\n|#.*\n)*)", text)
    assert match, name
    return match.group(1)


class ToolsetTests(unittest.TestCase):
    def setUp(self):
        with open(os.path.join(HERE, "config.yaml"), encoding="utf-8") as handle:
            self.text = handle.read()

    def test_every_chat_platform_is_listed_with_memory_as_its_only_tool(self):
        platforms = set(re.findall(r"(?m)^  ([a-z_]+):\s*$", block(self.text, "platforms")))
        listed = dict(re.findall(r"(?m)^  ([a-z_]+): (\[[^\]]*\])", block(self.text, "platform_toolsets")))
        self.assertTrue({"telegram", "line"} <= platforms, platforms)
        # A platform that is not listed gets Hermes' defaults: terminal, file, browser, code execution... (this happened to LINE until 12 Oct 2026).
        self.assertEqual(sorted(platforms - set(listed)), [], "a platform in `platforms:` has no entry in `platform_toolsets:`")
        for name, tools in listed.items():
            self.assertEqual(tools, "[memory]", name)
        for name in ("cli", "api_server", "webhook"):
            self.assertIn(name, listed)


if __name__ == "__main__":
    unittest.main()
