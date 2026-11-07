#!/bin/bash
# Adapted from afterglow-labs/odysseus-ios f81c2cd09b90, AGPL-3.0.
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -f APTEX/Info.plist && -f scripts/fixture_server.py ]] || { echo 'APTEX sources are missing.' >&2; exit 1; }
[[ $# -ge 1 ]] || { echo 'Usage: scripts/test.sh <booted simulator UDID> [xcodebuild test options]' >&2; exit 2; }
simulator_id=$1
shift
python3 - <<'PY'
import json,socket,subprocess,sys
with socket.socket() as probe:
    probe.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    try: probe.bind(('127.0.0.1',18768))
    except OSError: sys.exit('Port 18768 is occupied; refusing to attach to an unknown test server.')
PY
mkdir -p build
python3 scripts/fixture_server.py > build/fixture.log 2>&1 &
fixture_pid=$!
cleanup_fixture() { kill "$fixture_pid" 2>/dev/null || true; wait "$fixture_pid" 2>/dev/null || true; }
trap cleanup_fixture EXIT
python3 - <<'PY'
import time,urllib.request
for attempt in range(30):
    try:
        with urllib.request.urlopen('http://127.0.0.1:18768/api/v1/meta',timeout=1) as response: assert response.status==200
        break
    except OSError: time.sleep(.1)
else: raise SystemExit('The owned APTEX fixture did not start.')
PY
python3 scripts/generate_project.py
xcodebuild -project APTEX.xcodeproj -scheme APTEX -configuration Debug \
  -destination "platform=iOS Simulator,id=$simulator_id" -derivedDataPath build \
  -resultBundlePath "build/tests-$(date +%Y%m%d-%H%M%S).xcresult" \
  -parallel-testing-enabled NO -collect-test-diagnostics never \
  CODE_SIGN_IDENTITY=- DEVELOPMENT_TEAM="${APTEX_DEVELOPMENT_TEAM:-}" "$@" test
