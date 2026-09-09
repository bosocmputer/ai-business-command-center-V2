#!/bin/sh
set -eu

container=${1:?frontend container name or id is required}
base_url=${2:-http://127.0.0.1:18080}
marker="delivery-reference-must-not-be-logged-$$"

curl -fsS --max-time 10 \
  -H "Referer: https://dashboard.nextstep-soft.com/app?deliveryRef=$marker" \
  "$base_url/app?deliveryRef=$marker" \
  >/dev/null

logs=$(docker logs "$container" 2>&1)
if printf '%s\n' "$logs" | grep -F "$marker" >/dev/null; then
  echo "Nginx access log exposed a query parameter or Referer" >&2
  exit 1
fi
printf '%s\n' "$logs" | grep -E 'GET /(app|index\.html) HTTP/1\.1' >/dev/null

echo "Nginx access log redaction test passed."
