# Requirements — PhantomDep

Status: accepted for build during the Kiroween 2026 submission period (2026-10-30 → 2026-12-04).
Scope: single-repository scan, local execution, four-stage polyglot pipeline.

## Glossary

- **Declared dependency** — appears in a manifest file (`package.json`, `go.mod`,
  `requirements.txt`, `Cargo.toml`) of the scanned repository.
- **Imported dependency** — a module specifier that appears in at least one source file of
  the scanned repository.
- **Phantom dependency** — declared, never imported. A tombstone in the UI.
- **Undeclared dependency** — imported, never declared. Resolves only via hoisting.
- **Tombstone** — a declared dependency pinned to a wildcard or missing version.
- **Cycle** — a strongly connected component of size ≥ 2 in the dependency graph.

## Functional requirements

### R1 — Repository intake

- R1.1 WHEN the user submits an absolute directory path THE SYSTEM SHALL scan that
  directory recursively, skipping `.git`, `node_modules`, `target`, `dist`, `bin`,
  `.venv`, `__pycache__` and any directory listed in the ignore file.
- R1.2 WHEN the path does not exist or is not a directory THE SYSTEM SHALL return HTTP 400
  with a message naming the rejected path.
- R1.3 THE SYSTEM SHALL refuse to scan the filesystem root or a path outside the configured
  allowlist root, returning HTTP 403.

### R2 — Manifest parsing

- R2.1 THE SYSTEM SHALL parse `package.json` (`dependencies`, `devDependencies`) into npm
  packages.
- R2.2 THE SYSTEM SHALL parse `go.mod` `require` directives, including block form, into go
  packages, and SHALL exclude the main module itself.
- R2.3 THE SYSTEM SHALL parse `requirements.txt` into pypi packages, ignoring comments,
  blank lines, `-r` includes and environment markers.
- R2.4 THE SYSTEM SHALL parse `Cargo.toml` `[dependencies]` and `[dev-dependencies]` into
  cargo packages, ignoring table sub-keys and workspace inheritance entries.
- R2.5 WHEN a manifest is syntactically invalid THE SYSTEM SHALL record a finding of
  severity `high` naming the file and SHALL continue scanning the remaining manifests.

### R3 — Import extraction

- R3.1 THE SYSTEM SHALL extract ES module and CommonJS specifiers from `.js`, `.mjs`,
  `.cjs`, `.ts`, `.tsx`, `.vue` files.
- R3.2 THE SYSTEM SHALL extract import paths from `.go` files, ignoring the standard
  library by requiring a dot in the first path segment.
- R3.3 THE SYSTEM SHALL extract `import` / `from ... import` module names from `.py` files,
  ignoring modules in the standard library allowlist.
- R3.4 THE SYSTEM SHALL extract `use` crate roots from `.rs` files.
- R3.5 THE SYSTEM SHALL normalise scoped npm specifiers (`@scope/pkg/sub` → `@scope/pkg`)
  and strip Python submodule paths (`pkg.sub` → `pkg`).

### R4 — Graph analysis (Rust stage)

- R4.1 THE SYSTEM SHALL classify every package as declared, imported, both, or neither.
- R4.2 THE SYSTEM SHALL compute all strongly connected components of size ≥ 2 and report
  each as a cycle.
- R4.3 THE SYSTEM SHALL compute the maximum dependency depth from the root package.
- R4.4 THE SYSTEM SHALL compute a ghost score in `[0, 100]` where a higher value means more
  declared-but-unused mass, weighted by package count and transitive depth.
- R4.5 WHEN the analyser receives a malformed protocol record THE SYSTEM SHALL exit non-zero
  with the offending line number on stderr.

### R5 — Advisory intel (Python stage)

- R5.1 THE SYSTEM SHALL answer licence and advisory lookups for a batch of package names in
  a single request.
- R5.2 WHEN a package is absent from the offline dataset THE SYSTEM SHALL return
  `license: "unknown"` and an empty advisory list rather than an error.
- R5.3 WHEN started with `--live` THE SYSTEM SHALL query `https://api.osv.dev/v1/querybatch`
  and merge results with the offline dataset.
- R5.4 THE SYSTEM SHALL bind to `127.0.0.1` only.

### R6 — Presentation

- R6.1 THE SYSTEM SHALL render every package as a node with a visual distinction between
  live, phantom and undeclared states.
- R6.2 THE SYSTEM SHALL render a banner for each pipeline stage whose status is not `ok`,
  naming the stage and the reason.
- R6.3 THE SYSTEM SHALL allow scanning a bundled fixture in one click without typing a path.
- R6.4 THE SYSTEM SHALL remain usable with JavaScript modules loaded from the local server
  only; no external origins.

### R7 — Operability

- R7.1 THE SYSTEM SHALL expose `GET /api/health` returning per-stage readiness.
- R7.2 THE SYSTEM SHALL expose a `scan` CLI subcommand that prints the full JSON result to
  stdout so the pipeline can be verified without a browser.
- R7.3 THE SYSTEM SHALL run with zero third-party dependencies in all four languages.

## Non-functional requirements

- NFR1 A scan of a repository with 2 000 source files SHALL complete in under 3 seconds on
  an M-series laptop.
- NFR2 No source file content SHALL be transmitted off the machine in the default mode.
- NFR3 The UI SHALL remain functional at 1280×720 without horizontal scrolling.
- NFR4 Every reported number SHALL be reproducible from the JSON in `/api/scan`.

## Out of scope

Autofix, continuous monitoring, SBOM signing, container image scanning, private registry
authentication.
