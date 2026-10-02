#!/bin/sh
# Back up the assistant's data volume before an update (conversations included, so the file is mode 0600 and expires).
# The assistant is stopped for the copy, so the store is consistent.
#   backend/deploy$ ./assistant/backup.sh        -> backups/assistant/assistant-YYYYmmdd-HHMMSS.tgz ; older than 30 days deleted
set -eu
cd "$(dirname "$0")/.."
export COMPOSE_FILE="${COMPOSE_FILE:-compose.local.yml:assistant/compose.assistant.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
umask 077
mkdir -p backups/assistant
NAME="assistant-$(date +%Y%m%d-%H%M%S).tgz"
VOLUME="$(docker compose --env-file "$ENV_FILE" config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["name"] + "_assistant_data")')"
trap 'docker compose --env-file "$ENV_FILE" start assistant >/dev/null' EXIT
docker compose --env-file "$ENV_FILE" stop assistant >/dev/null
docker run --rm -v "$VOLUME":/d:ro -v "$PWD/backups/assistant":/b --entrypoint sh alpine:3 -c "tar czf /b/$NAME -C /d . && chmod 600 /b/$NAME"
find backups/assistant -name 'assistant-*.tgz' -mtime +30 -delete
echo "backup written: backups/assistant/$NAME ($(du -h "backups/assistant/$NAME" | cut -f1))"
