#!/bin/sh
# Delete the assistant's stored conversations at the shop's request, and say what is left.
#   backend/deploy$ ./assistant/forget.sh --all          every conversation of this shop's assistant
#   backend/deploy$ ./assistant/forget.sh --chat-id ID   one chat (the id Telegram or LINE gave it)
# Also empties the assistant's log files (they are operational only), then compacts the store so the text is gone
# from the files too. Backups made by backup.sh keep their copy until they expire (30 days).
set -eu
cd "$(dirname "$0")/.."
export COMPOSE_FILE="${COMPOSE_FILE:-compose.local.yml:assistant/compose.assistant.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
case "${1:-}" in
  --all) FILTER="--older-than 1s"; WHAT="every conversation";;
  --chat-id) [ -n "${2:-}" ] || { echo "chat id required" >&2; exit 2; }; FILTER="--chat-id $2"; WHAT="the conversations of chat $2";;
  *) echo "usage: forget.sh --all | --chat-id ID" >&2; exit 2;;
esac
dc() { docker compose --env-file "$ENV_FILE" "$@"; }
HERMES=/opt/hermes/bin/hermes
echo "This deletes $WHAT from the assistant of this deployment."
printf 'Type DELETE to continue: '; read -r answer; [ "$answer" = "DELETE" ] || { echo "cancelled"; exit 1; }
dc exec -T assistant $HERMES sessions prune $FILTER --yes --include-archived --include-pinned
dc exec -T assistant sh -c 'for f in /opt/data/logs/*; do [ -f "$f" ] && : > "$f"; done; true'
dc exec -T assistant $HERMES sessions optimize >/dev/null
dc exec -T assistant $HERMES sessions stats
