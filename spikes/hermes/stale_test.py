#!/usr/bin/env python3
"""Does a long conversation reuse old numbers? One session; the receivables total is asked, then raised by 1,000 at
the mock, then asked again in the same session. The second answer must carry the new figure.

usage: stale_test.py <model>      (run on the server after serve.sh; restarts the mock first)
"""
import json
import os
import subprocess
import sys
import urllib.request

import ask_gw
import mock_api as m

HERE = os.path.dirname(os.path.abspath(__file__))
for line in open(os.path.join(HERE, "tokens.env")):
    if "=" in line:
        k, v = line.strip().split("=", 1)
        os.environ[k] = v

model = sys.argv[1]
subprocess.run(["docker", "restart", "aibcc-mock"], check=True, capture_output=True)
ip = subprocess.run(["docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", "aibcc-mock"], capture_output=True, text=True).stdout.split()[0]  # the mock sits on two networks: any one address works
base = sum(v for _, v in m.AGING_BUCKETS)
session = "stale-" + os.urandom(4).hex()
results = []
for round_number, question in enumerate(["ตอนนี้ลูกหนี้ค้างรวมเท่าไหร่", "ขอถามอีกที ตอนนี้ลูกหนี้ค้างรวมเท่าไหร่", "แล้วตอนนี้ล่ะ ยอดค้างรวมเท่าไหร่"]):
    expected = m.money(base + 1000.0 * round_number)
    got = ask_gw.ask("a", question, model, session=session)
    ok = expected.rstrip("0").rstrip(".") in got["answer"].replace(",", "") or expected in got["answer"].replace(",", "")
    results.append(ok)
    print(f"ask {round_number + 1}: expected {expected}  {'PASS' if ok else 'FAIL (old or wrong figure)'}  {got['seconds']}s")
    print("   ", got["answer"][:160].replace("\n", " "))
    req = urllib.request.Request(f"http://{ip}:8099/_bump", headers={"Authorization": "Bearer " + os.environ["TOKEN_A"]})
    urllib.request.urlopen(req).read()
print("SUMMARY", json.dumps({"fresh_figures": f"{sum(results)}/{len(results)}"}))
