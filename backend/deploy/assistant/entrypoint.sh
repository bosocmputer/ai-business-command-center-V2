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
cp /assistant/config.yaml /opt/data/config.yaml
cp /assistant/SOUL.md /opt/data/SOUL.md
GW=""
trap '[ -n "$GW" ] && kill -TERM "$GW" 2>/dev/null; [ -n "$GW" ] && wait "$GW"; exit 0' TERM INT
last_day=""
GUARD_TICKS="${MEMORY_GUARD_TICKS:-20}"   # 20 x 30 s = every 10 minutes
while true; do
  "$HERMES" gateway run &
  GW=$!
  maintenance=0
  ticks=0
  while kill -0 "$GW" 2>/dev/null; do
    if [ "$(date -u +%H)" = "$MAINT_HOUR" ] && [ "$last_day" != "$(date -u +%F)" ]; then
      last_day="$(date -u +%F)"; maintenance=1; break
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
