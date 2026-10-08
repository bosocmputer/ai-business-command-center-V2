#!/bin/sh
# Start the assistant: put the repo's config and persona in place, run the gateway, and once a day (04:00 Bangkok)
# stop it for a few seconds so maintain.sh can delete expired conversations while nothing holds the store open.
# If the gateway exits on its own, so does this script, with the same status, and compose restarts the container.
set -u
HERMES=/opt/hermes/bin/hermes
MAINT_HOUR="${MAINTENANCE_HOUR_UTC:-21}"
# Fail closed: a bot token without a list of allowed people would let anyone who finds the bot ask for the shop's numbers.
if [ -n "${TELEGRAM_BOT_TOKEN:-}" ] && [ -z "${TELEGRAM_ALLOWED_USERS:-}" ]; then
  echo '{"telegram":"not started: TELEGRAM_BOT_TOKEN is set but TELEGRAM_ALLOWED_USERS is empty"}'
  unset TELEGRAM_BOT_TOKEN
fi
# Hermes nags with "no home channel is set, type /sethome" until one exists; /sethome is not allowed to users, so the
# first allowed person's private chat (the chat id of a private chat is the user id) is the home channel.
if [ -n "${TELEGRAM_ALLOWED_USERS:-}" ] && [ -z "${TELEGRAM_HOME_CHANNEL:-}" ]; then
  export TELEGRAM_HOME_CHANNEL="${TELEGRAM_ALLOWED_USERS%%,*}"
fi
cp /assistant/config.yaml /opt/data/config.yaml
# Alerts: the routes AI-BCC's worker posts to (see render_routes.py). Without a secret nothing is opened.
/opt/hermes/.venv/bin/python /assistant/render_routes.py
# The /new reply: a Thai line instead of an English one, and no random English "tip" (see patch_locale.py).
if /opt/hermes/.venv/bin/python /assistant/patch_locale.py /tmp/hermes-locales; then
  export HERMES_BUNDLED_LOCALES=/tmp/hermes-locales
else
  echo '{"locale":"default messages kept: patch_locale.py found nothing to change"}'
fi
cp /assistant/SOUL.md /opt/data/SOUL.md
GW=""
trap '[ -n "$GW" ] && kill -TERM "$GW" 2>/dev/null; [ -n "$GW" ] && wait "$GW"; exit 0' TERM INT
last_day=""
GUARD_TICKS="${MEMORY_GUARD_TICKS:-20}"   # 20 x 30 s = every 10 minutes
# Once a day, from this minute of the UTC day (30 = 00:30 UTC = 07:30 Bangkok) for two hours, fetch the reports owners ask
# about first so the first question of the day is not slow. It runs beside the gateway and does not stop it.
PREWARM_FROM="${PREWARM_FROM_MINUTE:-30}"
last_prewarm=""
while true; do
  "$HERMES" gateway run &
  GW=$!
  maintenance=0
  ticks=0
  while kill -0 "$GW" 2>/dev/null; do
    if [ "$(date -u +%H)" = "$MAINT_HOUR" ] && [ "$last_day" != "$(date -u +%F)" ]; then
      if /opt/hermes/.venv/bin/python /assistant/is_idle.py; then
        last_day="$(date -u +%F)"; maintenance=1; break
      elif [ "${last_wait:-}" != "$(date -u +%F)" ]; then
        last_wait="$(date -u +%F)"; echo '{"maintenance":"waiting: a conversation is running"}'
      fi
    fi
    h=$(date -u +%H); m=$(date -u +%M); now_minute=$(( ${h#0} * 60 + ${m#0} ))
    if [ "$now_minute" -ge "$PREWARM_FROM" ] && [ "$now_minute" -lt $((PREWARM_FROM + 120)) ] && [ "$last_prewarm" != "$(date -u +%F)" ]; then
      last_prewarm="$(date -u +%F)"
      /opt/hermes/.venv/bin/python /assistant/prewarm.py &
    fi
    ticks=$((ticks + 1))
    if [ $((ticks % GUARD_TICKS)) = 0 ]; then /opt/hermes/.venv/bin/python /assistant/memory_guard.py --apply --locked; fi
    sleep 30 & wait $!
  done
  if [ "$maintenance" = 0 ]; then
    wait "$GW"; exit $?
  fi
  kill -TERM "$GW" 2>/dev/null
  for _ in $(seq 1 60); do kill -0 "$GW" 2>/dev/null || break; sleep 1; done
  kill -KILL "$GW" 2>/dev/null; wait "$GW" 2>/dev/null
  GW=""
  /bin/sh /assistant/maintain.sh
done
