#!/bin/sh
# usage (from backend/deploy):  ./erase.sh shop <tenant-uuid> [--people] [--execute --confirm <tenant-uuid>]
#                               ./erase.sh person <recipient-uuid> [--allow-assistant-holder] [--execute --confirm <recipient-uuid>]
# Without --execute it only shows what would be removed (counts per table, never names). The shop must be switched off in the admin page first.
# Take a backup first when you really delete:  ./backup.sh .env.production pre-migration
set -eu
cd "$(dirname "$0")"
FILES="compose.local.yml"
[ -f assistant/compose.assistant.yml ] && FILES="$FILES:assistant/compose.assistant.yml"
export COMPOSE_FILE="$FILES"
exec docker compose --env-file .env.production exec -T worker /app/erase "$@"
