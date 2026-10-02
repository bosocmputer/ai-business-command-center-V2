#!/bin/sh
# usage: compare_models.sh "model|provider" ...   For each model: pin the provider, restart the gateways, run the
# 32-question set and the stale-figure test, and record what the key was charged. Nothing here prints the key.
set -u
cd "$(dirname "$0")"
set -a; . ./openrouter.env; . ./tokens.env; set +a
spent() { curl -s -H "Authorization: Bearer $OPENROUTER_API_KEY" https://openrouter.ai/api/v1/key | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["usage"])'; }
for spec in "$@"; do
  model="${spec%%|*}"; provider="${spec##*|}"; slug=$(echo "$model" | tr '/:' '__')
  echo "=== $model via $provider"
  python3 provider_check.py "$model" "$provider"
  SPIKE_MODEL="$model" SPIKE_PROVIDER="$provider" ./mkhome.sh >/dev/null
  ./serve.sh a b >/dev/null; sleep 40
  before=$(spent)
  python3 battery.py "$model" --gateway | grep -E "FAIL|SUMMARY"
  python3 stale_test.py "$model" | grep -E "^ask|SUMMARY"
  after=$(spent)
  python3 -c "print('cost of this model: \$%.3f' % ($after - $before))"
done
