# Design — PhantomDep

## 1. Pipeline

```
browser ──POST /api/scan {path}──▶ Go orchestrator
                                     │
                       ┌─────────────┼──────────────┐
                       ▼             ▼              ▼
                 scan stage    analyze stage    intel stage
                 (Go, in-proc)  (Rust, stdin)   (Python, HTTP)
                       │             │              │
                       └──── Graph ──┴─ Analysis ───┴─ Intel
                                     ▼
                              ScanResult (JSON)
```

The Go process owns the request lifecycle. The Rust analyser is a short-lived child process
spawned per scan. The Python intel service is long-lived and started by `scripts/dev.sh`;
the Go client probes it once per scan and degrades if it is absent.

## 2. Data model

`internal/model` is the single source of truth.

```go
type Package struct {
    Name      string  // normalised, ecosystem-qualified in the UI
    Ecosystem string  // npm | go | pypi | cargo
    Version   string  // raw declared range, empty when unspecified
    Dev       bool
    Declared  bool
    Imported  bool
    License   string
    Advisories []Advisory
}

type Edge struct { From, To int; Kind string } // import | dev

type Graph struct {
    Root     string
    Packages []Package
    Edges    []Edge
}
```

`Analysis` (produced by Rust) adds `Cycles [][]int`, `MaxDepth int`, `GhostScore float64`,
`Tombstones []int`, `Phantom []int`, `Undeclared []int`.

## 3. Why the stages are split this way

The split follows data shape, not fashion:

- **Extraction is IO-bound and string-heavy.** Go's `filepath.WalkDir` plus `encoding/json`
  covers it with no dependencies, and the same binary can serve HTTP.
- **Analysis is pointer-chasing.** Tarjan SCC over an adjacency list, repeated depth passes
  and a weighted score. Rust gives predictable performance and, more importantly, an
  ownership model that makes the adjacency-list construction obviously correct.
- **Intel is schema-chasing.** Advisory payloads are nested and irregular; Python's dict
  handling and `urllib` keep the optional OSV client at ~20 lines.
- **Presentation is interaction-heavy.** A framework would add a build step, which violates
  the zero-install principle.

## 4. IPC: the line protocol

JSON is used only at the two HTTP boundaries. Go → Rust uses a line protocol because writing
a JSON parser in dependency-free Rust is a needless 200 lines of risk:

```
PHANTOMDEP-GRAPH 1
NODE 0 root declared=1 imported=1 dev=0 name=haunted-node
NODE 1 ghost-left-pad declared=1 imported=0 dev=0 name=left-pad
EDGE 0 1 import
DONE
```

Rules:
- The first line is the version banner. A mismatch is a hard error on both sides.
- `NODE` fields after `name=` are free-form until end of line, so package names may contain
  spaces without escaping.
- `EDGE` references node indices allocated by Go, so Rust never has to resolve names.
- `DONE` is required; its absence means truncated input and the analyser exits 2.

Rust replies with a single-line JSON object on stdout. Because every value is either an
integer, a float or an index array, hand-rolled serialisation is safe — no string escaping
is required in the reply at all. This is a deliberate design constraint, not laziness:
keeping the reply string-free removes the entire class of escaping bugs.

## 5. Scoring

`ghostScore` is a heuristic, and the UI labels it as such.

```
phantomMass  = Σ over phantom packages of (1 + transitiveFanIn(pkg) / 10)
depthPenalty = min(maxDepth, 8) / 8
ghostScore   = clamp( 100 * phantomMass / (declaredCount + 1) * (0.6 + 0.4*depthPenalty), 0, 100 )
```

`declaredCount` excludes node 0, the repository root: the root is not a dependency and
would otherwise inflate the denominator by one. `maxDepth` is the BFS distance from the
root, so a flat manifest with no lockfile scores `maxDepth = 1` and the depth term
collapses to a constant 0.65 — that is expected, not a bug.

`transitiveFanIn` counts how many declared packages depend on the phantom package, so a
phantom that hides a subtree scores higher than a leaf nobody ever imported.

The score is only ever rendered alongside the raw counts that produced it (NFR4).

## 6. Failure modes

| Failure | Detection | Behaviour |
|---|---|---|
| Rust binary absent | `os.Stat` on `analyzer-rs/target/release/phantomdep-analyzer` | stage `skipped`, analysis computed by the Go fallback (phantom + undeclared only, no cycles) |
| Rust exits non-zero | `cmd.Wait()` | stage `degraded`, stderr surfaced in the banner |
| Python service absent | dial `127.0.0.1:8788` with 300 ms timeout | stage `skipped`, licences `unknown`, no advisories |
| Manifest malformed | parser error | finding of severity `high`, scan continues |
| Path outside allowlist | prefix check on cleaned absolute path | HTTP 403 |

## 7. Security posture

- Path traversal is blocked by resolving the target with `filepath.Abs` + `EvalSymlinks`
  and requiring a prefix match against the allowlist root, which defaults to the repository
  root. Symlinks are not followed during the walk.
- The HTTP server binds `127.0.0.1` by default; `--addr` exists for demoing over a tunnel
  and prints a warning when used.
- No request body is written to disk. Scan results are held in memory and returned.

## 8. Test strategy

- `fixtures/` doubles as the regression suite. Each fixture documents the exact defect it
  contains, so `phantomdep scan fixtures/<name>` has a deterministic expected count.
- `scripts/build.sh` runs `cargo build --release`, `go build`, and a `python3 -m py_compile`
  check, then scans all three fixtures and fails if any expected count drifts.
- No mocking framework: the stages are tested through the real IPC boundary, which is the
  boundary most likely to break.
