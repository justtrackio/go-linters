# go-linters

Custom golangci-lint bundle containing the `iferrinline`, `okinline`, `noanonstruct`, and `funcmethod` analyzers.

## Install with mise

Tagged releases include binaries for Linux and macOS on AMD64 and ARM64.
Each archive contains an executable named `golangci-lint`.

Select the bundle through its full backend name in the target repository's `mise.toml`:

```toml
[tools]
# Disable an inherited upstream golangci-lint tool.
golangci-lint = []
"github:justtrackio/go-linters" = "0.3.0"
```

The version selects a **go-linters release**, not an upstream golangci-lint release.
Remove the repository's upstream version pin.
The full backend name keeps the bundle separate from installed upstream binaries.

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
The individual `iferrinline`, `okinline`, `noanonstruct`, and `funcmethod` plugins remain available for selective use.
Enable either the aggregate or individual plugins, not both.

Existing lint commands remain unchanged when mise is activated or its shims are on `PATH`.
For an isolated shell, load the repository's tooling explicitly:

```sh
mise exec -- golangci-lint run --build-tags integration,fixtures ./...
```

`latest` does not continuously update installed tools.
Refresh it with `mise upgrade github:justtrackio/go-linters`, or add that command to the shared lint task.
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

## funcmethod

Reports private package functions whose production callers belong to one receiver
type, even when the function does not access receiver fields:

```go
func (s *SetupSchemaService) updateTable() {
    buildSchemaUpdate()
}

func buildSchemaUpdate() {} // make it a method of SetupSchemaService
```

The intended refactor is:

```go
func (s *SetupSchemaService) updateTable() {
    s.buildSchemaUpdate()
}

func (s *SetupSchemaService) buildSchemaUpdate() {}
```

Ownership follows direct calls through private helper chains across the whole
package, including recursive groups and calls inside closures, `go`, and `defer`.
Pointer and value receivers of the same type share ownership, as do generic
instantiations and receiver aliases. A helper shared by multiple receiver types
is allowed.

Test files do not establish ownership and receive no diagnostics. Ownership is
inferred from methods and production entry points (exported functions, `init`,
`main`, package initializers, and escaped function values). Private helpers
unreachable from these entry points, such as a `planSchemaUpdate` called only
from tests, do not prevent their callees from belonging to a production receiver
type. This works whether or not tests are loaded.

The rule conservatively skips exported functions, unused functions, function
values passed around as callbacks, functions reached from package initializers
or unrestricted package functions, generic functions with their own type
parameters, and names that collide with receiver fields or methods. Generated
declarations receive no diagnostics, but their calls still count. It analyzes
the files selected by the current build tags and platform.

No automatic fix is offered: converting helper chains requires adding receivers
and updating calls, including tests, while choosing pointer or value receivers.

Enable it in the target project's golangci-lint v2 configuration after rebuilding
the custom binary with `golangci-lint custom -v`:

```yaml
linters:
  enable:
    - funcmethod
  settings:
    custom:
      funcmethod:
        type: module
        description: reports private functions used only by a single receiver type that should be methods
        original-url: github.com/justtrackio/go-linters
```

For standalone use, build from the `linters` directory and run in the target module:

```sh
go build -o /tmp/funcmethod ./cmd/funcmethod
cd /path/to/target/module
/tmp/funcmethod ./...
```

## okinline

Reports assignments to `ok` immediately followed by `if !ok` or `if ok`,
including function calls, map lookups, type assertions, and channel receives.
It shares scope preservation and autofix behavior with `iferrinline`.

When the assigned variables are only used inside the `if`, it keeps `:=`:

```go
event, ok := NormalizeDenormalizeEventType(s.events, eventType)
if !ok {
    return SetupTableStatus{}, newBadRequestError("eventType is not configured")
}
// ↓
if event, ok := NormalizeDenormalizeEventType(s.events, eventType); !ok {
    return SetupTableStatus{}, newBadRequestError("eventType is not configured")
}
```

When `event` is used afterward, it declares `event` and `ok` at the top of the
function and uses `=` so the value remains available:

```go
var ok bool
var event EventType
// ...
if event, ok = NormalizeDenormalizeEventType(s.events, eventType); !ok {
    return SetupTableStatus{}, newBadRequestError("eventType is not configured")
}
use(event)
```

Existing `=` assignments can also be inlined. Like `iferrinline`, it skips
multiline assignments and checks that already have an initializer. A `:=`
assignment whose `ok` is used afterward without a companion to hoist is left
alone.

Enable it after rebuilding the custom binary with `golangci-lint custom -v`:

```yaml
linters:
  enable:
    - okinline
  settings:
    custom:
      okinline:
        type: module
        description: reports ok assignments followed by `if !ok` or `if ok` that can be inlined
        original-url: github.com/justtrackio/go-linters
```

For standalone use, install from the `linters` directory and run in the target module:

```sh
go install ./cmd/okinline
cd /path/to/target/module
okinline -fix ./...
```
