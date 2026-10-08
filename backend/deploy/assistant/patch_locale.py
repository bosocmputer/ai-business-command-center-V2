"""Make a private copy of Hermes' English message catalog with two changes, for the owner-facing /new reply:

  gateway.reset.header_default  "Session reset! Starting fresh."  ->  a Thai line
  gateway.reset.tip             a random English tip ("Over 80 bundled skills ...", false for us) -> a single space

Hermes reads its messages from the directory named by HERMES_BUNDLED_LOCALES, so nothing in the image is touched. The copy
goes to the directory given as the first argument. If either line is not found exactly once (a different Hermes version)
nothing is written and the exit status is 1, and entrypoint.sh leaves the default messages in place.
"""
import sys

SOURCE = "/opt/hermes/locales/en.yaml"
HEADER_OLD = '    header_default:        "✨ Session reset! Starting fresh."'
HEADER_NEW = '    header_default:        "✨ เริ่มเรื่องใหม่แล้วครับ ถามได้เลย"'
TIP_OLD = '    tip:                   "\\n✦ Tip: {tip}"'
TIP_NEW = '    tip:                   " "'

try:
    text = open(SOURCE, encoding="utf-8").read()
except OSError:
    sys.exit(1)
if text.count(HEADER_OLD) != 1 or text.count(TIP_OLD) != 1:
    sys.exit(1)
text = text.replace(HEADER_OLD, HEADER_NEW).replace(TIP_OLD, TIP_NEW)
import os
os.makedirs(sys.argv[1], exist_ok=True)
with open(os.path.join(sys.argv[1], "en.yaml"), "w", encoding="utf-8") as handle:
    handle.write(text)
