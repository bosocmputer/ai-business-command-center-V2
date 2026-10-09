#!/bin/sh
# Daily maintenance, run by entrypoint.sh while the gateway is stopped: delete conversations idle for longer than
# RETENTION_HOURS, delete log files older than LOG_RETENTION_DAYS, compact the store so deleted text leaves the file.
# One JSON line to stdout (docker logs) per run. A refusal or failure is reported, never hidden.
set -u
HOURS="${RETENTION_HOURS:-24}"
LOG_DAYS="${LOG_RETENTION_DAYS:-7}"
HERMES=/opt/hermes/bin/hermes
case "$HOURS$LOG_DAYS" in *[!0-9]*|"") echo '{"maintenance":"bad settings, nothing deleted"}'; exit 1;; esac
result=ok
/opt/hermes/.venv/bin/python /assistant/purge_sessions.py --idle-hours "$HOURS" > /tmp/purge.out 2>&1 || result=purge-failed
cat /tmp/purge.out
/opt/hermes/.venv/bin/python /assistant/memory_guard.py --apply
/opt/hermes/.venv/bin/python -c "import sys; sys.path.insert(0, \"/assistant\"); import usage_ledger; usage_ledger.prune()" || result="$result,ledger-prune-failed"
find /opt/data/logs -type f -mtime +"$LOG_DAYS" -delete 2>/dev/null
# What the owner sent (pictures, files) and what the secretary made for the owner are kept no longer than a conversation.
/opt/hermes/.venv/bin/python -c "import sys; sys.path.insert(0, \"/assistant\"); import secretary_tools; secretary_tools.purge_outbox(max_age_seconds=$HOURS * 3600)" || result="$result,outbox-purge-failed"
find /opt/data/document_cache /opt/data/image_cache /opt/data/audio_cache /opt/data/video_cache /opt/data/cache -type f -mmin +$((HOURS * 60)) -delete 2>/dev/null
"$HERMES" sessions optimize > /tmp/optimize.out 2>&1 || result="$result,optimize-failed"
printf '{"maintenance":"%s","idle_hours":%s,"log_days":%s,"t":%s}\n' "$result" "$HOURS" "$LOG_DAYS" "$(date +%s)"
