#!/bin/sh
# usage (from backend/deploy):  ./health-report.sh <tenant-uuid> "<shop name>" > health.html
# One-file HTML report an owner can read: what is wrong with the shop's own data, why it matters and what to do. It reads the shop
# through onboard-check.sh (read only, counts and aggregates, no names) and prints nothing else on stdout.
set -eu
TENANT="${1:?tenant uuid required}"
SHOP="${2:-}"
cd "$(dirname "$0")"
./onboard-check.sh "$TENANT" json 2>/dev/null | python3 health_report.py "$SHOP"
