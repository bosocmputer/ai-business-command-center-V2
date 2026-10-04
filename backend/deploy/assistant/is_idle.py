"""Exit 0 when the gateway has no conversation running (safe to stop it for maintenance), 1 when one is active.
Unreadable state counts as idle: maintenance must not wait forever on a broken state file."""
import json
import os
import sys

path = os.environ.get("GATEWAY_STATE", "/opt/data/gateway_state.json")
try:
    active = int(json.load(open(path)).get("active_agents") or 0)
except (OSError, ValueError, TypeError):
    active = 0
sys.exit(0 if active == 0 else 1)
