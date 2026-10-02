#!/bin/sh
# Delete the assistant's stored conversations at the shop's request, and say what is left.
#   backend/deploy$ ./assistant/forget.sh --all          every conversation of this shop's assistant
#   backend/deploy$ ./assistant/forget.sh --chat-id ID   one chat (the id Telegram or LINE gave it)
#   backend/deploy$ ./assistant/forget.sh --memory       only what the assistant remembers about the owner (--all clears it too)
# The assistant is stopped for a few seconds (Hermes refuses to delete while it holds the store), its log files are
# emptied (they are operational only) and the store is compacted so the text is gone from the files too. Backups made
# by backup.sh keep their copy until they expire (30 days).
set -eu
cd "$(dirname "$0")/.."
export COMPOSE_FILE="${COMPOSE_FILE:-compose.local.yml:assistant/compose.assistant.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
case "${1:-}" in
  --all) FILTER="--all"; WHAT="every conversation";;
  --memory) FILTER=""; WHAT="everything the assistant remembers about the owner";;
  --chat-id) [ -n "${2:-}" ] || { echo "chat id required" >&2; exit 2; }; FILTER="--chat-id $2"; WHAT="the conversations of chat $2";;
  *) echo "usage: forget.sh --all | --chat-id ID | --memory" >&2; exit 2;;
esac
dc() { docker compose --env-file "$ENV_FILE" "$@"; }
hermes() { dc run --rm --no-deps -T --entrypoint /opt/hermes/bin/hermes assistant "$@"; }
echo "This deletes $WHAT from the assistant of this deployment (it is stopped for a few seconds)."
printf 'Type DELETE to continue: '; read -r answer; [ "$answer" = "DELETE" ] || { echo "cancelled"; exit 1; }
trap 'dc start assistant >/dev/null' EXIT
dc stop assistant >/dev/null
if [ -n "$FILTER" ]; then
  dc run --rm --no-deps -T --entrypoint /opt/hermes/.venv/bin/python assistant /assistant/purge_sessions.py $FILTER
fi
if [ "$1" = "--all" ] || [ "$1" = "--memory" ]; then
  dc run --rm --no-deps -T --entrypoint sh assistant -c 'rm -f /opt/data/memories/*.md; echo "memory cleared"'
fi
dc run --rm --no-deps -T --entrypoint sh assistant -c 'for f in /opt/data/logs/*; do [ -f "$f" ] && : > "$f"; done; true'
hermes sessions optimize >/dev/null
hermes sessions stats
