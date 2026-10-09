#!/bin/sh
# usage (on the server, from backend/deploy):  ./assistant/model-probe.sh <label> <openrouter-model-id> <provider-slugs, comma separated>
# Starts a throwaway assistant (memory-only storage, no Telegram, no LINE, no alerts) on the given model with the given providers pinned,
# puts ONE measurable question to it (model_probe.py), prints one JSON line (score, time, tokens, cost) and removes it. The question
# costs a few cents at most; the shop's own assistant is not touched. It reads the shop's numbers through the shop's token, read only.
set -eu
LABEL="${1:?label}"; MODEL="${2:?model id}"; PROVIDERS="${3:?provider slugs}"
cd "$(dirname "$0")/.."
WORK=$(mktemp -d /tmp/probe.XXXXXX)
NAME="probe-$LABEL-$$"
cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "$WORK"; }
trap cleanup EXIT
cp -r assistant "$WORK/assistant"
python3 - "$WORK/assistant/config.yaml" "$MODEL" "$PROVIDERS" <<'PY'
import re, sys
path, model, providers = sys.argv[1:4]
text = open(path).read()
text, a = re.subn(r"(?m)^(  default: ).*$", lambda m: m.group(1) + model, text, count=1)
text, b = re.subn(r"(?m)^(  only: )\[.*\]$", lambda m: m.group(1) + "[" + providers + "]", text, count=1)
if a != 1 or b != 1:
    sys.exit("config.yaml has no model or provider line to change")
open(path, "w").write(text)
PY
RUNNING=$(docker ps --format '{{.Names}}' | grep -E 'assistant-1$' | head -1)
IMAGE=$(docker inspect -f '{{.Config.Image}}' "$RUNNING")
NETWORK=$(docker network ls --format '{{.Name}}' | grep -E '_agent$' | head -1)
docker run -d --name "$NAME" --network "$NETWORK" --read-only --cap-drop ALL --security-opt no-new-privileges:true --user 10000:10000 --init --memory 1g \
  --env-file secrets/assistant/hermes.env \
  -e TELEGRAM_BOT_TOKEN= -e TELEGRAM_ALLOWED_USERS= -e TELEGRAM_HOME_CHANNEL= -e LINE_CHANNEL_ACCESS_TOKEN= -e LINE_CHANNEL_SECRET= -e LINE_ALLOWED_USERS= -e ALERT_WEBHOOK_SECRET= \
  -e HERMES_HOME=/opt/data -e HOME=/opt/data -e AIBCC_URL=http://api:8080 -e API_SERVER_HOST=0.0.0.0 -e API_SERVER_PORT=8642 \
  -e HTTPS_PROXY=http://assistant-egress:3128 -e HTTP_PROXY=http://assistant-egress:3128 -e NO_PROXY=api,localhost,127.0.0.1 \
  --tmpfs /tmp:size=64m --tmpfs /opt/data:uid=10000,gid=10000,mode=0700,size=300m \
  -v "$WORK/assistant:/assistant:ro" --entrypoint /bin/sh \
  "$IMAGE" /assistant/entrypoint.sh >/dev/null
i=0
until docker exec "$NAME" python3 -c "import socket;socket.create_connection(('127.0.0.1',8642),2)" 2>/dev/null; do
  i=$((i+1)); [ "$i" -lt 60 ] || { echo '{"label":"'"$LABEL"'","error":"assistant did not start"}'; exit 1; }; sleep 3
done
docker exec "$NAME" /opt/hermes/.venv/bin/python /assistant/model_probe.py "$LABEL"
