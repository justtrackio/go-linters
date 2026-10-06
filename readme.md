# go-linters

Custom golangci-lint bundle containing the `iferrinline` and `noanonstruct` analyzers.

## Install with mise

Tagged releases include binaries for Linux and macOS on AMD64 and ARM64.
Each archive contains an executable named `golangci-lint`.

Change the provider for the existing tool in the target repository's `mise.toml`:

```toml
[tool_alias.golangci-lint]
backend = "github:justtrackio/go-linters"

[tools]
golangci-lint = "latest"
```

The version selects a **go-linters release**, not an upstream golangci-lint release.
Replace any existing upstream version pin with `latest` or an exact bundle release version.
At least one bundle release with binary assets must exist before installation works.

```sh
mise install
```

Installing the bundle does not enable its rules.
Merge this configuration into the target repository's `.golangci.yml`.
Keep its existing standard linters and settings.

```yaml
version: "2"

linters:
  enable:
    - justtrack
  settings:
    custom:
      justtrack:
        type: module
        description: runs the justtrack company rules
        original-url: github.com/justtrackio/go-linters
```

The `justtrack` plugin runs every analyzer in the bundle.
Repositories configure it once, including when later releases add rules.
The individual `iferrinline` and `noanonstruct` plugins remain available for selective use.
Enable either the aggregate or individual plugins, not both.

Existing lint commands remain unchanged when mise is activated or its shims are on `PATH`.
For an isolated shell, load the repository's tooling explicitly:

```sh
mise exec -- golangci-lint run --build-tags integration,fixtures ./...
```

`latest` does not continuously update installed tools.
Refresh it with `mise upgrade golangci-lint`, or add that command to the shared lint task.
A committed `mise.lock` can retain an older version.
New releases can introduce lint failures on unchanged branches.

## iferrinline

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

## Build and check the bundle

```sh
mise install
mise run test
mise run bundle
mise run test-bundle
```

`.custom-gcl.yml` pins the embedded golangci-lint version and imports this local checkout.
The custom binary is written to `./bin/golangci-lint`.
Analyzer tests use `analysistest` with checked-in Go fixtures and expected fix outputs.
The Go bundle integration test reuses these fixtures to check diagnostics and real `--fix` behavior.
It compares formatted golden files, compiles corrected packages, and verifies that they lint cleanly.

Run the custom binary from the project that you want to lint:

```sh
/path/to/go-linters/bin/golangci-lint run ./...
```

It reads that project's `.golangci.yml`, including the `justtrack` configuration shown above.

## Adding more analyzers

See [Add a linter](docs/adding-linters.md) for implementation, fixtures, autofix tests, and release steps.
The bundle test discovers existing golden files without additional per-rule examples.

## Releases

After merging a release commit, create and push a new `v*` tag.
The bundle workflow runs tests and builds all four platform archives.
It runs the Go bundle integration test on the native Linux AMD64 binary before publication.
The release job uploads the archives and `SHA256SUMS` to GitHub.
Pull requests and main-branch pushes build archives without publishing releases.

The bundle release version and embedded golangci-lint version are separate.
Keep the builder pin in `mise.toml`, the version in `.custom-gcl.yml`,
and the upstream license URL in the workflow aligned when upgrading golangci-lint.
Each archive includes the upstream runner's GPLv3 license as `LICENSE.golangci-lint`.

## noanonstruct

Requires a declared type name for non-empty structs in production Go files.
Checks literals, variable declarations, function signatures, and nested types.
Empty structs (`struct{}`) and all structs in `_test.go` files are allowed.
Direct struct type declarations (including aliases) are allowed; anonymous
structs nested inside them are still reported. No automatic fix is offered,
since naming and placing the new type requires a design decision.

The `justtrack` plugin enables this rule automatically.
For selective use, configure the individual `noanonstruct` module plugin instead.
