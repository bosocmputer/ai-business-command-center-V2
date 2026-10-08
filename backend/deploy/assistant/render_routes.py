"""Add the alert webhook to the assistant's config, once per start.

AI-BCC's worker posts an alert, signed, to http://assistant:8644/webhooks/alert-N. Each route delivers the message as it is
(no model in between, so the figures are AI-BCC's own) to one Telegram chat and also writes it into that chat's history, so
the owner can answer "show me the list" and the assistant knows what the alert was about.

One route per person to tell: ALERT_CHAT_IDS (comma separated Telegram ids), by default everyone allowed to chat. At most five.
Nothing is added unless there is a secret of 32+ characters and at least one id. The secret is read from the environment
when Hermes starts; it is never written to the config file.
"""
import os
import sys

import yaml

CONFIG = "/opt/data/config.yaml"
MAX_ROUTES = 5

ids = []
for raw in (os.environ.get("ALERT_CHAT_IDS") or os.environ.get("TELEGRAM_ALLOWED_USERS") or "").split(","):
    raw = raw.strip()
    if raw.isdigit() and raw not in ids:
        ids.append(raw)
secret = os.environ.get("ALERT_WEBHOOK_SECRET", "")
if len(secret) < 32 or not ids or not os.environ.get("TELEGRAM_BOT_TOKEN"):
    print('{"alert_webhook":"not enabled: needs ALERT_WEBHOOK_SECRET (32+ characters), a Telegram bot and at least one chat id"}')
    sys.exit(0)

with open(CONFIG, encoding="utf-8") as handle:
    config = yaml.safe_load(handle) or {}
routes = {}
for number, chat_id in enumerate(ids[:MAX_ROUTES], 1):
    routes[f"alert-{number}"] = {
        "events": ["aibcc.alert"],
        "secret": "${ALERT_WEBHOOK_SECRET}",
        "prompt": "{message}",
        "deliver": "telegram",
        "deliver_extra": {"chat_id": chat_id},
        "deliver_only": True,
        "mirror_to_session": True,
    }
platforms = config.setdefault("platforms", {})
platforms["webhook"] = {"enabled": True, "extra": {"host": "0.0.0.0", "port": 8644, "routes": routes}}
config.setdefault("platform_toolsets", {})["webhook"] = []
with open(CONFIG, "w", encoding="utf-8") as handle:
    yaml.safe_dump(config, handle, allow_unicode=True, sort_keys=False)
print('{"alert_webhook":"enabled","routes":%d}' % len(routes))
