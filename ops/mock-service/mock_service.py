#!/usr/bin/env python3

import json
import os
import signal
import socketserver
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


SERVICE_NAME = os.environ.get("MOCK_SERVICE_NAME", "mock-service")
PORTS = [
    int(part.strip())
    for part in os.environ.get("MOCK_PORTS", "8080").split(",")
    if part.strip()
]


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = {
            "service": SERVICE_NAME,
            "status": "ok",
            "path": self.path,
            "mode": "mock",
        }

        if self.path == "/status":
            body["result"] = {"sync_info": {"catching_up": False}}

        data = json.dumps(body, sort_keys=True).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        if length:
            self.rfile.read(length)

        body = {
            "service": SERVICE_NAME,
            "status": "accepted",
            "mode": "mock",
            "path": self.path,
        }
        data = json.dumps(body, sort_keys=True).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        print(f"{SERVICE_NAME} {self.address_string()} {fmt % args}", flush=True)


class ReusableThreadingHTTPServer(ThreadingHTTPServer):
    allow_reuse_address = True


def serve(port):
    server = ReusableThreadingHTTPServer(("0.0.0.0", port), Handler)
    print(f"{SERVICE_NAME} listening on 0.0.0.0:{port}", flush=True)
    server.serve_forever()


def main():
    if not PORTS:
        raise SystemExit("MOCK_PORTS must include at least one port")

    socketserver.ThreadingMixIn.daemon_threads = True

    stop = threading.Event()
    signal.signal(signal.SIGTERM, lambda *_: stop.set())
    signal.signal(signal.SIGINT, lambda *_: stop.set())

    for port in PORTS:
        thread = threading.Thread(target=serve, args=(port,), daemon=True)
        thread.start()

    stop.wait()


if __name__ == "__main__":
    main()
