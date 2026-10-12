#!/bin/sh
# usage (on the server, from backend/deploy):  ./assistant/add-shop.sh shop2
# Prepares one more shop's assistant next to the first: writes assistant/compose.assistant-<name>.yml from compose.shop.template.yml (its own
# network, egress proxy, volume and secrets file), creates secrets/assistant/hermes-<name>.env (mode 0600) with a random API_SERVER_KEY,
# and prints what is left to do. Running it again rewrites the compose file (use that after the image in compose.assistant.yml changes) and
# keeps the secrets file as it is. It starts nothing and prints no secret.
set -eu
INST="${1:?name required, for example shop2 (lower-case letters, digits and dashes)}"
case "$INST" in *[!a-z0-9-]*|-*|*-|"") echo "name: lower-case letters, digits and dashes only, not starting or ending with a dash" >&2; exit 2;; esac
[ "${#INST}" -le 24 ] || { echo "name: at most 24 characters" >&2; exit 2; }
cd "$(dirname "$0")/.."
IMAGE=$(sed -n 's/^x-assistant-image: &assistant-image //p' assistant/compose.assistant.yml)
[ -n "$IMAGE" ] || { echo "cannot read the image line of assistant/compose.assistant.yml" >&2; exit 1; }
OUT="assistant/compose.assistant-$INST.yml"
sed -e "s|__IMAGE__|$IMAGE|g" -e "s|__INST__|$INST|g" assistant/compose.shop.template.yml > "$OUT"
umask 077
mkdir -p secrets/assistant
FILE="secrets/assistant/hermes-$INST.env"
touch "$FILE"; chmod 600 "$FILE"
if ! grep -q '^API_SERVER_KEY=' "$FILE"; then
  printf 'API_SERVER_KEY=%s\n' "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')" >> "$FILE"
fi
echo "written: $OUT and $FILE (secret names in the file: $(cut -d= -f1 "$FILE" | tr '\n' ' '))"
echo "assistant host to set in the admin page: assistant-$INST"
echo "from now on start the stack with: COMPOSE_FILE=\$PWD/compose.local.yml:\$PWD/assistant/compose.assistant.yml:\$PWD/$OUT"
echo "still to do: issue this shop's assistant token in the admin page, then: ASSISTANT_INSTANCE=$INST ./assistant/set-secret.sh AIBCC_TOKEN"
