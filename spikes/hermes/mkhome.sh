#!/bin/sh
# Write the Hermes home for each shop: model, the one MCP server it may use, and the persona.
set -eu
cd "$(dirname "$0")"
umask 077
set -a; . ./tokens.env; set +a
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
platform_toolsets:
  cli: []
tools:
  tool_search:
    enabled: "${SPIKE_TOOL_SEARCH:-off}"
YAML
  cp SOUL.md "home-$shop/SOUL.md"
  chmod 666 "home-$shop/config.yaml" "home-$shop/SOUL.md"
done
echo "homes written for model $MODEL"
