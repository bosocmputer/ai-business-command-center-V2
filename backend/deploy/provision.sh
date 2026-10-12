#!/bin/sh
# Creates and stops the assistants that admins asked for in the admin page (tab "เลขา AI"). Started every minute by cron (see
# provision-install.sh); by hand, from backend/deploy:  ./provision.sh
#
# AI-BCC keeps a row per shop saying what should run. This script asks it (through /app/provision in the worker container: no network
# port is opened for it), and for each thing to do:
#   start: assistant/add-shop.sh <instance> (its compose file, network and secrets file), writes the assistant token AI-BCC kept sealed into
#          secrets/assistant/hermes-<instance>.env, starts the containers, waits until they are healthy, then reports RUNNING and has AI-BCC
#          forget the token. A failure is reported as ERROR with a short code; the admin sees it and can try again.
#   stop:  removes the instance's containers (its volume and settings stay) and reports STOPPED.
# The token only passes through a pipe into the secrets file (mode 0600); it is never printed or put in a command line.
set -u
cd "$(dirname "$0")"
. ./assistant/compose-files.sh
dc() { docker compose --env-file .env.production "$@"; }
prov() { dc exec -T worker /app/provision "$@"; }

report() { prov report "$@" >/dev/null 2>&1; }

start_instance() {
  name="$1"
  report "$name" CREATING
  if ! ./assistant/add-shop.sh "$name" >/dev/null 2>&1; then report "$name" ERROR ADD_SHOP_FAILED; return; fi
  . ./assistant/compose-files.sh   # the file add-shop.sh just wrote must be part of the stack
  file="secrets/assistant/hermes-$name.env"
  umask 077
  tmp=$(mktemp "$file.XXXXXX") || { report "$name" ERROR NO_TEMP_FILE; return; }
  grep -v '^AIBCC_TOKEN=' "$file" > "$tmp" || true
  if token=$(prov token "$name" 2>/dev/null) && [ -n "$token" ]; then
    printf 'AIBCC_TOKEN=%s\n' "$token" >> "$tmp"
  elif ! grep -q '^AIBCC_TOKEN=' "$file"; then
    rm -f "$tmp"; report "$name" ERROR NO_TOKEN; return
  else
    grep '^AIBCC_TOKEN=' "$file" >> "$tmp"   # already written by an earlier run; keep it
  fi
  token=""
  mv "$tmp" "$file"; chmod 600 "$file"
  if ! dc up -d --no-build "assistant-$name" >/dev/null 2>&1; then report "$name" ERROR COMPOSE_UP_FAILED; return; fi
  tries=0
  while [ "$tries" -lt "${PROVISION_HEALTH_TRIES:-40}" ]; do
    id=$(dc ps -q "assistant-$name" 2>/dev/null | head -n 1)
    health=""
    [ -n "$id" ] && health=$(docker inspect --format '{{.State.Health.Status}}' "$id" 2>/dev/null)
    if [ "$health" = healthy ]; then
      prov ack "$name" >/dev/null 2>&1
      report "$name" RUNNING
      return
    fi
    tries=$((tries + 1)); sleep "${PROVISION_HEALTH_SLEEP:-5}"
  done
  report "$name" ERROR START_TIMEOUT
}

stop_instance() {
  name="$1"
  if dc rm -sf "assistant-$name" "assistant-$name-egress" >/dev/null 2>&1; then report "$name" STOPPED; else report "$name" ERROR STOP_FAILED; fi
}

work=$(prov list 2>/dev/null) || exit 0
[ -n "$work" ] || exit 0
printf '%s\n' "$work" | while read -r name tenant action; do
  case "$name" in ""|*[!a-z0-9-]*) continue;; esac
  case "$action" in
    start) start_instance "$name";;
    stop) stop_instance "$name";;
  esac
done
