#!/bin/sh
# usage (on the server, from backend/deploy):  ssh -t <server> 'cd <deploy dir>/backend/deploy && ./assistant/set-secret.sh OPENROUTER_API_KEY'
# Asks for one value without echoing it and stores it in secrets/assistant/hermes.env (mode 0600), replacing any
# earlier value of the same name. Names: OPENROUTER_API_KEY, AIBCC_TOKEN, WEB_SEARCH_API_KEY (Brave Search or Tavily, chosen by WEB_SEARCH_PROVIDER in .env.production), API_SERVER_KEY (any 16+ characters; or
# type "generate" to make a random one), TELEGRAM_BOT_TOKEN (from @BotFather), TELEGRAM_ALLOWED_USERS (numeric Telegram
# user ids, comma separated; the people allowed to message the bot). The value never appears in a command line, in shell history or in chat.
# Add one more person without retyping the others:  ./assistant/set-secret.sh TELEGRAM_ALLOWED_USERS --add
# (the new id is merged into the existing list; duplicates are dropped).
set -eu
NAME="${1:?name required: OPENROUTER_API_KEY | AIBCC_TOKEN | API_SERVER_KEY | TELEGRAM_BOT_TOKEN | TELEGRAM_ALLOWED_USERS | WEB_SEARCH_API_KEY}"
case "$NAME" in OPENROUTER_API_KEY|AIBCC_TOKEN|API_SERVER_KEY|TELEGRAM_BOT_TOKEN|TELEGRAM_ALLOWED_USERS|WEB_SEARCH_API_KEY) ;; *) echo "unknown name: $NAME" >&2; exit 2;; esac
MODE="${2:-}"
case "$MODE" in ""|--add) ;; *) echo "unknown option: $MODE" >&2; exit 2;; esac
[ "$MODE" != "--add" ] || [ "$NAME" = "TELEGRAM_ALLOWED_USERS" ] || { echo "--add works for TELEGRAM_ALLOWED_USERS only" >&2; exit 2; }
cd "$(dirname "$0")/.."
umask 077
mkdir -p secrets/assistant
FILE=secrets/assistant/hermes.env
touch "$FILE"; chmod 600 "$FILE"
printf '%s: ' "$NAME"; stty -echo 2>/dev/null || true; read -r VALUE; stty echo 2>/dev/null || true; echo
if [ "$VALUE" = "generate" ] && [ "$NAME" = "API_SERVER_KEY" ]; then VALUE=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n'); fi
[ -n "$VALUE" ] || { echo "empty value, nothing saved" >&2; exit 1; }
case "$VALUE" in *[!A-Za-z0-9._~+/=:,-]*) echo "value has characters that do not belong in a key; nothing saved" >&2; exit 1;; esac
if [ "$MODE" = "--add" ]; then
  OLD=$(grep "^$NAME=" "$FILE" | cut -d= -f2- || true)
  VALUE=$(printf '%s,%s' "$OLD" "$VALUE" | tr ',' '\n' | grep -v '^$' | awk '!seen[$0]++' | paste -sd, -)
fi
if [ "$NAME" = "TELEGRAM_ALLOWED_USERS" ]; then
  case "$VALUE" in *[!0-9,]*) echo "Telegram ids are digits only (comma separated); nothing saved" >&2; exit 1;; esac
fi
TMP=$(mktemp "$FILE.XXXXXX")
grep -v "^$NAME=" "$FILE" > "$TMP" || true
printf '%s=%s\n' "$NAME" "$VALUE" >> "$TMP"
mv "$TMP" "$FILE"; chmod 600 "$FILE"
if [ "$NAME" = "TELEGRAM_ALLOWED_USERS" ]; then
  echo "saved $NAME: $(printf '%s' "$VALUE" | tr ',' '\n' | grep -c .) ids in the list"
else
  echo "saved $NAME ($(grep -c '=' "$FILE") values in the file; names: $(cut -d= -f1 "$FILE" | tr '\n' ' '))"
fi
