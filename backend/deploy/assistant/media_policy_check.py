"""Which files can the gateway send to a chat? Run inside the assistant container (upgrade-check.sh does):

  docker exec -i <container> /opt/hermes/.venv/bin/python - < assistant/media_policy_check.py

Only a file in the outbox (where make_file writes) may go out; the config, the persona, the credentials, system files and a file
that was just written anywhere else may not. Prints PASS or FAIL per path and exits 1 if any failed.
"""
import os
import sys

sys.path.insert(0, "/opt/hermes")
from gateway.media_policy import apply_media_policy_env  # noqa: E402
from gateway.platforms.base import validate_media_delivery_path  # noqa: E402

apply_media_policy_env()
os.makedirs("/opt/data/outbox/check", exist_ok=True)
allowed = "/opt/data/outbox/check/ตรวจ.csv"
fresh_elsewhere = "/tmp/fresh-file.txt"
for path in (allowed, fresh_elsewhere):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write("x")
cases = [(allowed, True), (fresh_elsewhere, False), ("/opt/data/config.yaml", False), ("/opt/data/SOUL.md", False),
         ("/opt/data/auth.json", False), ("/etc/passwd", False), ("/opt/data/outbox/check/../../config.yaml", False)]
failed = 0
for path, want in cases:
    got = validate_media_delivery_path(path) is not None
    ok = got == want
    failed += not ok
    print(("PASS" if ok else "FAIL"), "would send" if got else "refused", path, flush=True)
os.remove(allowed)
os.remove(fresh_elsewhere)
os.rmdir("/opt/data/outbox/check")
sys.exit(1 if failed else 0)
