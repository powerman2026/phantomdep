# PhantomDep

**A dependency graveyard scanner.** PhantomDep finds the dependencies that are declared
but never imported, imported but never declared, and the cycles that hide inside a
lockfile — then draws them as tombstones.

Built for **Kiroween 2026**, Frankenstein category: four languages, each doing the one
thing it is genuinely best at.

```
$ phantomdep scan fixtures/haunted-node

  nodes 11 · declared 8 · phantom 5 · undeclared 1 · cycles 1 · maxDepth 3
  ghost score 47.5

  HIGH    Undeclared dependency   npm:ghost-missing-dep imported in 1 file(s), absent from every manifest
  HIGH    Dependency cycle        @ghost/ouija -> @ghost/seance -> @ghost/ouija
  MEDIUM  Advisory CVE-2021-23337 Command injection in lodash template
  LOW     Phantom dependency      npm:moment declared but never imported
```

## Why this exists

Every real repository accumulates dependencies nobody removed. `npm audit` tells you
about known vulnerabilities; it says nothing about the 40 % of your `package.json` that
no file imports. That dead weight is attack surface you are paying for and not using —
and it is invisible to every mainstream tool.

The name is literal: a *phantom dependency* is a package that resolves at runtime only
because a parent hoisted it into `node_modules`. Delete the parent and the child
vanishes. PhantomDep looks for the whole family of these ghosts.

## Quick start

Requires Go 1.21+, Rust 1.70+, Python 3.9+. **No third-party packages in any language.**

```bash
git clone <this repo> && cd phantomdep

./scripts/build.sh    # builds Rust + Go, runs Rust tests, verifies all fixtures
./scripts/dev.sh      # starts the Python intel service and the UI on :8787
```

Then open <http://127.0.0.1:8787> and click a fixture chip.

Headless, no browser:

```bash
./bin/phantomdep scan /path/to/your/repo -root .
```

## What it detects

| Defect | Definition | Why it matters |
|---|---|---|
| **Phantom** | declared in a manifest, imported by zero source files | dead weight, extra attack surface |
| **Undeclared** | imported by source, declared nowhere | works only by hoisting; breaks on a clean install |
| **Cycle** | strongly connected component of size ≥ 2 | makes upgrades and tree-shaking intractable |
| **Tombstone** | declared with `*`, `latest` or no version | unreproducible builds |

## Architecture

```
browser ──POST /api/scan {path}──▶  Go orchestrator
                                      │
                        ┌─────────────┼──────────────┐
                        ▼             ▼              ▼
                  scan stage     analyze stage    intel stage
                  Go, in-proc    Rust, stdin      Python, HTTP
                  manifests +    Tarjan SCC,      licences +
                  imports        depth, score     advisories
                        └──── Graph ──┴─ Analysis ──┴─ Intel
                                      ▼
                               ScanResult (JSON)
```

The split is functional, not decorative:

- **Go** — `filepath.WalkDir` plus `encoding/json` handles manifest parsing and import
  extraction with no dependencies, and the same binary serves HTTP.
- **Rust** — Tarjan SCC and repeated depth passes are pointer-chasing hot loops. Rust
  keeps them allocation-light and, more usefully, makes the adjacency-list construction
  hard to get wrong.
- **Python** — advisory payloads are nested and irregular; `urllib` plus dicts keeps the
  optional OSV client at ~20 lines.
- **Vanilla JS/SVG** — a framework would add a build step, which violates the zero-install
  rule. There is no `npm install` anywhere in this repository.

### IPC

JSON is used only at the two HTTP boundaries. Go → Rust is a line protocol because a
dependency-free JSON parser in Rust would be 200 lines of avoidable risk:

```
PHANTOMDEP-GRAPH 1
NODE 0 declared=1 imported=1 dev=0 tomb=0 files=0 name=ghost-house
EDGE 0 1 import
DONE
```

The Rust reply contains only integers, floats and index arrays, so it needs no string
escaping at all. See `.kiro/specs/phantomdep/design.md` §4.

### Graceful degradation

Every stage reports `ok`, `skipped` or `degraded`, and the UI renders a banner for each
non-ok stage. A clone without a Rust toolchain still produces a usable phantom report;
it just says the cycle analysis was skipped.

## Fixtures

Each fixture documents the exact defects it contains, so you can verify the claim instead
of trusting it. `scripts/verify.py` asserts every count and fails the build on drift.

| Fixture | Phantom | Undeclared | Cycles | Demonstrates |
|---|---|---|---|---|
| `haunted-node` | 5 | 1 | 1 | `package.json` + `package-lock.json` v3, transitive nodes, a real SCC |
| `go-crypt` | 2 | 0 | 0 | `go.mod` block `require`, Go stdlib exclusion |
| `py-ritual` | 3 | 2 | 0 | `requirements.txt` parsing and a distribution/import name mismatch |

## HTTP API

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/health` | GET | per-stage readiness |
| `/api/fixtures` | GET | bundled sample repositories |
| `/api/scan` | POST | `{"path": "/abs/dir"}` → full result |

The server binds `127.0.0.1` by default and refuses to scan outside the repository root
unless `-allow` widens the allowlist. Symlinks are resolved and checked before the walk.

## Known limitations

These are stated rather than hidden, because a scanner that overstates its confidence is
worse than no scanner.

1. **Python distribution vs import names.** `pyyaml` is declared, `yaml` is imported.
   Both the phantom verdict on `pyyaml` and the undeclared verdict on `yaml` are
   mechanically correct and useless to a human. Tracked in `tasks.md` §7.3;
   `fixtures/py-ritual` is the failing regression case.
2. **Dynamic imports** are invisible. `require(variable)` and `import(computed)` are not
   resolved.
3. **The ghost score is a heuristic**, not a verdict. The UI always renders it beside the
   raw counts that produced it.
4. **No advisory feed of its own.** The offline dataset is a fixture; `--live` defers to
   OSV.dev, which is the authoritative source.
5. **Go and Python have no lockfile edges**, so cycle detection is npm-only today.

## Kiro usage

`.kiro/` contains the spec, steering and hook configuration used to build this:
`specs/phantomdep/` (requirements → design → tasks), `steering/` (product, tech,
structure) and `hooks/` (format-on-save, clippy, fixture regression).

`KIRO_USAGE.md` records what was actually done in Kiro, per feature, including the parts
that did not work first time.

## Licence

MIT — see `LICENSE`.
