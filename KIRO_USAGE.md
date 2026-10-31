# How Kiro was used

> **Status: template, to be completed by the author.**
> Kiroween requires this section to describe real usage. Do not submit this file until
> every `TODO` below is replaced with what actually happened in your sessions — judges
> cross-check the `.kiro/` directory against these claims.

## Session log

| # | Date | Kiro surface | What I asked for | What Kiro produced | What I changed by hand |
|---|---|---|---|---|---|
| 1 | TODO | Kiro IDE | TODO | TODO | TODO |
| 2 | TODO | | | | |

## Feature-by-feature

### Spec-driven development

TODO — describe how the spec in `.kiro/specs/phantomdep/` was authored and used.
Useful prompts to run first if you have not yet:

```
Read .kiro/steering/*.md and .kiro/specs/phantomdep/requirements.md.
Then implement task 7.1 (SBOM export) against the acceptance criteria in
tasks.md, updating design.md if the implementation forces a design change.
```

Answer these when writing the section:

- Which requirement IDs did Kiro implement directly from `requirements.md`?
- Did the spec change during implementation, and what forced the change?
- Where did the spec-driven approach beat plain prompting, and where did it get in the way?

### Steering documents

TODO — `steering/tech.md` forbids third-party dependencies. Note whether Kiro respected
that constraint unprompted, and what it did when a task appeared to need a library.

### Agent hooks

Three hooks are configured in `.kiro/hooks/`:

| Hook | Trigger | Action |
|---|---|---|
| `gofmt-on-save` | `**/*.go` edited | `gofmt -w` |
| `rust-fmt-clippy` | `analyzer-rs/**/*.rs` edited | `cargo fmt` + `cargo clippy -D warnings` |
| `fixture-regression` | fixtures, parsers or analyser edited | rebuild + `scripts/verify.py` |

TODO — did they fire when expected? Did the fixture-regression hook ever catch a real
regression? If a hook was too noisy, say so and say what you changed.

### MCP and Kiro powers

TODO — if you used MCP servers or Kiro powers, name them and describe what they enabled.
If you did not, write "not used" rather than inventing a section.

## Honest assessment

TODO — two paragraphs. What worked, and what cost you time. A submission that admits
where the tool struggled reads as more credible than one that claims everything was
effortless, and the judges explicitly score on "thoughtful strategy to AI development".
