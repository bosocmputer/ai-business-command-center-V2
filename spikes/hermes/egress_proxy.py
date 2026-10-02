"""A small allow-list proxy for HTTPS: the assistant may reach only the hosts named in EGRESS_ALLOW.

It speaks CONNECT only (every allowed destination is HTTPS), refuses everything else, and logs the host and the
decision for each request - never a payload. It is a test stand-in; a production deployment would use a maintained
proxy (squid or Hermes' own `hermes egress`) with the same allow-list.
"""
import asyncio
import json
import os
import sys
import time

ALLOW = {h.strip().lower() for h in os.environ.get("EGRESS_ALLOW", "").split(",") if h.strip()}
LOG = os.environ.get("EGRESS_LOG", "/data/egress.jsonl")


def log(**entry):
    entry["t"] = int(time.time())
    try:
        with open(LOG, "a", encoding="utf-8") as handle:
            handle.write(json.dumps(entry) + "\n")
    except OSError as error:  # a silent audit log is worse than a loud one
        print("egress log not written:", type(error).__name__, file=sys.stderr, flush=True)


async def pipe(reader, writer):
    try:
        while data := await reader.read(65536):
            writer.write(data)
            await writer.drain()
    except (ConnectionError, asyncio.CancelledError):
        pass
    finally:
        writer.close()


async def handle(reader, writer):
    try:
        head = await asyncio.wait_for(reader.readuntil(b"\r\n\r\n"), 10)
        method, target, _ = head.split(b"\r\n", 1)[0].decode("latin-1").split(" ", 2)
        host, _, port = target.rpartition(":") if method == "CONNECT" else (target, "", "")
        if method != "CONNECT" or host.lower() not in ALLOW or port != "443":
            log(host=host or target[:80], decision="DENY")
            writer.write(b"HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n")
            await writer.drain()
            writer.close()
            return
        upstream_reader, upstream_writer = await asyncio.wait_for(asyncio.open_connection(host, 443), 10)
        log(host=host, decision="ALLOW")
        writer.write(b"HTTP/1.1 200 Connection Established\r\n\r\n")
        await writer.drain()
        await asyncio.gather(pipe(reader, upstream_writer), pipe(upstream_reader, writer))
    except Exception:
        writer.close()


async def main():
    server = await asyncio.start_server(handle, "0.0.0.0", int(sys.argv[1]) if len(sys.argv) > 1 else 3128)
    async with server:
        await server.serve_forever()


asyncio.run(main())
