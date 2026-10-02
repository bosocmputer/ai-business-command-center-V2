"""Ask one question of a running gateway (serve.sh) over the private network. Returns the answer and the timing.

The server host can reach a container's private address, so nothing is published. The key is read from
gateway.env on the host and never printed.
"""
import json
import os
import subprocess
import time
import uuid
import urllib.error
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))


def _key():
    for line in open(os.path.join(HERE, "gateway.env")):
        if line.startswith("API_SERVER_KEY="):
            return line.strip().split("=", 1)[1]
    raise RuntimeError("gateway.env has no API_SERVER_KEY")


def _address(shop):
    out = subprocess.run(["docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", f"hermes-{shop}"], capture_output=True, text=True, check=True)
    return out.stdout.strip()


def ask(shop, question, model, session=None, timeout=240):
    """session=None starts a fresh conversation. Hermes derives the session from the first message, so without an
    explicit id an identical first question resumes the old conversation and its old tool results."""
    body = json.dumps({"model": model, "messages": [{"role": "user", "content": question}], "stream": False}).encode()
    request = urllib.request.Request(f"http://{_address(shop)}:8642/v1/chat/completions", data=body,
                                     headers={"Authorization": f"Bearer {_key()}", "Content-Type": "application/json",
                                              "X-Hermes-Session-Id": session or "t-" + uuid.uuid4().hex[:12]})
    started = time.time()
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            data = json.loads(response.read())
        answer = data["choices"][0]["message"]["content"] or ""
        usage = data.get("usage") or {}
    except urllib.error.HTTPError as error:
        answer, usage = "", {"error": f"HTTP {error.code}"}
    except Exception as error:  # timeouts and resets are results too
        answer, usage = "", {"error": type(error).__name__}
    return {"answer": answer, "seconds": round(time.time() - started, 1), "usage": usage}


if __name__ == "__main__":
    import sys
    result = ask(sys.argv[1], sys.argv[2], sys.argv[3] if len(sys.argv) > 3 else "hermes")
    print(json.dumps(result, ensure_ascii=False, indent=1))
