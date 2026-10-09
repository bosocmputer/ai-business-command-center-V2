"""The one door LINE may knock on. LINE sends the owner's messages to a public HTTPS address (a tunnel); that address ends here,
not at the assistant, so the assistant itself stays on the internal network with no route in from outside.

It forwards exactly one thing, `POST /line/webhook`, to the assistant (INGRESS_UPSTREAM, default http://assistant:8646), with only the
headers the assistant needs (Content-Type and X-Line-Signature, which the assistant verifies against the channel secret). Any other
method or path answers 404, a body over 1 MiB 413, and more than INGRESS_PER_MINUTE requests a minute 429. `GET /healthz` answers ok.
One JSON line per request goes to stdout (path and decision, never a body), so `docker logs` is the audit trail.
"""
import asyncio
import json
import os
import sys
import time
from urllib.parse import urlsplit

UPSTREAM = urlsplit(os.environ.get("INGRESS_UPSTREAM", "http://assistant:8646"))
PATH = "/line/webhook"
MAX_BODY = 1_048_576
PER_MINUTE = int(os.environ.get("INGRESS_PER_MINUTE", "120"))
FORWARD_HEADERS = ("content-type", "x-line-signature")
RESPONSES = {200: "OK", 204: "No Content", 400: "Bad Request", 401: "Unauthorized", 403: "Forbidden", 404: "Not Found",
             413: "Payload Too Large", 429: "Too Many Requests", 500: "Internal Server Error", 502: "Bad Gateway", 504: "Gateway Timeout"}
recent = []


def log(**entry):
    entry["t"] = int(time.time())
    print(json.dumps(entry), flush=True)


def too_many(now=None):
    now = time.time() if now is None else now
    while recent and recent[0] < now - 60:
        recent.pop(0)
    if len(recent) >= PER_MINUTE:
        return True
    recent.append(now)
    return False


async def reply(writer, status, body=b"", content_type="text/plain"):
    head = f"HTTP/1.1 {status} {RESPONSES.get(status, 'Status')}\r\nContent-Type: {content_type}\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n"
    writer.write(head.encode("latin-1") + body)
    await writer.drain()


async def forward(body, headers):
    reader, writer = await asyncio.wait_for(asyncio.open_connection(UPSTREAM.hostname, UPSTREAM.port or 80), 5)
    try:
        lines = [f"POST {PATH} HTTP/1.1", f"Host: {UPSTREAM.hostname}:{UPSTREAM.port or 80}", f"Content-Length: {len(body)}", "Connection: close"]
        lines += [f"{name}: {value}" for name, value in headers.items() if name in FORWARD_HEADERS]
        writer.write(("\r\n".join(lines) + "\r\n\r\n").encode("latin-1") + body)
        await writer.drain()
        raw = await asyncio.wait_for(reader.read(), 30)
    finally:
        writer.close()
    head, _, payload = raw.partition(b"\r\n\r\n")
    status = int(head.split(b" ", 2)[1]) if head.startswith(b"HTTP/") else 502
    return status, payload


async def handle(reader, writer):
    status, what = 500, "error"
    try:
        head = await asyncio.wait_for(reader.readuntil(b"\r\n\r\n"), 10)
        first, *header_lines = head.decode("latin-1").split("\r\n")
        method, target, _ = first.split(" ", 2)
        headers = {}
        for line in header_lines:
            name, _, value = line.partition(":")
            if name:
                headers[name.strip().lower()] = value.strip()
        path = target.split("?", 1)[0]
        if method == "GET" and path == "/healthz":
            status, what = 200, "healthz"
            await reply(writer, 200, b"ok")
        elif method != "POST" or path != PATH:
            status, what = 404, "refused"
            await reply(writer, 404)
        elif too_many():
            status, what = 429, "rate"
            await reply(writer, 429)
        else:
            length = int(headers.get("content-length", "-1"))
            if length < 0 or length > MAX_BODY:
                status, what = 413, "size"
                await reply(writer, 413)
            else:
                body = await asyncio.wait_for(reader.readexactly(length), 10)
                try:
                    status, payload = await forward(body, headers)
                    what = "forwarded"
                except (OSError, asyncio.TimeoutError):
                    status, payload = 502, b""
                    what = "upstream down"
                await reply(writer, status, payload, "application/json")
    except (asyncio.IncompleteReadError, asyncio.TimeoutError, ValueError, ConnectionError):
        status, what = 400, "bad request"
        try:
            await reply(writer, 400)
        except ConnectionError:
            pass
    finally:
        log(decision=what, status=status)
        writer.close()


async def main(port):
    server = await asyncio.start_server(handle, "0.0.0.0", port)
    async with server:
        await server.serve_forever()


if __name__ == "__main__":
    asyncio.run(main(int(sys.argv[1]) if len(sys.argv) > 1 else 8646))
