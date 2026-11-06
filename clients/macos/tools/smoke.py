"""Run the actual native client against a tiny local test fixture, never a model server."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import os
import subprocess
import sys
import threading

app = Path(sys.argv[1]).resolve(strict=True)
executable = app / 'Contents/MacOS/APTEX'
assert executable.is_file()
class Fixture(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != '/function/tasks':
            self.send_error(404)
            return
        body = b'<!doctype html><html data-aptex-smoke-ready><head><title>APTEX</title></head><body style="background:#090e13;color:#67e8f9;font:18px monospace;padding:48px"><h1>APTEX by Afterglow Labs</h1><p>Native client connected to the desktop-server fixture.</p></body></html>'
        self.send_response(200)
        self.send_header('Content-Type','text/html')
        self.send_header('Content-Length',str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self,*args): pass
server = ThreadingHTTPServer(('127.0.0.1',0),Fixture)
threading.Thread(target=server.serve_forever,daemon=True).start()
try:
    subprocess.run([str(executable),'--smoke-test',f'http://127.0.0.1:{server.server_port}'],env=os.environ.copy(),timeout=60,check=True)
finally:
    server.shutdown()
    server.server_close()
