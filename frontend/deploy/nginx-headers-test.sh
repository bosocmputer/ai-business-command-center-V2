#!/bin/sh
set -eu

base_url=${1:-http://127.0.0.1:18080}
temporary=$(mktemp -d "${TMPDIR:-/tmp}/nextstep-nginx-headers.XXXXXX")
trap 'rm -rf "$temporary"' EXIT INT TERM

fetch() {
  host=$1
  name=$2
  curl -fsS --max-time 10 -H "Host: $host" -D "$temporary/$name.headers" -o "$temporary/$name.body" "$base_url/"
}

header_exists() {
  grep -Eqi "^$1:" "$2"
}

fetch 10.121.20.83 direct
fetch dashboard.nextstep-soft.com production

grep -Eqi '^referrer-policy:[[:space:]]*no-referrer' "$temporary/direct.headers"
grep -Eqi '^referrer-policy:[[:space:]]*no-referrer' "$temporary/production.headers"

if header_exists strict-transport-security "$temporary/direct.headers" ||
   header_exists cross-origin-opener-policy "$temporary/direct.headers" ||
   grep -Eqi '^content-security-policy:.*upgrade-insecure-requests' "$temporary/direct.headers"; then
  echo "HTTP origin received HTTPS-only security headers" >&2
  exit 1
fi

header_exists strict-transport-security "$temporary/production.headers"
header_exists cross-origin-opener-policy "$temporary/production.headers"
grep -Eqi '^content-security-policy:.*upgrade-insecure-requests' "$temporary/production.headers"
grep -Eqi '^content-security-policy:.*script-src[^;]*https://static\.line-scdn\.net' "$temporary/production.headers"

asset_path=$(grep -Eo '/assets/[^" ]+\.js' "$temporary/direct.body" | head -1)
if [ -z "$asset_path" ]; then
  echo "Frontend HTML did not contain a JavaScript asset" >&2
  exit 1
fi
curl -fsS --max-time 10 -H 'Host: 10.121.20.83' "$base_url$asset_path" >/dev/null

echo "Nginx HTTP and production-host header tests passed."
