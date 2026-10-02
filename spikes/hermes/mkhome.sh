#!/bin/sh
# Write the Hermes home for each shop: model, the one MCP server it may use, and the persona.
set -eu
cd "$(dirname "$0")"
umask 077
set -a; . ./tokens.env; set +a
# Hermes writes into these as uid 10000; open them up so this script can rewrite the config.
docker run --rm --entrypoint sh -v "$PWD":/w "${IMAGE:-nousresearch/hermes-agent:v2026.9.24}" -c 'chmod -R a+rwX /w/home-a /w/home-b /w/data /w/runs 2>/dev/null; true'
MODEL="${SPIKE_MODEL:-qwen/qwen3.8-27b:free}"
for shop in a b; do
  upper=$(echo "$shop" | tr a-z A-Z)
  eval token=\$TOKEN_$upper
  mkdir -p "home-$shop"
  cat > "home-$shop/config.yaml" <<YAML
model:
  provider: openrouter
  default: $MODEL
mcp_servers:
  aibcc:
    command: /opt/hermes/.venv/bin/python
    args: ["/spike/aibcc_mcp.py"]
    env:
      AIBCC_URL: "http://aibcc-mock:8099"
      AIBCC_TOKEN: "$token"
# Built-in toolsets are chosen per platform: every channel the shop can reach must be listed, or it gets the defaults.
platform_toolsets:
  cli: []
  api_server: []
  telegram: []
tools:
  tool_search:
    enabled: "${SPIKE_TOOL_SEARCH:-off}"
YAML
  cp SOUL.md "home-$shop/SOUL.md"
  chmod 666 "home-$shop/config.yaml" "home-$shop/SOUL.md"
done
echo "homes written for model $MODEL"
