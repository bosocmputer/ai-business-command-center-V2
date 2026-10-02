#!/usr/bin/env python3
"""usage: provider_check.py <model> <provider-slug>   (run where OPENROUTER_API_KEY is in the environment)

One tiny request with the same routing rules the assistant uses, plus zdr=true, to see which provider answers.
Prints the provider name and whether the request was accepted; never prints the key.
"""
import json, os, sys, urllib.request, urllib.error
model, provider = sys.argv[1], sys.argv[2]
body = {"model": model, "max_tokens": 16, "messages": [{"role": "user", "content": "ตอบคำเดียวว่า โอเค"}],
        "provider": {"only": [provider], "data_collection": "deny", "zdr": True, "allow_fallbacks": False}}
req = urllib.request.Request("https://openrouter.ai/api/v1/chat/completions", data=json.dumps(body).encode(),
                             headers={"Authorization": "Bearer " + os.environ["OPENROUTER_API_KEY"], "Content-Type": "application/json"})
try:
    d = json.load(urllib.request.urlopen(req, timeout=60))
    print("OK provider=%s model=%s" % (d.get("provider"), d.get("model")))
except urllib.error.HTTPError as e:
    print("REFUSED", e.code, e.read().decode()[:300])
