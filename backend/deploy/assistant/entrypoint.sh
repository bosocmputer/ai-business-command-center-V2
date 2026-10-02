#!/bin/sh
# Start the assistant: put the repo's config and persona in place, run the gateway, and once a day (04:00 Bangkok)
# stop it for a few seconds so maintain.sh can delete expired conversations while nothing holds the store open.
# If the gateway exits on its own, so does this script, with the same status, and compose restarts the container.
set -u
HERMES=/opt/hermes/bin/hermes
MAINT_HOUR="${MAINTENANCE_HOUR_UTC:-21}"
cp /assistant/config.yaml /opt/data/config.yaml
cp /assistant/SOUL.md /opt/data/SOUL.md
GW=""
trap '[ -n "$GW" ] && kill -TERM "$GW" 2>/dev/null; [ -n "$GW" ] && wait "$GW"; exit 0' TERM INT
last_day=""
while true; do
  "$HERMES" gateway run &
  GW=$!
  maintenance=0
  while kill -0 "$GW" 2>/dev/null; do
    if [ "$(date -u +%H)" = "$MAINT_HOUR" ] && [ "$last_day" != "$(date -u +%F)" ]; then
      last_day="$(date -u +%F)"; maintenance=1; break
    fi
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
