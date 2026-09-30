# go-linters

Custom golangci-lint plugin module containing the `iferrinline` and `noanonstruct` analyzers.

`iferrinline` flags the pattern

```go
err := foo()
if err != nil { ... }
```

when `err` isn't referenced after the `if`, and suggests inlining it as

```go
if err := foo(); err != nil { ... }
```

It also handles multi-return assignments. When the other variables are
unused after the `if`, the suggestion is the same inline form:

```go
x, err := bar()
if err != nil { ... }
// ↓
if x, err := bar(); err != nil { ... }
```

When some companion variable (e.g. `x`) is still used after the `if`, the
diagnostic instead suggests hoisting it to a `var` declaration at the top of
the enclosing function and switching the assignment to `=`:

```go
var x SomeType
// ...
if x, err = bar(); err != nil { ... }
use(x)
```

## Autofix

The analyzer attaches a `SuggestedFix` for the simple-inline case, so
`golangci-lint run --fix` (or `iferrinline -fix ./...` against the standalone
binary) rewrites the source in-place. Example:

```sh
cd /path/to/some/other/repo
iferrinline -fix ./...
```

The hoist case is also autofixed: the analyzer uses type info to emit one
`var <name> <type>` per hoisted variable at the top of the enclosing
function and switches the assignment to `=`. It adds imports when a hoisted variable's type references a package that
isn't imported in the current file, choosing an alias that avoids name collisions. Same-line trailing comments on the
assignment are dropped by the rewrite.

## Develop

```sh
go mod tidy
go test ./...
```

## Run the analyzer standalone

For iteration without building the custom golangci-lint binary, install the
analyzer as its own binary via `singlechecker.Main`:

```sh
# from this repo — installs to $(go env GOPATH)/bin
go install ./cmd/iferrinline
```

Then run it from inside whatever project you want to lint. `go/packages`
honors the current directory's `go.mod`, so you must `cd` into the target
module first — running with an absolute path from elsewhere will fail with
`directory prefix ... does not contain main module`.

```sh
cd /path/to/some/other/repo
iferrinline ./...
iferrinline ./pkg/orchestrator/...
```

Exits non-zero (3) when any diagnostic is reported — that's the convention
from `go/analysis`. After editing the rule, re-run `go install ./cmd/iferrinline`
to refresh the installed binary.

## Build the custom golangci-lint binary

Requires `golangci-lint` >= v2 installed on `$PATH`. The version in
`.custom-gcl.yml` pins the golangci-lint version baked into the custom binary.

```sh
golangci-lint custom -v
```

The binary is written to `./bin/custom-gcl`.

## Run it

```sh
./bin/custom-gcl run ./...
```

It reads `.golangci.yml` from the project being linted. The plugin is enabled
under `linters.settings.custom.iferrinline` — see `.golangci.yml` here for a
template.

## Adding more analyzers

In `ifferrinline.go`:

- Register additional plugins with `register.Plugin("name", New)` in `init()`,
  each with its own `New` constructor — or
- Return multiple `*analysis.Analyzer` from `BuildAnalyzers()` under a single
  plugin name.

## noanonstruct

Requires a declared type name for non-empty structs in production Go files.
Checks literals, variable declarations, function signatures, and nested types.
Empty structs (`struct{}`) and all structs in `_test.go` files are allowed.
Direct struct type declarations (including aliases) are allowed; anonymous
structs nested inside them are still reported. No automatic fix is offered,
since naming and placing the new type requires a design decision.

Enable it in the target project's golangci-lint v2 configuration after rebuilding
the custom binary with `golangci-lint custom -v`:

```yaml
linters:
  enable:
    - noanonstruct
  settings:
    custom:
      noanonstruct:
        type: module
        description: reports non-empty anonymous structs outside test files
        original-url: github.com/justtrackio/go-linters
```
