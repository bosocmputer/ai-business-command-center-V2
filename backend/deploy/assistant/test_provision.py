import os
import shutil
import stat
import subprocess
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
DEPLOY = os.path.dirname(HERE)

SHIM = r"""#!/bin/sh
# A stand-in for docker: records every call and answers from files in $SHIM_DIR.
echo "$*" >> "$SHIM_DIR/calls.log"
case "$*" in *" up "*) echo "COMPOSE_FILE=$COMPOSE_FILE" >> "$SHIM_DIR/calls.log";; esac
case "$*" in
  *"exec -T worker /app/provision list"*) cat "$SHIM_DIR/list.out" 2>/dev/null; exit 0;;
  *"exec -T worker /app/provision token"*) if [ -f "$SHIM_DIR/token.out" ]; then cat "$SHIM_DIR/token.out"; exit 0; else exit 1; fi;;
  *"exec -T worker /app/provision "*) exit 0;;
  *" ps -q "*) echo container-id; exit 0;;
  *" up "*) [ -f "$SHIM_DIR/up.fail" ] && exit 1; exit 0;;
  *" rm -sf "*) exit 0;;
  inspect*) cat "$SHIM_DIR/health.out"; exit 0;;
esac
exit 0
"""

TOKEN = "abcc_SECRET_TOKEN_VALUE_0123456789"


class ProvisionScriptTests(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()
        self.deploy = os.path.join(self.root, "deploy")
        os.makedirs(os.path.join(self.deploy, "assistant"))
        os.makedirs(os.path.join(self.root, "bin"))
        self.shim = os.path.join(self.root, "shim")
        os.makedirs(self.shim)
        for name in ("provision.sh",):
            shutil.copy(os.path.join(DEPLOY, name), self.deploy)
        for name in ("add-shop.sh", "compose-files.sh", "compose.assistant.yml", "compose.shop.template.yml"):
            shutil.copy(os.path.join(HERE, name), os.path.join(self.deploy, "assistant"))
        open(os.path.join(self.deploy, "compose.local.yml"), "w").close()
        open(os.path.join(self.deploy, ".env.production"), "w").close()
        docker = os.path.join(self.root, "bin", "docker")
        with open(docker, "w") as handle:
            handle.write(SHIM)
        os.chmod(docker, 0o755)
        self.write("health.out", "healthy\n")

    def write(self, name, text):
        with open(os.path.join(self.shim, name), "w") as handle:
            handle.write(text)

    def run_script(self):
        env = dict(os.environ, PATH=os.path.join(self.root, "bin") + ":" + os.environ["PATH"], SHIM_DIR=self.shim,
                   PROVISION_HEALTH_TRIES="2", PROVISION_HEALTH_SLEEP="0")
        done = subprocess.run(["sh", os.path.join(self.deploy, "provision.sh")], capture_output=True, text=True, env=env, cwd=self.deploy)
        log = os.path.join(self.shim, "calls.log")
        calls = ""
        if os.path.exists(log):
            with open(log) as handle:
                calls = handle.read()
        return done, calls

    def secrets_file(self, name):
        return os.path.join(self.deploy, "secrets", "assistant", f"hermes-{name}.env")

    def test_nothing_to_do_changes_nothing(self):
        done, calls = self.run_script()
        self.assertEqual(done.returncode, 0)
        self.assertNotIn("up -d", calls)
        self.assertFalse(os.path.exists(os.path.join(self.deploy, "secrets")))

    def test_a_start_writes_the_token_privately_starts_the_containers_and_reports_running(self):
        self.write("list.out", "s9d36e282 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        self.write("token.out", TOKEN)
        done, calls = self.run_script()
        self.assertEqual(done.returncode, 0, done.stderr)
        file = self.secrets_file("s9d36e282")
        with open(file) as handle:
            content = handle.read()
        self.assertIn("AIBCC_TOKEN=" + TOKEN + "\n", content)
        self.assertIn("API_SERVER_KEY=", content)
        self.assertEqual(stat.S_IMODE(os.stat(file).st_mode), 0o600)
        self.assertNotIn(TOKEN, calls + done.stdout + done.stderr, "the token must only pass through the pipe into the file")
        self.assertTrue(os.path.exists(os.path.join(self.deploy, "assistant", "compose.assistant-s9d36e282.yml")))
        order = [line for line in calls.splitlines() if "provision report" in line or "provision ack" in line or " up -d" in line]
        self.assertIn("provision report s9d36e282 CREATING", order[0])
        self.assertTrue(any("up -d --no-build assistant-s9d36e282" in line for line in order))
        self.assertIn("provision ack s9d36e282", " ".join(order))
        self.assertIn("provision report s9d36e282 RUNNING", order[-1])
        self.assertLess(" ".join(order).index("provision ack"), " ".join(order).index("RUNNING"))

    def test_the_generated_file_is_part_of_the_stack_when_the_containers_are_started(self):
        self.write("list.out", "s1 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        self.write("token.out", TOKEN)
        _, calls = self.run_script()
        stack = [line for line in calls.splitlines() if line.startswith("COMPOSE_FILE=")]
        self.assertTrue(stack and "compose.assistant-s1.yml" in stack[-1], stack)

    def test_a_container_that_never_gets_healthy_is_reported_as_an_error_and_the_token_stays_for_another_try(self):
        self.write("list.out", "s9d36e282 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        self.write("token.out", TOKEN)
        self.write("health.out", "starting\n")
        _, calls = self.run_script()
        self.assertIn("provision report s9d36e282 ERROR START_TIMEOUT", calls)
        self.assertNotIn("provision ack", calls)

    def test_a_failure_to_start_is_reported_with_a_short_code(self):
        self.write("list.out", "s9d36e282 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        self.write("token.out", TOKEN)
        self.write("up.fail", "")
        _, calls = self.run_script()
        self.assertIn("provision report s9d36e282 ERROR COMPOSE_UP_FAILED", calls)

    def test_without_a_token_anywhere_it_reports_no_token_and_starts_nothing(self):
        self.write("list.out", "s9d36e282 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        _, calls = self.run_script()
        self.assertIn("provision report s9d36e282 ERROR NO_TOKEN", calls)
        self.assertNotIn("up -d", calls)

    def test_a_second_run_keeps_the_token_already_written_when_ai_bcc_no_longer_has_it(self):
        self.write("list.out", "s9d36e282 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        self.write("token.out", TOKEN)
        self.run_script()
        os.remove(os.path.join(self.shim, "token.out"))
        self.run_script()
        with open(self.secrets_file("s9d36e282")) as handle:
            content = handle.read()
        self.assertEqual(content.count("AIBCC_TOKEN="), 1)
        self.assertIn(TOKEN, content)

    def test_a_stop_removes_the_containers_and_reports_stopped(self):
        self.write("list.out", "s9d36e282 9d36e282-0355-4caf-91e0-daf0857c935b stop\n")
        _, calls = self.run_script()
        self.assertIn("rm -sf assistant-s9d36e282 assistant-s9d36e282-egress", calls)
        self.assertIn("provision report s9d36e282 STOPPED", calls)

    def test_instance_names_that_could_do_harm_are_ignored(self):
        self.write("list.out", "../evil 9d36e282-0355-4caf-91e0-daf0857c935b start\nA;b 9d36e282-0355-4caf-91e0-daf0857c935b start\n")
        _, calls = self.run_script()
        self.assertNotIn("up -d", calls)
        self.assertNotIn("add-shop", calls)


if __name__ == "__main__":
    unittest.main()
