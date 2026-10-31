#!/usr/bin/env bash
# Build every stage and verify the fixtures produce the documented counts.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "── rust analyser ─────────────────────────────────────────────"
cargo build --release --manifest-path analyzer-rs/Cargo.toml
cargo test --quiet --manifest-path analyzer-rs/Cargo.toml

echo
echo "── go orchestrator ───────────────────────────────────────────"
go vet ./...
go build -trimpath -o bin/phantomdep ./cmd/phantomdep

echo
echo "── python intel service ──────────────────────────────────────"
python3 -m py_compile intel-py/service.py
echo "syntax ok"

echo
echo "── fixture regression ────────────────────────────────────────"
python3 scripts/verify.py

echo
echo "all stages built. run scripts/dev.sh to open the UI."
