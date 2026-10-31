# fixture: go-crypt

Demonstrates phantom detection in a `go.mod` block `require` list, and confirms
that Go standard-library imports (`net/http`) are not reported as undeclared.

## Declared (3)

`github.com/gin-gonic/gin`, `github.com/spf13/viper`, `golang.org/x/crypto`

## Imported (1)

`github.com/gin-gonic/gin` — `net/http` is standard library and is excluded by the
"first path segment must contain a dot" rule.

## Expected scan result

| Metric | Value |
|---|---|
| phantom | 2 (`viper`, `golang.org/x/crypto`) |
| undeclared | 0 |
| cycles | 0 |
| maxDepth | 1 |

`golang.org/x/crypto` also carries an advisory in the offline dataset, so the intel
stage produces a finding even though the package is phantom — which is exactly the
argument for deleting it.
