#!/bin/sh
# Long-running helper for the admin Monitor page. Every few seconds it writes the
# per-container CPU and memory reported by `docker stats` to a file the API reads
# through its read-only host mount. The API never gets the docker socket.
set -eu

umask 022
runtime_dir=${SENTINEL_HOST_RUNTIME_DIR:-/run/nextstep-dashboard}
interval=${MONITOR_INTERVAL_SECONDS:-5}
target="$runtime_dir/host/containers.json"

case "$interval" in ''|*[!0-9]*) echo "MONITOR_INTERVAL_SECONDS must be a whole number." >&2; exit 1 ;; esac
install -d -o root -g root -m 0755 "$runtime_dir" "$runtime_dir/host"

while :; do
  checked_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  rows="$target.rows.$$"
  temporary="$target.tmp.$$"
  # Keep the last good file when docker is briefly unavailable; the API treats a
  # file older than 30 seconds as "container stats unavailable".
  if docker stats --no-stream --format '{{json .}}' > "$rows" 2>/dev/null; then
    { printf '{"version":1,"checkedAt":"%s","containers":[' "$checked_at"; paste -sd, "$rows"; printf ']}\n'; } > "$temporary"
    if [ "$(wc -c < "$temporary")" -le 65536 ]; then
      chmod 0644 "$temporary"
      mv "$temporary" "$target"
    fi
  fi
  rm -f "$rows" "$temporary"
  sleep "$interval"
done
