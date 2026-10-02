#!/bin/sh
# Runs in the assistant-retention sidecar. Every hour: delete conversations that have been idle longer than
# RETENTION_HOURS, delete log files older than LOG_RETENTION_DAYS, and compact the store so deleted text leaves
# the file. The assistant keeps conversations only as working memory; AI-BCC holds any longer record.
set -u
HOURS="${RETENTION_HOURS:-24}"
LOG_DAYS="${LOG_RETENTION_DAYS:-7}"
HERMES=/opt/hermes/bin/hermes
case "$HOURS$LOG_DAYS" in *[!0-9]*|"") echo '{"retention":"bad settings, refusing to run"}'; exit 1;; esac
while true; do
  if "$HERMES" sessions prune --older-than "${HOURS}h" --yes --include-archived --include-pinned > /tmp/prune.out 2>&1; then
    result=ok
  else
    result=failed
  fi
  find /opt/data/logs -type f -mtime +"$LOG_DAYS" -delete 2>/dev/null
  "$HERMES" sessions optimize > /tmp/optimize.out 2>&1 || result="$result,optimize-failed"
  printf '{"retention":"%s","idle_hours":%s,"log_days":%s,"t":%s}\n' "$result" "$HOURS" "$LOG_DAYS" "$(date +%s)"
  sleep 3600
done
