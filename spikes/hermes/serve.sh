#!/bin/sh
# usage: serve.sh [a|b ...]   Start one long-running Hermes gateway per shop (OpenAI-compatible API on the private
# network only, nothing published to the host). The MCP server inside stays alive between questions.
# The gateways sit on an internal network with no route out; the only way out is the allow-list proxy.
# EGRESS_ALLOW is the comma-separated host list (default: the model provider only; add api.telegram.org for Telegram).
set -eu
cd "$(dirname "$0")"
IMAGE=nousresearch/hermes-agent:v2026.9.24
umask 077
[ -f gateway.env ] || printf 'API_SERVER_KEY=%s\n' "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" > gateway.env
[ $# -gt 0 ] || set -- a b
ALLOW="${EGRESS_ALLOW:-openrouter.ai}"
mkdir -p data && touch data/egress.jsonl && chmod 666 data/egress.jsonl
docker network inspect hermes-inner >/dev/null 2>&1 || docker network create --internal hermes-inner >/dev/null
docker network connect hermes-inner aibcc-mock >/dev/null 2>&1 || true
docker rm -f egress-proxy >/dev/null 2>&1 || true
docker run -d --name egress-proxy --init --network hermes-spike --read-only --cap-drop ALL --security-opt no-new-privileges --memory 128m --pids-limit 64 \
  -e EGRESS_ALLOW="$ALLOW" -v "$PWD":/spike:ro -v "$PWD/data":/data --entrypoint /opt/hermes/.venv/bin/python "$IMAGE" /spike/egress_proxy.py 3128 >/dev/null
docker network connect hermes-inner egress-proxy
for shop in "$@"; do
  docker rm -f "hermes-$shop" >/dev/null 2>&1 || true
  docker run -d --name "hermes-$shop" --init --network hermes-inner --user 10000:10000 --cap-drop ALL --security-opt no-new-privileges \
    --memory 1g --cpus 1 --pids-limit 256 --read-only --tmpfs /tmp:size=64m \
    --env-file openrouter.env --env-file gateway.env -e API_SERVER_HOST=0.0.0.0 -e API_SERVER_PORT=8642 \
    -e HERMES_HOME=/opt/data -e HOME=/opt/data \
    -e HTTPS_PROXY=http://egress-proxy:3128 -e HTTP_PROXY=http://egress-proxy:3128 -e NO_PROXY=aibcc-mock,localhost,127.0.0.1 \
    -v "$PWD/home-$shop":/opt/data -v "$PWD":/spike:ro \
    --entrypoint /opt/hermes/bin/hermes "$IMAGE" gateway run >/dev/null
done
docker ps --filter name=hermes- --filter name=egress --format '{{.Names}} {{.Status}}'
