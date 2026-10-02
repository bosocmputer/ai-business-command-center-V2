#!/bin/sh
# usage (on the server, from backend/deploy):  ssh -t <server> 'cd <deploy dir>/backend/deploy && ./assistant/set-secret.sh OPENROUTER_API_KEY'
# Asks for one value without echoing it and stores it in secrets/assistant/hermes.env (mode 0600), replacing any
# earlier value of the same name. Names: OPENROUTER_API_KEY, AIBCC_TOKEN, API_SERVER_KEY (any 16+ characters; or
# type "generate" to make a random one). The value never appears in a command line, in shell history or in chat.
set -eu
NAME="${1:?name required: OPENROUTER_API_KEY | AIBCC_TOKEN | API_SERVER_KEY}"
case "$NAME" in OPENROUTER_API_KEY|AIBCC_TOKEN|API_SERVER_KEY) ;; *) echo "unknown name: $NAME" >&2; exit 2;; esac
cd "$(dirname "$0")/.."
umask 077
mkdir -p secrets/assistant
FILE=secrets/assistant/hermes.env
touch "$FILE"; chmod 600 "$FILE"
printf '%s: ' "$NAME"; stty -echo; read -r VALUE; stty echo; echo
if [ "$VALUE" = "generate" ] && [ "$NAME" = "API_SERVER_KEY" ]; then VALUE=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'); fi
[ -n "$VALUE" ] || { echo "empty value, nothing saved" >&2; exit 1; }
case "$VALUE" in *[!A-Za-z0-9._~+/=-]*) echo "value has characters that do not belong in a key; nothing saved" >&2; exit 1;; esac
TMP=$(mktemp "$FILE.XXXXXX")
grep -v "^$NAME=" "$FILE" > "$TMP" || true
printf '%s=%s\n' "$NAME" "$VALUE" >> "$TMP"
mv "$TMP" "$FILE"; chmod 600 "$FILE"
echo "saved $NAME ($(grep -c '=' "$FILE") of 3 values present)"
