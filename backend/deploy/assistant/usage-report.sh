#!/bin/sh
# What the trial week looks like, from both sides, with no conversation text and no figures of the shop:
#   assistant side: sessions, questions, tokens, cost, how long answers take, answers that said "no data" (per day)
#   AI-BCC side:    every tool call the assistant made: which report, how it ended, how long the API took
#   backend/deploy$ ./assistant/usage-report.sh [days]        (default 7)
set -eu
cd "$(dirname "$0")/.."
export COMPOSE_FILE="${COMPOSE_FILE:-compose.local.yml:assistant/compose.assistant.yml}"
ENV_FILE="${ENV_FILE:-.env.production}"
DAYS="${1:-7}"
case "$DAYS" in ''|*[!0-9]*) echo "days must be a whole number" >&2; exit 2;; esac
dc() { docker compose --env-file "$ENV_FILE" "$@"; }
echo "== ฝั่งผู้ช่วย (Hermes) =="
dc exec -T assistant /opt/hermes/.venv/bin/python /assistant/usage_report.py --days "$DAYS" --openrouter
echo
echo "== ฝั่ง AI-BCC: การเรียกเครื่องมือของผู้ช่วย ($DAYS วันล่าสุด) =="
dc exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -P pager=off' <<SQL
select to_char(created_at at time zone 'Asia/Bangkok', 'YYYY-MM-DD') as day,
       tool, coalesce(report_key, '-') as report, outcome, count(*) as calls, round(avg(duration_ms)) as avg_ms
from agent_calls
where created_at > now() - interval '$DAYS days'
group by 1, 2, 3, 4
order by 1 desc, 5 desc
limit 60;
SQL
echo "ผลลัพธ์: OK ตอบได้ · PREPARING กำลังดึงข้อมูลสด · NO_DATA ไม่มีรายงานนี้หรือไม่มีสิทธิ์ · RATE_LIMITED ถามถี่เกิน"
