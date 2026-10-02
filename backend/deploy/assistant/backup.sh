#!/bin/sh
# Back up the assistant's data volume before an update (conversations included, so the file is mode 0600 and expires).
#   backend/deploy$ ./assistant/backup.sh        -> backups/assistant/assistant-YYYYmmdd-HHMMSS.tgz ; older than 30 days deleted
set -eu
cd "$(dirname "$0")/.."
export COMPOSE_FILE="${COMPOSE_FILE:-compose.local.yml:assistant/compose.assistant.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
umask 077
mkdir -p backups/assistant
OUT="backups/assistant/assistant-$(date +%Y%m%d-%H%M%S).tgz"
VOLUME="$(docker compose --env-file "$ENV_FILE" config --format json | python3 -c 'import json,sys; c=json.load(sys.stdin); print(c["name"] + "_assistant_data")')"
docker run --rm -v "$VOLUME":/d:ro -v "$PWD/backups/assistant":/b --entrypoint sh alpine:3 -c "tar czf /b/$(basename "$OUT") -C /d . && chmod 600 /b/$(basename "$OUT")"
find backups/assistant -name 'assistant-*.tgz' -mtime +30 -delete
echo "backup written: $OUT ($(du -h "$OUT" | cut -f1))"
