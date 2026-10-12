# Sourced by the scripts that start or stop containers (from backend/deploy): sets COMPOSE_FILE to the stack, the first assistant and every
# extra shop's assistant that add-shop.sh generated. Starting the stack without one of them would take its network off `api`.
COMPOSE_STACK="$PWD/compose.local.yml"
[ -f assistant/compose.assistant.yml ] && COMPOSE_STACK="$COMPOSE_STACK:$PWD/assistant/compose.assistant.yml"
for overlay in assistant/compose.assistant-*.yml; do
  [ -f "$overlay" ] && COMPOSE_STACK="$COMPOSE_STACK:$PWD/$overlay"
done
export COMPOSE_FILE="$COMPOSE_STACK"
