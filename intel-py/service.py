#!/usr/bin/env python3
"""PhantomDep advisory intel service.

Standard library only. Serves licence and advisory metadata for the packages the
Go orchestrator has already extracted. It never scans source code: that is the
Go stage's job, and keeping the split clean is what makes this stage swappable.

Endpoints
    GET  /health            -> {"ok": true, "packages": <dataset size>, "live": <bool>}
    POST /intel             -> {"items": [Entry, ...]}
         body: {"items": [{"ecosystem": "npm", "name": "left-pad"}, ...]}

The offline dataset is authoritative for the fixtures. With --live, results from
OSV.dev are merged on top, but an OSV failure never removes an offline answer.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Any

DATA_PATH = os.path.join(os.path.dirname(os.path.abspath(__file__)), "data", "advisories.json")
OSV_BATCH_URL = "https://api.osv.dev/v1/querybatch"
MAX_BODY = 1 << 20


def load_dataset(path: str = DATA_PATH) -> dict[str, dict[str, Any]]:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    flat: dict[str, dict[str, Any]] = {}
    for ecosystem, packages in raw.get("packages", {}).items():
        for name, entry in packages.items():
            flat[f"{ecosystem}:{name.lower()}"] = {
                "ecosystem": ecosystem,
                "name": name,
                "license": entry.get("license", "unknown"),
                "deprecated": bool(entry.get("deprecated", False)),
                "advisories": entry.get("advisories", []),
            }
    return flat


def osv_lookup(items: list[dict[str, str]], timeout: float = 4.0) -> dict[str, list[dict[str, Any]]]:
    """Batch query OSV.dev. Returns {key: [advisory, ...]} for hits only."""
    osv_ecosystem = {"npm": "npm", "pypi": "PyPI", "cargo": "crates.io", "go": "Go"}
    queries = []
    keys = []
    for it in items:
        eco = osv_ecosystem.get(it["ecosystem"])
        if not eco:
            continue
        queries.append({"package": {"name": it["name"], "ecosystem": eco}})
        keys.append(f"{it['ecosystem']}:{it['name'].lower()}")

    if not queries:
        return {}

    payload = json.dumps({"queries": queries}).encode("utf-8")
    req = urllib.request.Request(
        OSV_BATCH_URL,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = json.loads(resp.read().decode("utf-8"))
    except (urllib.error.URLError, TimeoutError, ValueError) as exc:
        print(f"[intel] OSV lookup skipped: {exc}", file=sys.stderr)
        return {}

    out: dict[str, list[dict[str, Any]]] = {}
    for key, result in zip(keys, body.get("results", [])):
        vulns = result.get("vulns") or []
        if not vulns:
            continue
        advisories = []
        for vuln in vulns[:5]:
            severity = "medium"
            for sev in vuln.get("severity", []) or []:
                if sev.get("score", "").startswith("CVSS:4") or sev.get("score", "").startswith("CVSS:3"):
                    severity = "high"
            advisories.append(
                {
                    "id": vuln.get("id", "OSV-unknown"),
                    "severity": severity,
                    "summary": (vuln.get("summary") or vuln.get("details") or "")[:220],
                    "fixedIn": "",
                }
            )
        out[key] = advisories
    return out


class Handler(BaseHTTPRequestHandler):
    dataset: dict[str, dict[str, Any]] = {}
    live: bool = False

    server_version = "PhantomDepIntel/1.0"

    def log_message(self, fmt: str, *args: Any) -> None:  # quieter default logging
        if self.path.startswith("/intel"):
            sys.stderr.write("[intel] %s\n" % (fmt % args))

    def _send(self, code: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload).encode("utf-8")
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:  # noqa: N802 - stdlib naming
        if self.path.startswith("/health"):
            self._send(
                200,
                {"ok": True, "packages": len(self.dataset), "live": self.live},
            )
        else:
            self._send(404, {"error": "unknown path"})

    def do_POST(self) -> None:  # noqa: N802 - stdlib naming
        if not self.path.startswith("/intel"):
            self._send(404, {"error": "unknown path"})
            return

        length = int(self.headers.get("Content-Length") or 0)
        if length <= 0 or length > MAX_BODY:
            self._send(400, {"error": "missing or oversized body"})
            return

        try:
            request = json.loads(self.rfile.read(length).decode("utf-8"))
            items = request.get("items", [])
            if not isinstance(items, list):
                raise ValueError("items must be a list")
        except (ValueError, UnicodeDecodeError) as exc:
            self._send(400, {"error": f"bad request: {exc}"})
            return

        osv_hits: dict[str, list[dict[str, Any]]] = {}
        if self.live:
            osv_hits = osv_lookup(items)

        out = []
        for it in items:
            try:
                key = f"{it['ecosystem']}:{str(it['name']).lower()}"
            except (KeyError, TypeError):
                continue
            entry = self.dataset.get(key)
            if entry is None:
                out.append(
                    {
                        "ecosystem": it.get("ecosystem", ""),
                        "name": it.get("name", ""),
                        "license": "unknown",
                        "deprecated": False,
                        "advisories": osv_hits.get(key, []),
                    }
                )
                continue
            merged = dict(entry)
            if key in osv_hits:
                seen = {a.get("id") for a in merged.get("advisories", [])}
                extra = [a for a in osv_hits[key] if a.get("id") not in seen]
                merged["advisories"] = list(merged.get("advisories", [])) + extra
            out.append(merged)

        self._send(200, {"items": out})


def main() -> int:
    parser = argparse.ArgumentParser(description="PhantomDep advisory intel service")
    parser.add_argument("--host", default="127.0.0.1", help="bind address (loopback only by default)")
    parser.add_argument("--port", type=int, default=8788)
    parser.add_argument("--live", action="store_true", help="also query OSV.dev (requires network)")
    args = parser.parse_args()

    if args.host not in ("127.0.0.1", "localhost", "::1"):
        print(
            f"[intel] refusing to bind {args.host}: this service must stay on loopback",
            file=sys.stderr,
        )
        return 2

    Handler.dataset = load_dataset()
    Handler.live = args.live

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(
        f"[intel] listening on http://{args.host}:{args.port} "
        f"({len(Handler.dataset)} packages, live={args.live})",
        file=sys.stderr,
    )
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("[intel] stopped", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
