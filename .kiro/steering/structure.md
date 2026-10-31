# Structure Steering — PhantomDep

## Layout

```
.
├── cmd/phantomdep/         Go entrypoint: HTTP server + `scan` CLI subcommand
├── internal/
│   ├── model/              Shared types crossing every stage boundary
│   ├── scan/               Manifest parsing and import extraction
│   ├── analyze/            Rust analyser IPC client
│   └── intel/              Python intel service client
├── analyzer-rs/            Rust graph analyser (std bin)
├── intel-py/               Python advisory service (http.server)
├── web/                    Static UI served by Go
├── fixtures/               Deliberately haunted sample repositories
└── scripts/                dev.sh, build.sh
```

## Rules

1. `internal/model` imports nothing from the other internal packages. It is the only place
   where cross-stage types are defined. Anything added here is a protocol change and needs
   both sides updated.
2. `internal/scan` must not know about Rust or Python. It produces a `Graph` and stops.
3. `cmd/phantomdep` is the only package allowed to wire stages together.
4. `web/` must not hardcode fixture names. Everything it renders comes from `/api/scan`.
5. `fixtures/` is part of the demo, not test data with hidden semantics. Each fixture
   directory gets a `README.md` naming the exact defect it demonstrates, so a judge can
   verify the scanner found the thing we claim it found.

## Adding a new ecosystem

Touch exactly three places:

1. `internal/scan/manifests.go` — a parser returning `[]model.Package`
2. `internal/scan/imports.go` — an import extractor for that language's file extensions
3. `intel-py/data/advisories.json` — seed entries, or the intel stage reports `unknown`

Anything else means the ecosystem abstraction leaked. Fix the abstraction instead.

## Naming

- Wire format field names: `snake_case` inside Rust/Python payloads, `camelCase` in the
  JSON returned to the browser. Conversion happens once, in `internal/model`.
- Fixture package names use a `ghost-` prefix so a grep for `ghost-` in the UI output
  proves the pipeline reached that fixture.
