#!/bin/sh
# usage (from backend/deploy):  ./onboard-check.sh <tenant-uuid> [json]
# Checks whether one shop is ready: its configuration here, that every approved report query runs on its SML, which document
# types it uses against the ones the reports count, and the gaps in its data that change what the numbers can say.
# It only reads and prints counts and aggregates (never names or per-customer figures). Exit code 1 when something failed.
set -eu
TENANT="${1:?tenant uuid required}"
FORMAT="${2:-text}"
cd "$(dirname "$0")"
FILES="compose.local.yml"
[ -f assistant/compose.assistant.yml ] && FILES="$FILES:assistant/compose.assistant.yml"
export COMPOSE_FILE="$FILES"
exec docker compose --env-file .env.production exec -T -e ONBOARDING_TENANT_ID="$TENANT" -e ONBOARDING_FORMAT="$FORMAT" worker /app/onboarding-check
