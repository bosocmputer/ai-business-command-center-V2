"""Keep the assistant's memory to what SOUL.md allows: the owner's preferences and vocabulary, never business facts.

Run by maintain.sh every day with the gateway stopped (nothing is writing). A memory entry (one line) is removed when
it holds a number of three or more digits, a decimal, a link, a customer code, a bank-account style number, or the
words of an instruction aimed at an assistant. Why: figures change all day and must be fetched, never recalled; and
text planted in report data (a product name someone typed in SML, say) must not become a standing instruction.

  memory_guard.py            report only (prints counts, never the text)
  memory_guard.py --apply    remove the flagged entries
Also lists skills the agent wrote for itself (names only), since self-written skills are off and none are expected.
"""
import json
import os
import re
import sys

MEMORY_DIR = "/opt/data/memories"
SKILLS_DIR = "/opt/data/skills"
BUNDLED_SKILLS_DIR = "/opt/hermes/skills"
APPLY = "--apply" in sys.argv

FLAGS = [
    re.compile(r"\d{3,}"),                                   # a figure, a date, an account number
    re.compile(r"\d[\d,]*\.\d"),                             # a decimal
    re.compile(r"https?://|www\.", re.I),                    # a link
    re.compile(r"ลูกค้า-[0-9A-Fa-f]{4}"),                    # a customer code
    re.compile(r"ละเว้น|ลืมคำสั่ง|คำสั่งถึง|โอนเงิน|โอนไป|เลขบัญชี|ignore (all|previous)|system prompt|api[_ ]?key|token", re.I),
]


def flagged(line):
    return any(pattern.search(line) for pattern in FLAGS)


files = entries = removed = 0
if os.path.isdir(MEMORY_DIR):
    for name in sorted(os.listdir(MEMORY_DIR)):
        path = os.path.join(MEMORY_DIR, name)
        if not name.endswith(".md") or not os.path.isfile(path):
            continue
        files += 1
        with open(path, encoding="utf-8") as handle:
            lines = handle.read().split("\n")
        keep = []
        for line in lines:
            if line.strip():
                entries += 1
            if line.strip() and flagged(line):
                removed += 1
                continue
            keep.append(line)
        if APPLY and len(keep) != len(lines):
            with open(path, "w", encoding="utf-8") as handle:
                handle.write("\n".join(keep))

own_skills = []
if os.path.isdir(SKILLS_DIR) and os.path.isdir(BUNDLED_SKILLS_DIR):
    bundled = set(os.listdir(BUNDLED_SKILLS_DIR))
    own_skills = sorted(n for n in os.listdir(SKILLS_DIR) if not n.startswith(".") and n not in bundled)

print(json.dumps({"memory_guard": "applied" if APPLY else "report-only", "files": files, "entries": entries,
                  "flagged": removed, "own_skills": own_skills}, ensure_ascii=False))
