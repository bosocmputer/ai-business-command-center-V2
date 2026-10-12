#!/bin/sh
# Run once on the server, from backend/deploy:  ./provision-install.sh
# Adds a cron line for the deploy user that runs provision.sh every minute (at most one run at a time), so assistants asked for in the admin
# page get created without anyone logging in. Safe to run again. To remove it: crontab -e and delete the line that mentions provision.sh.
set -eu
cd "$(dirname "$0")"
command -v flock >/dev/null 2>&1 || { echo "flock is needed (util-linux)" >&2; exit 1; }
chmod +x provision.sh assistant/add-shop.sh
LINE="* * * * * cd $PWD && flock -n /tmp/aibcc-provision.lock ./provision.sh >> $HOME/aibcc-provision.log 2>&1"
CURRENT=$(crontab -l 2>/dev/null || true)
if printf '%s\n' "$CURRENT" | grep -qF "./provision.sh"; then
  echo "already installed"
else
  { printf '%s\n' "$CURRENT" | grep -v '^$' || true; printf '%s\n' "$LINE"; } | crontab -
  echo "installed: $LINE"
fi
