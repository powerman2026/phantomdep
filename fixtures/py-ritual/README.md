# fixture: py-ritual

Demonstrates `requirements.txt` parsing (including a comment, a blank line and a
`>=` pin) and the Python standard-library exclusion list.

## Declared (4)

`pyyaml`, `requests`, `rich`, `jinja2`

## Imported (3)

`yaml` → `pyyaml`, `rich` → `rich`, `ghost_import` → undeclared.
`json`, `os` and `__future__` are standard library and are excluded.

## Expected scan result

| Metric | Value |
|---|---|
| phantom | 3 (`pyyaml`, `requests`, `jinja2`) |
| undeclared | 2 (`yaml`, `ghost_import`) |
| cycles | 0 |
| maxDepth | 1 |

Note the naming gap this fixture deliberately exposes: the declared distribution is
`pyyaml` but the import name is `yaml`, so the scanner reports `pyyaml` as phantom
and `yaml` as undeclared. Both verdicts are mechanically correct and both are
useless to a human. Resolving distribution-name to import-name pairs is the open
problem tracked in tasks.md §7.3 — this fixture is the regression case for it, and
it is left failing on purpose rather than papered over with a hardcoded alias table.

