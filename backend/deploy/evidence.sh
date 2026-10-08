#!/bin/sh
# usage (from backend/deploy):  ./evidence.sh capture <tenant-id> <from> <to> "<reason>" [your-name]
#                               ./evidence.sh list <tenant-id> | show <tenant-id> <case-id> > case.json | delete <tenant-id> <case-id> [your-name]
# Keeps the proof behind a disputed number for three years (see cmd/evidence).
set -eu
cd "$(dirname "$0")"
FILES="compose.local.yml"
[ -f assistant/compose.assistant.yml ] && FILES="$FILES:assistant/compose.assistant.yml"
export COMPOSE_FILE="$FILES"
exec docker compose --env-file .env.production exec -T worker /app/evidence "$@"
