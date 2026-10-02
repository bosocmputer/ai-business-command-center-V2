"""Healthy when the gateway says it is running and its session store is fine."""
import json
import sys

try:
    state = json.load(open("/opt/data/gateway_state.json"))
    ok = state.get("gateway_state") == "running" and state.get("session_store", {}).get("status") == "ok"
except (OSError, ValueError):
    ok = False
sys.exit(0 if ok else 1)
