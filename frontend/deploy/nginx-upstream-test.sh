#!/bin/sh
set -eu

image=${1:?frontend image tag is required}
port=${2:-18081}
suffix=$$
network="nextstep-nginx-upstream-$suffix"
frontend="nextstep-nginx-frontend-$suffix"
api_one="nextstep-nginx-api-one-$suffix"
api_two="nextstep-nginx-api-two-$suffix"
spacer="nextstep-nginx-spacer-$suffix"

cleanup() {
  docker rm -f "$frontend" "$api_one" "$api_two" "$spacer" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

start_api() {
  name=$1
  revision=$2
  docker run --rm -d \
    --name "$name" \
    --network "$network" \
    --network-alias api \
    node:22-alpine \
    node -e "require('http').createServer((_request, response) => response.end('$revision')).listen(8080, '0.0.0.0')" \
    >/dev/null
}

docker network create "$network" >/dev/null
start_api "$api_one" v1
docker run --rm -d \
  --name "$frontend" \
  --network "$network" \
  -p "127.0.0.1:$port:8080" \
  "$image" \
  >/dev/null

attempt=0
until [ "$(curl -fsS --max-time 2 "http://127.0.0.1:$port/api/revision" 2>/dev/null || true)" = v1 ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 30 ]; then
    echo "Frontend did not proxy to the initial API container" >&2
    exit 1
  fi
  sleep 1
done

docker rm -f "$api_one" >/dev/null
docker run --rm -d \
  --name "$spacer" \
  --network "$network" \
  node:22-alpine \
  node -e "setInterval(() => {}, 1000)" \
  >/dev/null
start_api "$api_two" v2

attempt=0
until [ "$(curl -fsS --max-time 2 "http://127.0.0.1:$port/api/revision" 2>/dev/null || true)" = v2 ]; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 15 ]; then
    echo "Frontend did not re-resolve the API container after replacement" >&2
    exit 1
  fi
  sleep 1
done

echo "Nginx API upstream re-resolution test passed."
