#!/usr/bin/env python3
"""Fixture regression check.

Each fixture documents its expected defect counts in its README. This script runs
the real CLI (no mocking) and fails if any count drifts, which is the cheapest
possible guard against a parser regression.
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BIN = ROOT / "bin" / "phantomdep"

EXPECTED = {
    "haunted-node": {"phantom": 5, "undeclared": 1, "cycles": 1},
    "go-crypt": {"phantom": 2, "undeclared": 0, "cycles": 0},
    "py-ritual": {"phantom": 3, "undeclared": 2, "cycles": 0},
}


def scan(fixture: str) -> dict:
    proc = subprocess.run(
        [str(BIN), "scan", "-root", str(ROOT), str(ROOT / "fixtures" / fixture)],
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        raise SystemExit(f"{fixture}: scan failed\n{proc.stderr}")
    return json.loads(proc.stdout)


def main() -> int:
    if not BIN.exists():
        raise SystemExit(f"missing {BIN}; run scripts/build.sh first")

    failures = 0
    print(f"{'fixture':<16} {'phantom':>8} {'undeclared':>11} {'cycles':>7}  result")
    print("-" * 62)

    for fixture, expected in EXPECTED.items():
        result = scan(fixture)
        actual = {
            "phantom": result["stats"]["phantom"],
            "undeclared": result["stats"]["undeclared"],
            "cycles": result["stats"]["cycles"],
        }
        ok = actual == expected
        failures += 0 if ok else 1
        print(
            f"{fixture:<16} {actual['phantom']:>8} {actual['undeclared']:>11} "
            f"{actual['cycles']:>7}  {'ok' if ok else 'MISMATCH ' + str(expected)}"
        )

    print("-" * 62)
    if failures:
        print(f"{failures} fixture(s) drifted")
        return 1
    print("all fixtures match their documented counts")
    return 0


if __name__ == "__main__":
    sys.exit(main())
