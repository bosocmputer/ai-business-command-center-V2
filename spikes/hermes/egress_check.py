"""Run inside a Hermes container: what can the assistant reach?"""
import socket, urllib.request, urllib.error, os

def direct(host, port=443):
    try:
        socket.create_connection((host, port), timeout=5).close(); return "REACHED (bad)"
    except OSError as e: return "blocked (" + type(e).__name__ + ")"

def via_proxy(host):
    proxy = os.environ["HTTPS_PROXY"].split("//")[1]
    h, p = proxy.split(":")
    s = socket.create_connection((h, int(p)), timeout=8)
    s.sendall(f"CONNECT {host}:443 HTTP/1.1\r\nHost: {host}:443\r\n\r\n".encode())
    line = s.recv(200).split(b"\r\n")[0].decode(); s.close(); return line

print("direct 1.1.1.1:443 ->", direct("1.1.1.1"))
print("direct example.com:443 ->", direct("example.com"))
for host in ("example.com", "api.github.com", "openrouter.ai"):
    print("proxy CONNECT", host, "->", via_proxy(host))
print("aibcc-mock on the private network (should be reachable) ->", direct("aibcc-mock", 8099).replace("REACHED (bad)", "reachable, as intended"))
