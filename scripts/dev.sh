#!/usr/bin/env bash
# Start the full stack for a demo: Python intel service + Go orchestrator.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [ ! -x "analyzer-rs/target/release/phantomdep-analyzer" ]; then
  echo "Rust analyser not built — the analysis stage will be skipped."
  echo "Run scripts/build.sh for the full pipeline."
fi

INTEL_PID=""
cleanup() {
  if [ -n "$INTEL_PID" ] && kill -0 "$INTEL_PID" 2>/dev/null; then
    kill "$INTEL_PID" 2>/dev/null || true
    wait "$INTEL_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

python3 intel-py/service.py --port 8788 &
INTEL_PID=$!

# The Go client probes with a 300 ms timeout; give the socket a moment to bind.
for _ in $(seq 1 20); do
  if python3 - <<'PY' 2>/dev/null
import socket
s = socket.socket()
s.settimeout(0.2)
try:
    s.connect(("127.0.0.1", 8788))
finally:
    s.close()
PY
  then
    break
  fi
  sleep 0.1
done

go run ./cmd/phantomdep -addr 127.0.0.1:8787 -root "$ROOT"
