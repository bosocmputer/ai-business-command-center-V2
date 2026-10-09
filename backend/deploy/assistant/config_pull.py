"""The shop's assistant takes its settings from AI-BCC instead of from files on the server. Standard library only.

  config_pull.py start     at container start: fetch the settings, keep a copy, write /opt/data/config.yaml, print shell lines
                           (export NAME='value' / unset NAME) for the entrypoint to eval, tell AI-BCC which version is now used.
                           Exit 3 when there is neither an answer nor an earlier copy: the entrypoint then keeps the old way.
  config_pull.py changed   exit 0 when AI-BCC has newer settings than the ones in use, 1 when not (or when it cannot be reached).
  config_pull.py check     compare what would be used with what is in use now, by hash, never printing a value.

AI-BCC knows which shop this is from the token in AIBCC_TOKEN. A shop that is switched off, past its end date or has no key gets
enabled=false and nothing else: the secrets are then unset, so no chat platform starts and nobody can reach the assistant.
Nothing printed to stderr or stdout except the shell lines of `start` ever contains a secret.
"""
import hashlib
import json
import os
import re
import shlex
import sys
import time
import urllib.error
import urllib.request

DATA = os.environ.get("ASSISTANT_DATA_DIR", "/opt/data")
TEMPLATE = os.environ.get("ASSISTANT_TEMPLATE", "/assistant/config.yaml")
BASE = os.environ.get("AIBCC_URL", "http://api:8080").rstrip("/") + "/api/v1/agent"
SECRET_ENV = {
    "openrouterKey": "OPENROUTER_API_KEY",
    "telegramBotToken": "TELEGRAM_BOT_TOKEN",
    "lineChannelSecret": "LINE_CHANNEL_SECRET",
    "lineChannelAccessToken": "LINE_CHANNEL_ACCESS_TOKEN",
}
EXIT_NO_CONFIG = 3


def log(**entry):
    print(json.dumps(entry, ensure_ascii=False), file=sys.stderr, flush=True)


def paths():
    return os.path.join(DATA, "assistant-config.json"), os.path.join(DATA, "config.yaml")


def fetch(etag=None, timeout=10):
    """(status, etag, config or None). 304 gives (304, etag, None). Any failure gives (0, None, None)."""
    request = urllib.request.Request(BASE + "/assistant-config", headers={"Authorization": "Bearer " + os.environ.get("AIBCC_TOKEN", "")})
    if etag:
        request.add_header("If-None-Match", etag)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return response.status, response.headers.get("ETag"), json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as error:
        return (304, etag, None) if error.code == 304 else (error.code, None, None)
    except (urllib.error.URLError, TimeoutError, ValueError, OSError):
        return 0, None, None


def load_cache():
    try:
        with open(paths()[0], encoding="utf-8") as handle:
            data = json.load(handle)
        return data["etag"], data["config"]
    except (OSError, ValueError, KeyError):
        return None, None


def save_cache(etag, config):
    path = paths()[0]
    os.makedirs(DATA, exist_ok=True)
    temporary = path + ".tmp"
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as handle:
        json.dump({"etag": etag, "config": config}, handle)
    os.replace(temporary, path)


def render_yaml(template, config):
    """The repository's config.yaml with the model and the pinned providers of this shop put in."""
    model = config.get("model")
    if not config.get("enabled") or not model:
        return template  # nothing runs for this shop, but the file must still be valid
    text, changed = re.subn(r"(?m)^(  default: ).*$", lambda match: match.group(1) + model["modelId"], template, count=1)
    if changed != 1:
        raise ValueError("template has no model line")
    providers = model.get("providers") or []
    pin = "  only: [" + ", ".join(providers) + "]" if providers else "  # no provider pin (a test-only model)"
    text, changed = re.subn(r"(?m)^  only: \[.*\]$", lambda match: pin, text, count=1)
    if changed != 1:
        raise ValueError("template has no provider line")
    if not model.get("dataCollectionDeny", True):
        text = text.replace("data_collection: deny", "data_collection: allow", 1)
    return text


def shell_lines(config):
    secrets = config.get("secrets") or {}
    lines = []
    for key, name in SECRET_ENV.items():
        value = secrets.get(key) if config.get("enabled") else None
        lines.append(f"export {name}={shlex.quote(value)}" if value else f"unset {name}")
    return lines


def write_yaml(text):
    path = paths()[1]
    os.makedirs(DATA, exist_ok=True)
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(text)


def post_status(config):
    body = json.dumps({"configVersion": int(config.get("configVersion") or 0), "modelKey": (config.get("model") or {}).get("key", ""),
                       "startedAt": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}).encode()
    request = urllib.request.Request(BASE + "/assistant-status", data=body, method="POST",
                                     headers={"Authorization": "Bearer " + os.environ.get("AIBCC_TOKEN", ""), "Content-Type": "application/json"})
    try:
        urllib.request.urlopen(request, timeout=10).read()
    except (urllib.error.URLError, TimeoutError, OSError):
        log(config="status not reported")


def start():
    etag_old, cached = load_cache()
    status, etag, config = fetch()
    source = "AI-BCC"
    if status == 200 and config is not None:
        save_cache(etag, config)
    elif cached is not None:
        config, etag, source = cached, etag_old, "earlier copy"
        log(config="AI-BCC not reachable or refused; using the earlier copy", status=status)
    else:
        log(config="no settings from AI-BCC and no earlier copy", status=status)
        return EXIT_NO_CONFIG
    try:
        with open(TEMPLATE, encoding="utf-8") as handle:
            write_yaml(render_yaml(handle.read(), config))
    except (OSError, ValueError) as error:
        log(config="could not write the model settings", error=type(error).__name__)
        return EXIT_NO_CONFIG
    print("\n".join(shell_lines(config)))
    log(config="applied", source=source, enabled=bool(config.get("enabled")), reason=config.get("reason", ""), version=config.get("configVersion"),
        model=(config.get("model") or {}).get("key", ""))
    if source == "AI-BCC":
        post_status(config)
    return 0


def changed():
    etag_old, _ = load_cache()
    if not etag_old:
        return 1
    status, etag, config = fetch(etag_old)
    return 0 if status == 200 and config is not None and etag != etag_old else 1


def digest(value):
    return hashlib.sha256((value or "").encode()).hexdigest()[:10]


def check():
    status, _, config = fetch()
    if status != 200 or config is None:
        print(json.dumps({"result": "no answer from AI-BCC", "status": status}))
        return 1
    report = {"enabled": bool(config.get("enabled")), "reason": config.get("reason", ""), "variables": {}}
    secrets = config.get("secrets") or {}
    for key, name in SECRET_ENV.items():
        wanted, current = secrets.get(key) if config.get("enabled") else None, os.environ.get(name)
        report["variables"][name] = "same" if digest(wanted) == digest(current) and (wanted or not current) else ("missing here" if wanted and not current else "different")
    model = config.get("model") or {}
    try:
        with open(paths()[1], encoding="utf-8") as handle:
            in_use = re.search(r"(?m)^  default: (.*)$", handle.read())
        report["model"] = "same" if in_use and in_use.group(1).strip() == model.get("modelId") else "different"
    except OSError:
        report["model"] = "no file"
    print(json.dumps(report, ensure_ascii=False))
    return 0 if all(value == "same" for value in report["variables"].values()) and report["model"] == "same" else 1


if __name__ == "__main__":
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    sys.exit({"start": start, "changed": changed, "check": check}.get(command, lambda: 2)())
