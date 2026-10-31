# Implementation Tasks — PhantomDep

Legend: `[x]` implemented and verified locally · `[ ]` open

## Stage 1 — Skeleton

- [x] 1.1 Repository layout, MIT licence, `.gitignore` that keeps `/.kiro` tracked
- [x] 1.2 Steering documents (product, tech, structure)
- [x] 1.3 Spec documents (requirements, design, tasks)

## Stage 2 — Go orchestrator

- [x] 2.1 `internal/model` shared types
- [x] 2.2 `package.json` parser (dependencies + devDependencies)
- [x] 2.3 `go.mod` parser (single-line and block `require`)
- [x] 2.4 `requirements.txt` parser
- [x] 2.5 `Cargo.toml` parser
- [x] 2.6 Import extraction: JS/TS/Vue, Go, Python, Rust
- [x] 2.7 Specifier normalisation and stdlib exclusion lists
- [x] 2.8 Rust analyser IPC client with skip/degrade handling
- [x] 2.9 Python intel client with timeout and degrade handling
- [x] 2.10 HTTP server: `/api/health`, `/api/scan`, `/api/fixtures`, static `web/`
- [x] 2.11 `scan` CLI subcommand for headless verification
- [x] 2.12 Path allowlist enforcement

## Stage 3 — Rust analyser

- [x] 3.1 Line-protocol reader with version banner check
- [x] 3.2 Adjacency list construction from node/edge records
- [x] 3.3 Tarjan SCC cycle detection
- [x] 3.4 Max-depth computation via iterative BFS
- [x] 3.5 Phantom / undeclared / tombstone classification
- [x] 3.6 Ghost score per design §5
- [x] 3.7 Hand-rolled string-free JSON reply

## Stage 4 — Python intel

- [x] 4.1 `http.server` on loopback with `/intel` and `/health`
- [x] 4.2 Offline dataset: licences + advisories for the fixture packages
- [x] 4.3 `--live` OSV.dev batch query merged over the offline dataset
- [x] 4.4 Unknown-package fallback

## Stage 5 — UI

- [x] 5.1 Layout, theme tokens, typography
- [x] 5.2 Scan form + fixture quick-pick
- [x] 5.3 Summary cards and stage banners
- [x] 5.4 Layered SVG graph with tombstone styling
- [x] 5.5 Package detail panel
- [x] 5.6 Cycle and advisory lists

## Stage 6 — Verification

- [x] 6.1 Three fixtures with documented defects
- [x] 6.2 `scripts/build.sh` full-pipeline check
- [x] 6.3 `scripts/dev.sh` one-command demo

## Open — intended for the next Kiro spec cycle

These are deliberately left open. Each one is scoped so that a spec-driven Kiro run can
implement it end to end from this document alone.

- [ ] 7.1 **SBOM export** — emit CycloneDX 1.5 JSON for the scanned repository, downloadable
      from the UI. Acceptance: `GET /api/scan?format=cyclonedx` returns a document that
      validates against the CycloneDX 1.5 schema subset used by the `components` and
      `dependencies` sections.
- [ ] 7.2 **Git history mode** — `--since <rev>` limits import extraction to files touched
      since a revision, so a reviewer can scan only the delta of a pull request.
      Acceptance: `git diff --name-only` is used as the file filter; a fixture with a clean
      base and a dirty head reports only the head's defects.
- [ ] 7.3 **Transitive resolution for npm lockfiles** — read `package-lock.json` v3 to
      distinguish a genuinely phantom package from one that is a real transitive dependency
      of a used package. Acceptance: a fixture where `ghost-left-pad` is hoisted by a used
      parent is reported as `transitive`, not `phantom`.
- [ ] 7.4 **Report export** — write a self-contained HTML report to `reports/` so a scan can
      be attached to a code review. Acceptance: the report renders with JS disabled and
      contains every finding and every graph node.
- [ ] 7.5 **Score calibration** — property-based tests over generated graphs asserting
      `0 <= ghostScore <= 100` and that removing a phantom never lowers the score.

## Submission chores (outside the repo)

- [ ] 8.1 Record the demo video (≤ 3 min, public on YouTube)
- [ ] 8.2 Publish the repository publicly with this licence visible in the About section
- [ ] 8.3 Fill `KIRO_USAGE.md` from real Kiro sessions
- [ ] 8.4 Enter the Devpost submission in the Frankenstein category
