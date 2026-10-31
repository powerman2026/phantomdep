# fixture: haunted-node

Demonstrates the three npm defect classes in one directory.

## Declared (8)

`express`, `lodash`, `moment`, `left-pad`, `request`, `@ghost/ouija`, `@ghost/seance`, `chalk` (dev)

## Imported (4)

`express`, `lodash`, `chalk` — plus `ghost-missing-dep`, which appears in no manifest.

## Expected scan result

| Metric | Value | Why |
|---|---|---|
| nodes | 11 | 8 declared + 2 lockfile-only (`body-parser`, `bytes`) + 1 undeclared |
| phantom | 5 | `moment`, `left-pad`, `request`, `@ghost/ouija`, `@ghost/seance` are declared but never imported |
| undeclared | 1 | `ghost-missing-dep` is required by `src/legacy.js` and declared nowhere |
| transitive | 2 | `body-parser` and `bytes` appear only in the lockfile |
| cycles | 1 | `package-lock.json` makes `@ghost/ouija` and `@ghost/seance` mutually dependent |
| tombstones | 1 | `left-pad` is pinned to `*` |
| maxDepth | 3 | root → `express` → `body-parser` → `bytes` |
| ghostScore | 47.5 | five phantoms, two of which hide a subtree |

The two `transitive` nodes exist so that depth and cycle analysis have something to
measure. Without them every node sits one hop from the root and the depth term of the
ghost score would be constant.


## Why `@ghost/*`

The two scoped packages are synthetic. A real npm lockfile can contain cycles, but
attaching one to `express` and `lodash` would be a false claim about those packages.
The `@ghost/` prefix makes the synthetic part obvious while still exercising the
Tarjan path in the Rust analyser.
