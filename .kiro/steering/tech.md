# Tech Steering — PhantomDep

## Hard constraints

- **Go 1.21+ standard library only.** No `go.sum` beyond the toolchain line. If a task
  appears to need a third-party module, redesign the task.
- **Rust 2021 edition, standard library only.** No `serde`, no `clap`, no `regex`. JSON is
  emitted by hand and the IPC protocol is line-oriented precisely so that parsing stays
  trivial.
- **Python 3.9+ standard library only.** `http.server`, `json`, `urllib`. No FastAPI, no
  requests.
- **Frontend: no build step.** Plain ES modules, plain CSS, hand-written SVG. No bundler,
  no framework, no CDN script tags (the demo must work offline).

## Why the polyglot split

This is a Frankenstein-category entry, and the split is functional rather than decorative:

| Stage | Language | Justification |
|---|---|---|
| Manifest + import extraction | Go | Fast recursive file walking, `encoding/json` handles `package.json` natively |
| Graph algorithms | Rust | Tarjan SCC and scoring are pointer-chasing hot loops; Rust keeps them allocation-light |
| Advisory / licence intel | Python | Text and data wrangling, plus the optional OSV client is 20 lines with `urllib` |
| Visualisation | JS/SVG | No server round-trip per interaction |

If a stage is moved to another language, the justification table must be updated in the
same commit.

## IPC contract

Go is the orchestrator. It never links the other stages.

- **Go → Rust**: newline-delimited records on stdin, JSON on stdout.
  See `internal/analyze/rust.go` for the writer and `analyzer-rs/src/main.rs` for the reader.
  The protocol is versioned with a leading `PHANTOMDEP-GRAPH 1` line; bump it on any change
  and keep both sides in the same commit.
- **Go → Python**: HTTP on `127.0.0.1:8788`, single `GET /intel?ecosystem=..&packages=..`
  endpoint, JSON response. Loopback only — never bind `0.0.0.0`.

## Failure policy

Every stage returns a `status` field: `ok`, `skipped`, or `degraded`, plus a human-readable
reason. The Go orchestrator aggregates these into `ScanResult.Stages` and the UI renders a
banner per non-ok stage. A missing analyser binary is `skipped`, not an error.

## Conventions

- Go: `gofmt` clean, errors wrapped with `%w`, no `panic` outside `main`.
- Rust: `cargo fmt`, `cargo clippy -- -D warnings` clean, no `unwrap` on IO paths.
- Python: `ruff`-compatible formatting, type hints on public functions, no bare `except`.
- Commits: `stage: imperative summary` — e.g. `analyzer: add tombstone detection`.
