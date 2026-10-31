# Product Steering — PhantomDep

## What this is

PhantomDep is a dependency-supply-chain scanner for the "ghost" class of dependency
defects: packages that are declared but never imported, packages that are imported but
never declared, and dependency cycles that hide inside a monorepo.

The name comes from the npm ecosystem's real *phantom dependency* problem — a package
that resolves at runtime only because a transitive dependency happens to hoist it into
`node_modules`. Remove the parent and the child vanishes.

## Who it is for

- Developers inheriting a repository with a bloated `package.json`
- Security reviewers who need a fast, local, zero-install first pass before a full SCA tool
- Anyone preparing a hackathon submission who has to prove a dependency graph is clean

## Product principles

1. **Zero install.** No `npm install`, no virtualenv, no `go mod download`. A reviewer
   clones the repo and runs one script. Every dependency we add is a reason for someone
   not to try it.
2. **Local first.** Source code never leaves the machine. The optional advisory lookup is
   the only outbound call and it is off by default.
3. **Explain, do not score.** A "ghost score" is a summary, not a verdict. Every number in
   the UI must be traceable to a specific edge or file.
4. **Degrade loudly.** If the Rust analyser or the Python intel service is missing, the app
   still runs and the UI says which stage was skipped. Silent degradation is a bug.

## Deliberate non-goals

- Not a replacement for Snyk / Dependabot / `npm audit`. We do not maintain a CVE feed.
- No autofix. The tool reports; the human decides.
- No network account, no telemetry, no cloud sync.

## Tone

The visual theme is a graveyard because the subject matter is dead code. The copy stays
literal: a tombstone means "declared, never imported", nothing more.
