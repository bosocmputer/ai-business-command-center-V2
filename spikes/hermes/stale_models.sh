#!/bin/sh
# usage: stale_models.sh "model|provider" ...   Pin each model and run the stale-figure conversation test twice.
set -u
cd "$(dirname "$0")"
set -a; . ./openrouter.env; . ./tokens.env; set +a
for spec in "$@"; do
  model="${spec%%|*}"; provider="${spec##*|}"
  echo "=== $model via $provider"
  SPIKE_MODEL="$model" SPIKE_PROVIDER="$provider" ./mkhome.sh >/dev/null
  ./serve.sh a b >/dev/null; sleep 40
  for i in 1 2; do python3 stale_test.py "$model" 2>&1 | grep -E "^ask|SUMMARY|Error"; done
done
