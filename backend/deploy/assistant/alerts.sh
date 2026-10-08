#!/bin/sh
# Control the daily alerts from the server. Alerts are what the owner sets up by talking to the assistant ("tell me when
# overdue receivables pass 500,000"); AI-BCC's worker checks them each morning and sends a message through the assistant.
#
#   backend/deploy$ ./assistant/alerts.sh status    what is on, what fired recently, what the owners have set (no names, no figures of the shop)
#   ./assistant/alerts.sh dry     check every morning and RECORD what would be sent; send nothing
#   ./assistant/alerts.sh live    check every morning and SEND (needs the webhook secret; set up automatically)
#   ./assistant/alerts.sh off     stop checking
#   ./assistant/alerts.sh test    send one clearly-labelled test message through the assistant to ONE Telegram id: test <id>
#
# It edits .env.production (a copy is kept) and recreates the worker, which takes a few seconds.
set -eu
cd "$(dirname "$0")/.."
export COMPOSE_FILE="${COMPOSE_FILE:-compose.local.yml:assistant/compose.assistant.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
SECRETS=secrets/assistant/hermes.env
dc() { docker compose --env-file "$ENV_FILE" "$@"; }

set_env() { # name value : replace or append in .env.production
  python3 - "$ENV_FILE" "$1" "$2" <<'PY'
import sys
path, name, value = sys.argv[1:4]
lines = open(path).read().split("\n")
done = False
for index, line in enumerate(lines):
    if line.startswith(name + "="):
        lines[index] = f"{name}={value}"; done = True
if not done:
    if lines and lines[-1] == "": lines.pop()
    lines.append(f"{name}={value}")
    lines.append("")
open(path, "w").write("\n".join(lines))
PY
}

ensure_secret() { # one shared secret: hermes.env (assistant) and .env.production (worker). It is never printed.
  umask 077
  touch "$SECRETS"; chmod 600 "$SECRETS"
  if ! grep -q '^ALERT_WEBHOOK_SECRET=' "$SECRETS"; then
    printf 'ALERT_WEBHOOK_SECRET=%s\n' "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" >> "$SECRETS"
    echo "created the alert webhook secret"
  fi
  VALUE=$(grep '^ALERT_WEBHOOK_SECRET=' "$SECRETS" | cut -d= -f2-)
  set_env AGENT_ALERT_WEBHOOK_SECRET "$VALUE"
  set_env AGENT_ALERT_WEBHOOK_URL "http://assistant:8644"
}

apply() { # recreate the worker and the assistant so both read the new values
  dc up -d --no-build --force-recreate worker assistant 2>&1 | grep -E "Recreated|Started|Error" || true
}

case "${1:-status}" in
  dry)
    cp -p "$ENV_FILE" "$ENV_FILE.bak-alerts"
    ensure_secret
    set_env AGENT_ALERTS_ENABLED true; set_env AGENT_ALERT_DRY_RUN true
    apply; echo "alerts: DRY RUN (recorded, not sent)";;
  live)
    cp -p "$ENV_FILE" "$ENV_FILE.bak-alerts"
    ensure_secret
    set_env AGENT_ALERTS_ENABLED true; set_env AGENT_ALERT_DRY_RUN false
    apply; echo "alerts: LIVE (messages are sent)";;
  off)
    cp -p "$ENV_FILE" "$ENV_FILE.bak-alerts"
    set_env AGENT_ALERTS_ENABLED false; set_env AGENT_ALERT_DRY_RUN true
    apply; echo "alerts: OFF";;
  test)
    ID="${2:?usage: alerts.sh test <telegram id>}"
    case "$ID" in *[!0-9]*) echo "the id is digits only" >&2; exit 2;; esac
    dc exec -T -e TEST_CHAT_ID="$ID" assistant /opt/hermes/.venv/bin/python - <<'PY'
import hashlib, hmac, json, os, time, urllib.error, urllib.request
secret = os.environ.get("ALERT_WEBHOOK_SECRET", "")
ids = [i.strip() for i in (os.environ.get("ALERT_CHAT_IDS") or os.environ.get("TELEGRAM_ALLOWED_USERS") or "").split(",") if i.strip().isdigit()]
chat = os.environ["TEST_CHAT_ID"]
if not secret or chat not in ids:
    raise SystemExit("that id is not one of the people the assistant talks to, or there is no secret yet (run: alerts.sh dry)")
route = "alert-%d" % (ids.index(chat) + 1)
body = json.dumps({"event_type": "aibcc.alert", "message": "ทดสอบระบบแจ้งเตือนของผู้ช่วย (ข้อความทดสอบ ไม่ต้องทำอะไร)", "alertId": "test-%d" % time.time(), "rule": "test"}).encode()
stamp = str(int(time.time()))
signature = hmac.new(secret.encode(), stamp.encode() + b"." + body, hashlib.sha256).hexdigest()
request = urllib.request.Request("http://127.0.0.1:8644/webhooks/" + route, data=body, headers={"Content-Type": "application/json",
    "X-Webhook-Timestamp": stamp, "X-Webhook-Signature-V2": signature, "X-Request-ID": "test-%s" % stamp})
try:
    print("webhook answered", urllib.request.urlopen(request, timeout=15).status, "on", route)
except urllib.error.HTTPError as error:
    print("webhook refused:", error.code)
PY
    ;;
  status)
    echo "== switches (worker)"; grep -E '^AGENT_ALERT(S_ENABLED|_DRY_RUN)=' "$ENV_FILE" || echo "(not set: alerts are off)"
    echo "== webhook secret present: $(grep -c '^AGENT_ALERT_WEBHOOK_SECRET=.\{32,\}' "$ENV_FILE") (assistant: $(grep -c '^ALERT_WEBHOOK_SECRET=.\{32,\}' "$SECRETS" 2>/dev/null || echo 0))"
    dc exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off' <<'SQL'
select rule_key, enabled, count(*) as rules, count(*) filter (where last_checked_on = (now() at time zone 'Asia/Bangkok')::date) as checked_today,
       count(*) filter (where last_status = 'NOT_READY') as not_ready, count(*) filter (where last_status in ('ERROR', 'NO_ACCESS')) as problems
from agent_alert_rules group by 1, 2 order by 1, 2;
select to_char(created_at at time zone 'Asia/Bangkok', 'MM-DD HH24:MI') as at, rule_key, status, attempts, coalesce(last_error, '') as last_error
from agent_alert_events order by created_at desc limit 10;
SQL
    ;;
  *) echo "usage: alerts.sh status|dry|live|off|test <id>" >&2; exit 2;;
esac
