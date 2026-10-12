import os
import subprocess
import sys
import tempfile
import textwrap
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
PATCH = os.path.join(HERE, "pypatch")


def run(code, package_source):
    root = tempfile.mkdtemp()
    package = os.path.join(root, "gateway")
    os.makedirs(package)
    open(os.path.join(package, "__init__.py"), "w").close()
    with open(os.path.join(package, "run_turn.py"), "w") as handle:
        handle.write(textwrap.dedent(package_source))
    env = dict(os.environ, PYTHONPATH=os.pathsep.join([PATCH, root]))
    return subprocess.run([sys.executable, "-c", textwrap.dedent(code)], capture_output=True, text=True, env=env)


class PyPatchTests(unittest.TestCase):
    def test_the_model_lines_of_the_new_reply_are_dropped_and_the_rest_is_untouched(self):
        done = run('''
            import gateway.run_turn as m
            print(repr(m.GatewayTurnMixin()._format_session_info()), m.GatewayTurnMixin().other())
        ''', '''
            class GatewayTurnMixin:
                def _format_session_info(self): return "◆ Model: x"
                def other(self): return "kept"
        ''')
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(done.stdout.strip(), "'' kept")
        self.assertIn("the /new reply carries no model lines", done.stderr)

    def test_a_hermes_without_that_method_is_left_alone_and_does_not_break(self):
        done = run('''
            import gateway.run_turn as m
            print(m.GatewayTurnMixin().hello())
        ''', '''
            class GatewayTurnMixin:
                def hello(self): return "hi"
        ''')
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(done.stdout.strip(), "hi")
        self.assertNotIn("carries no model lines", done.stderr)

    def test_other_modules_are_not_touched(self):
        done = run('''
            import json, gateway
            print(json.dumps({"a": 1}))
        ''', '''
            class GatewayTurnMixin:
                def _format_session_info(self): return "x"
        ''')
        self.assertEqual(done.returncode, 0, done.stderr)
        self.assertEqual(done.stdout.strip(), '{"a": 1}')

    def test_the_check_script_says_ok_only_when_the_lines_are_gone(self):
        script = os.path.join(HERE, "reset_notice_check.py")
        root = tempfile.mkdtemp()
        os.makedirs(os.path.join(root, "gateway"))
        open(os.path.join(root, "gateway", "__init__.py"), "w").close()
        with open(os.path.join(root, "gateway", "run_turn.py"), "w") as handle:
            handle.write("class GatewayTurnMixin:\n    def _format_session_info(self): return 'lines'\n")
        patched = subprocess.run([sys.executable, script], capture_output=True, text=True, env=dict(os.environ, PYTHONPATH=os.pathsep.join([PATCH, root])))
        self.assertEqual((patched.returncode, patched.stdout.strip()), (0, "ok"), patched.stderr)
        unpatched = subprocess.run([sys.executable, script], capture_output=True, text=True, env=dict(os.environ, PYTHONPATH=root))
        self.assertEqual(unpatched.returncode, 1)


if __name__ == "__main__":
    unittest.main()
