#!/bin/sh
# usage: ask.sh <a|b> "<question in Thai>" [model]
# One question to Hermes for one shop, in a locked-down container. Prints the answer, the tool calls it made and the usage.
set -eu
cd "$(dirname "$0")"
SHOP="$1"; QUESTION="$2"; MODEL="${3:-${SPIKE_MODEL:-qwen/qwen3.8-27b:free}}"
IMAGE=nousresearch/hermes-agent:v2026.9.24
[ -f openrouter.env ] || { echo "missing openrouter.env" >&2; exit 2; }
mkdir -p runs
RUN="runs/$(date +%H%M%S)-$SHOP"
BEFORE=$(wc -l < data/calls.jsonl 2>/dev/null || echo 0)
START=$(date +%s)
docker run --rm --init --network hermes-spike --user 10000:10000 --cap-drop ALL --security-opt no-new-privileges \
  --memory 1g --cpus 1 --pids-limit 256 --read-only --tmpfs /tmp:size=64m \
  --env-file openrouter.env -e HERMES_HOME=/opt/data -e HOME=/opt/data \
  -v "$PWD/home-$SHOP":/opt/data -v "$PWD":/spike:ro \
  --entrypoint /opt/hermes/bin/hermes "$IMAGE" \
  -m "$MODEL" --provider openrouter --usage-file /opt/data/usage.json -z "$QUESTION" > "$RUN.out" 2> "$RUN.err" || echo "exit=$?" >> "$RUN.err"
END=$(date +%s)
echo "RUN=$RUN"
echo "== shop $SHOP | $MODEL | $((END-START))s | Q: $QUESTION"
cat "$RUN.out"
echo "-- tool calls seen by the API:"
tail -n +$((BEFORE+1)) data/calls.jsonl 2>/dev/null | python3 -c '
import json,sys
for l in sys.stdin:
    d=json.loads(l); print("  ", d.get("shop"), d.get("path"), d.get("query") or "", "->", d.get("status"))' || true
docker run --rm --entrypoint sh -v "$PWD/home-$SHOP":/d "$IMAGE" -c 'cat /d/usage.json 2>/dev/null; rm -f /d/usage.json' | python3 -c '
import json,sys
try:
    u=json.load(sys.stdin); t=u.get("total_including_auxiliary",{})
    print("-- usage: calls=%s input=%s cache_read=%s output=%s total=%s completed=%s" % (u.get("api_calls"),u.get("input_tokens"),u.get("cache_read_tokens"),u.get("output_tokens"),t.get("total_tokens"),u.get("completed")))
except Exception: print("-- usage: n/a")' || true
[ -s "$RUN.err" ] && { echo "-- stderr:"; tail -n 4 "$RUN.err"; }
true
