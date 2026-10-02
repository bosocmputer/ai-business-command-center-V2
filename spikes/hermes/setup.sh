#!/bin/sh
# Prepare the Hermes spike: a private network, the mock API, one data directory per shop.
set -eu
cd "$(dirname "$0")"
IMAGE=nousresearch/hermes-agent:v2026.9.24
umask 077
[ -f tokens.env ] || { printf 'TOKEN_A=%s\nTOKEN_B=%s\n' "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" > tokens.env; }
. ./tokens.env
mkdir -p data home-a home-b
chmod 777 data 2>/dev/null || true
docker network inspect hermes-spike >/dev/null 2>&1 || docker network create hermes-spike >/dev/null
docker rm -f aibcc-mock >/dev/null 2>&1 || true
docker run -d --name aibcc-mock --network hermes-spike --init --read-only --cap-drop ALL --memory 128m --pids-limit 64 \
  -e TOKEN_A="$TOKEN_A" -e TOKEN_B="$TOKEN_B" -v "$PWD":/spike:ro -v "$PWD/data":/data \
  --entrypoint /opt/hermes/.venv/bin/python "$IMAGE" /spike/mock_api.py 8099 >/dev/null
sleep 2
docker ps --filter name=aibcc-mock --format '{{.Names}} {{.Status}}'
