#!/bin/sh
# usage (from backend/deploy):  ./assistant/upgrade-check.sh <candidate-image-ref> [question-numbers]
#   e.g. ./assistant/upgrade-check.sh nousresearch/hermes-agent:v2026.10.2@sha256:... 5,9,13
# Runs a CANDIDATE Hermes image beside the live assistant, with a made-up Telegram token (so nothing can be sent) and a fresh empty memory, then
# checks what an update must never break, and removes it. The live assistant is not touched; nobody outside is messaged.
#   1. the gateway starts and reports healthy;
#   2. no built-in tool is enabled on any channel (cli, api_server, telegram, webhook) except memory, which is on by decision;
#   3. the MCP shim exposes exactly the tools this repository defines;
#   4. real questions (assistant/example_check.py) pass against the real Agent API.
# It uses the live token, so it spends the hourly call quota (about 3 per question): keep the question list short.
# Only change the pinned image in compose.assistant.yml after this prints "UPGRADE CHECK PASSED" (see assistant/README.md).
set -eu
IMAGE="${1:?candidate image reference required (name:tag@sha256:digest)}"
ONLY="${2:-5,9,13}"
cd "$(dirname "$0")/.."
NET=$(docker network ls --format '{{.Name}}' | grep -E '(^|_)agent$' | head -1)
[ -n "$NET" ] || { echo "the agent network is not there; start the stack with both compose files first" >&2; exit 2; }
SUFFIX=$$
NAME="assistant-staging-$SUFFIX"
VOLUME="assistant_staging_$SUFFIX"
ENVFILE=$(mktemp)
chmod 600 "$ENVFILE"
cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  docker volume rm "$VOLUME" >/dev/null 2>&1 || true
  rm -f "$ENVFILE"
}
trap cleanup EXIT INT TERM
# The live secrets minus everything that would reach people: no real Telegram bot, allowed user or alert destination.
grep -v -E '^(TELEGRAM_|ALERT_CHAT_IDS)' secrets/assistant/hermes.env > "$ENVFILE"
# The webhook channel only exists when a bot and a chat id are configured (render_routes.py), and its tools can be listed only
# then. So the candidate gets a made-up bot token and chat id: Telegram rejects the token, nothing can be sent, and the channel
# is there to be checked.
{
  echo 'TELEGRAM_BOT_TOKEN=123456789:AAstagingOnlyNotARealToken0000000000000'
  echo 'TELEGRAM_ALLOWED_USERS=1'
  echo 'ALERT_CHAT_IDS=1'
} >> "$ENVFILE"
echo "pulling $IMAGE"
docker pull -q "$IMAGE" >/dev/null
docker volume create "$VOLUME" >/dev/null
docker run --rm --network none --user 0:0 --cap-drop ALL --cap-add CHOWN --cap-add FOWNER -v "$VOLUME":/opt/data --entrypoint sh "$IMAGE" -c 'chmod 0700 /opt/data && chown 10000:10000 /opt/data'
docker run -d --name "$NAME" --network "$NET" --user 10000:10000 --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --tmpfs /tmp:size=64m --memory 1g --cpus 1 --pids-limit 256 --init --env-file "$ENVFILE" \
  -e HERMES_HOME=/opt/data -e HOME=/opt/data -e AIBCC_URL=http://api:8080 -e API_SERVER_HOST=0.0.0.0 -e API_SERVER_PORT=8642 \
  -e HTTPS_PROXY=http://assistant-egress:3128 -e HTTP_PROXY=http://assistant-egress:3128 -e NO_PROXY=api,localhost,127.0.0.1 \
  -e RETENTION_HOURS=24 -e LOG_RETENTION_DAYS=7 -e MAINTENANCE_HOUR_UTC=21 -e MEMORY_GUARD_TICKS=20 -e PREWARM_FROM_MINUTE=1500 \
  -v "$VOLUME":/opt/data -v "$PWD/assistant":/assistant:ro --entrypoint /bin/sh "$IMAGE" /assistant/entrypoint.sh >/dev/null
PY=/opt/hermes/.venv/bin/python
failed=0
fail() { echo "  FAIL $1"; failed=$((failed + 1)); }

printf "1. gateway healthy ... "
ok=0
for _ in $(seq 1 40); do
  if docker exec "$NAME" "$PY" /assistant/healthcheck.py >/dev/null 2>&1; then ok=1; break; fi
  sleep 3
done
if [ "$ok" = 1 ]; then echo "ok"; else echo; fail "the gateway did not become healthy in 2 minutes"; docker exec "$NAME" sh -c 'head -c 700 /opt/data/gateway_state.json' 2>&1; echo; docker logs --tail 15 "$NAME" 2>&1 | cut -c1-200; fi

printf "2. no built-in tool enabled on any channel except memory ... "
bad=""
skipped=""
for platform in cli api_server telegram webhook; do
  listing=$(docker exec "$NAME" /opt/hermes/bin/hermes tools list --platform "$platform" 2>&1 || true)
  # Lines look like "  ✓ enabled  memory  ..." / "  ✗ disabled  web  ...". Memory is on by decision (guarded by memory_guard.py).
  if ! echo "$listing" | grep -q "disabled"; then
    # The webhook channel only exists when an alert route is configured; the candidate has none (no destinations), so it may have
    # nothing to list. The other three channels must always be readable.
    if [ "$platform" = webhook ]; then skipped="$skipped webhook"; continue; fi
    bad="$bad $platform(unreadable)"; continue
  fi
  extra=$(echo "$listing" | grep -E '^ *[^ ]+ enabled ' | grep -v -E ' enabled +memory( |$)' || true)
  if [ -n "$extra" ]; then bad="$bad $platform"; echo; echo "$extra" | sed 's/^/    /' | cut -c1-120; fi
done
if [ -z "$bad" ]; then echo "ok${skipped:+ (not listed by the candidate:$skipped; the live assistant is checked below)}"; else echo; fail "a built-in tool other than memory is enabled (or the list could not be read) on:$bad"; fi

printf "3. the shim exposes every tool of this repository ... "
want=$(grep -c '^@mcp.tool()' assistant/aibcc_mcp.py)
got=$(docker exec "$NAME" /opt/hermes/bin/hermes mcp test aibcc 2>&1 | grep -Eo 'Tools discovered: [0-9]+' | grep -Eo '[0-9]+' | head -1 || true)
if [ "${got:-0}" = "$want" ]; then echo "ok ($want tools)"; else echo; fail "expected $want tools, the candidate shows ${got:-0}"; fi

echo "4. real questions (EXAMPLE_ONLY=$ONLY) ..."
out=$(docker exec -e EXAMPLE_ONLY="$ONLY" -i "$NAME" "$PY" - < assistant/example_check.py 2>&1 || true)
echo "$out" | grep -E '^(PASS|FAIL|SUMMARY)' | cut -c1-160
if echo "$out" | grep -q '^FAIL'; then fail "a question failed"; fi
if ! echo "$out" | grep -q '^SUMMARY'; then fail "the question run did not finish"; fi

if [ "$failed" = 0 ]; then echo "UPGRADE CHECK PASSED for $IMAGE"; else echo "UPGRADE CHECK FAILED ($failed) for $IMAGE: do not change the pinned image"; exit 1; fi
