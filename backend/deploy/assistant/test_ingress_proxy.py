import asyncio
import unittest

import ingress_proxy as proxy


async def http(port, request):
    reader, writer = await asyncio.open_connection("127.0.0.1", port)
    writer.write(request)
    await writer.drain()
    raw = await reader.read()
    writer.close()
    head, _, body = raw.partition(b"\r\n\r\n")
    return int(head.split(b" ")[1]), body


def post(body, path="/line/webhook", extra=""):
    return (f"POST {path} HTTP/1.1\r\nHost: x\r\nContent-Length: {len(body)}\r\nContent-Type: application/json\r\nX-Line-Signature: sig\r\n"
            f"Cookie: secret\r\n{extra}\r\n").encode() + body


class IngressTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.seen = []

        async def upstream(reader, writer):
            head = await reader.readuntil(b"\r\n\r\n")
            length = int([line for line in head.decode().split("\r\n") if line.lower().startswith("content-length")][0].split(":")[1])
            body = await reader.readexactly(length)
            self.seen.append((head.decode(), body))
            writer.write(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
            await writer.drain()
            writer.close()

        self.upstream = await asyncio.start_server(upstream, "127.0.0.1", 0)
        proxy.UPSTREAM = proxy.urlsplit(f"http://127.0.0.1:{self.upstream.sockets[0].getsockname()[1]}")
        self.server = await asyncio.start_server(proxy.handle, "127.0.0.1", 0)
        self.port = self.server.sockets[0].getsockname()[1]
        proxy.recent.clear()
        proxy.PER_MINUTE = 120

    async def asyncTearDown(self):
        self.server.close()
        self.upstream.close()

    async def test_the_webhook_is_forwarded_with_only_the_headers_it_needs(self):
        status, body = await http(self.port, post(b'{"events":[]}'))
        self.assertEqual((status, body), (200, b"ok"))
        head, payload = self.seen[0]
        self.assertEqual(payload, b'{"events":[]}')
        self.assertIn("x-line-signature: sig", head.lower())  # header names are case-insensitive
        self.assertNotIn("cookie", head.lower())

    async def test_everything_else_is_refused_and_never_reaches_the_assistant(self):
        for request in (post(b"{}", "/admin"), post(b"{}", "/line/media/x"), b"GET /line/webhook HTTP/1.1\r\nHost: x\r\n\r\n", b"DELETE /line/webhook HTTP/1.1\r\nHost: x\r\n\r\n"):
            status, _ = await http(self.port, request)
            self.assertEqual(status, 404)
        self.assertEqual(self.seen, [])

    async def test_a_big_body_and_a_flood_are_stopped(self):
        status, _ = await http(self.port, f"POST /line/webhook HTTP/1.1\r\nHost: x\r\nContent-Length: {proxy.MAX_BODY + 1}\r\n\r\n".encode())
        self.assertEqual(status, 413)
        proxy.PER_MINUTE = 2
        proxy.recent.clear()
        codes = [(await http(self.port, post(b"{}")))[0] for _ in range(3)]
        self.assertEqual(codes, [200, 200, 429])

    async def test_a_dead_assistant_is_a_gateway_error_and_healthz_answers(self):
        self.upstream.close()
        await self.upstream.wait_closed()
        status, _ = await http(self.port, post(b"{}"))
        self.assertEqual(status, 502)
        status, body = await http(self.port, b"GET /healthz HTTP/1.1\r\nHost: x\r\n\r\n")
        self.assertEqual((status, body), (200, b"ok"))


if __name__ == "__main__":
    unittest.main()
