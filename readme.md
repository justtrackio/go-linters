# go-linters

Custom golangci-lint bundle containing the `funcmethod`, `iferrinline`, `methodfilecheck`, `multilineblock`, `noanonstruct`, `okinline`, and `recvaftertype` analyzers.

## Install with mise

Tagged releases include binaries for Linux and macOS on AMD64 and ARM64.
Each archive contains an executable named `golangci-lint`.

Select the bundle through its full backend name in the target repository's `mise.toml`:

```toml
[settings]
# Disable the inherited upstream tool while retaining the full-backend bundle.
disable_tools = ["golangci-lint"]

[tools]
"github:justtrackio/go-linters" = "0.4.2"
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

The `justtrack` plugin runs the active company analyzers in the bundle.
Do not enable individual plugins already included in the aggregate.

Existing lint commands remain unchanged when mise is activated or its shims are on `PATH`.
For an isolated shell, load the repository's tooling explicitly:

```sh
mise exec -- golangci-lint run --build-tags integration,fixtures ./...
```

`latest` does not continuously update installed tools.
Refresh it with `mise upgrade github:justtrackio/go-linters`, or add that command to the shared lint task.
A committed `mise.lock` can retain an older version.
New releases can introduce lint failures on unchanged branches.

## Rules

| Rule | Autofix support |
| --- | --- |
| `funcmethod` | No |
| `iferrinline` | **Yes**, when a safe fix can be generated |
| `methodfilecheck` | No |
| `multilineblock` | **Yes** |
| `noanonstruct` | No |
| `okinline` | **Yes**, when a safe fix can be generated |
| `recvaftertype` | No |

Apply supported fixes by running the custom binary with `--fix`:

```sh
cd /path/to/some/other/repo
/path/to/go-linters/bin/golangci-lint run --fix ./...
```

## Build and check the bundle

The root `mise.toml` selects the published `github:justtrackio/go-linters` release for normal local linting.
The bundle task deliberately uses the upstream Aqua runner instead, so a released bundle is never used to rebuild itself:

```sh
mise install
mise run test
mise run bundle
mise run test-bundle
```

`mise run bundle` installs `aqua:golangci/golangci-lint@2.15.0` and invokes the
binary returned by `mise which golangci-lint --tool aqua:golangci/golangci-lint@2.15.0`.
The build task does not resolve its runner through PATH.
`.custom-gcl.yml` pins the upstream version and imports this local checkout.
The custom binary is written to `./bin/golangci-lint`.
Analyzer tests use `analysistest` with checked-in Go fixtures and expected fix outputs.
The Go bundle integration test reuses these fixtures to check diagnostics and real `--fix` behavior.
It compiles the actual corrected packages and checks that only the expected
manual-fix diagnostics remain. Per-rule tests check the exact golden output.

For self-linting, run this command from the checkout:

```sh
mise exec -- golangci-lint run --build-tags integration,fixtures \
  --max-issues-per-linter=0 --max-same-issues=0 --uniq-by-line=false ./...
```

This includes integration tests and reports all findings. The repository's
`.golangci.yml` enables the company rules. Analyzer source and tests must pass
without suppressions or changes to the test fixtures.

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
Keep the upstream builder pin in the `mise.toml` bundle task, the embedded version in `.custom-gcl.yml`,
and the upstream license URL in the workflow aligned when upgrading golangci-lint.
Each archive includes the upstream runner's GPLv3 license as `LICENSE.golangci-lint`.

## funcmethod

**Autofix: No.**

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

## iferrinline

**Autofix: Yes**, when a safe fix can be generated (see [Autofix](#autofix)).

`iferrinline` flags the pattern

```go
err := foo()
if err != nil { ... }
```

when `err` isn't referenced after the `if`, and suggests inlining it as

```go
if err := foo(); err != nil { ... }
```

Error variables are recognized by their type, so names such as `convErr` and
`listErr` are handled as well. The check must refer to the same variable assigned
by the preceding statement. Non-error values are skipped, even if named `err`.

It also handles multi-return assignments. When the other variables are
unused after the `if`, the suggestion is the same inline form:

```go
x, err := bar()
if err != nil { ... }
// ↓
if x, err := bar(); err != nil { ... }
```

When a companion variable (e.g. `x`) is used after the `if`, the fix hoists the
variables to declarations at the top of the enclosing function and switches
the assignment to `=`:

```go
var x SomeType
var err error
// ...
if x, err = bar(); err != nil { ... }
use(x)
```

### Autofix

The analyzer attaches a `SuggestedFix` for the simple-inline case, so
running the custom golangci-lint binary with `--fix` rewrites the source in-place.
Example:

```sh
cd /path/to/some/other/repo
/path/to/go-linters/bin/golangci-lint run --fix ./...
```

New declarations retain the existing function-top hoisting, renames, imports,
and type rendering. Existing leading `var` declarations remain unchanged;
generated declarations follow them. Initializers, inferred types, grouped
declarations, comments, and evaluation order are preserved.

When an inferred result has a private type from another package, the existing
fix can use an exported interface that preserves its public methods. If no such
interface exists or the choice is ambiguous, no automatic fix is offered.

Both `iferrinline` and `okinline` use the same compiler-safety check. It applies
the proposed edits in memory and type-checks the selected package using its
loaded imports, language version, and target sizes. Edits that cannot be proven
compiler-safe are withheld; the diagnostic remains and says `manual fix required`.

A standalone plugin checks only its own rule's edits, so a disabled rule cannot
block its safe fixes. The `justtrack` aggregate shares one plan for both rules
and also checks their combined edits.

Fixes that share function-level declarations or import edits are checked as a
group. If the original group cannot compile safely, the group is left unchanged
rather than generating a different transformation.

Shared declaration and import edits preserve the original resulting source
bytes, but cover common ownership boundaries so the fixer cannot apply
conflicting standalone groups together. A deferred group can be applied on a
later pass. Run read-only lint after `--fix` and repeat as needed; a successful
`--fix` exit alone does not prove that every fix was applied.

Each plan also has a distinct root cache identity, so changing between the
aggregate and standalone plugins cannot reuse the other selection's fixes.

This check covers compilation, not semantic equivalence. Review the changes and
compile the affected production and test packages before committing them.

## methodfilecheck

**Autofix: No.**

Keeps receiver types and their methods in the same file. Within that file, the
first method must follow the type declaration with only constructors in between.
A constructor is any package function whose first result is the receiver type
or a pointer to it, regardless of the function's name or additional results.
Constructors in the type's file must appear before its first method.

```go
// Before: the constructor appears after the first method.
type Service struct{}

func (s *Service) Start() {}
func BuildService() (*Service, error) { return nil, nil } // reported
func (s *Service) Stop() {}
```

Move the constructor before the methods:

```go
type Service struct{}

func BuildService() (*Service, error) { return nil, nil }

func (s *Service) Start() {}
func (s *Service) Stop()  {}
```

Methods for one receiver must remain contiguous relative to other receivers'
methods: once another receiver's methods appear, returning to the earlier
receiver is reported. Package functions and other non-method declarations between
methods do not interrupt a receiver's method group.

Pointer, value, and generic receivers and grouped type declarations are supported.
Receiver aliases are checked under their declared alias name. The same-file
requirement is waived when either the type's file or the method's file contains a
`//go:build` directive; ordering and method grouping are still checked. Test files
are checked, and generated files are skipped. Only files selected by the current
build tags and platform participate in the check.

No automatic fix is offered. Move declarations to keep the type, constructors,
and method groups together.

Enable it after rebuilding the custom binary with `golangci-lint custom -v`:

```yaml
linters:
  enable:
    - methodfilecheck
  settings:
    custom:
      methodfilecheck:
        type: module
        description: keeps receiver types, constructors, and contiguous method blocks in the same file
        original-url: github.com/justtrackio/go-linters
```

## multilineblock

**Autofix: Yes.** Inserts the required blank lines.

Requires a blank line after blocks and before `if`,
`for` (including range), `switch` (including type switches), and `select` when
the preceding contiguous group of statements spans more than one source line.
This includes a single multiline assignment or several one-line statements.
One-line assignments may still touch the following block. Leading comments on
the first statement do not count toward the group's length. Existing blank lines
separate groups, and generated files are skipped.

```go
// Before: a multiline statement group touches the following control block.
name := "worker"
retries := 3
if name != "" { // reported: missing blank line before the block
    start(name, retries)
}
stop(name) // reported: missing blank line after the block
```

The automatic fix separates the group and the blocks:

```go
name := "worker"
retries := 3

if name != "" {
    start(name, retries)
}

stop(name)
```

When an assignment immediately precedes an `if` whose condition uses
one of the assigned variables, the assignment and check stay together. If earlier
statements make the group span multiple lines, the blank line is inserted before
the assignment instead. This includes multiple return values and compound
conditions such as `err == nil || status.Exists != nil`, including multiline
calls and assignments. Checks with an `if` initializer use the normal rule.

After-block spacing also covers standalone blocks and labeled control blocks.
No blank line is required before an enclosing closing brace or between `if`
and `else`. A deferred cleanup using a variable from the acquisition assignment
may immediately follow its `if err != nil` check. Other deferred calls require
the usual blank line.

The automatic fix inserts a blank line after the preceding statement, preserving
its trailing comment and keeping intervening comments attached to the block.
Rebuild the custom binary with `golangci-lint custom -v`, then enable:

```yaml
linters:
  enable:
    - multilineblock
  settings:
    custom:
      multilineblock:
        type: module
        description: requires blank lines after blocks and before control blocks following multiline statement groups
        original-url: github.com/justtrackio/go-linters
```

Run `golangci-lint run --fix` with the custom binary to insert the blank lines.

## noanonstruct

**Autofix: No.**

Requires a declared type name for non-empty structs in production Go files.
Checks literals, variable declarations, function signatures, and nested types.
Empty structs (`struct{}`) and all structs in `_test.go` files are allowed.
Direct struct type declarations (including aliases) are allowed; anonymous
structs nested inside them are still reported. No automatic fix is offered,
since naming and placing the new type requires a design decision.

```go
// Before: a non-empty anonymous struct in a production file.
var config struct { // reported: define a named type instead
    Name string
}
```

Declare a named type:

```go
type Config struct {
    Name string
}

var config Config
```

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

## okinline

**Autofix: Yes.** Shares the safe-fix limitations of `iferrinline`.

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

## recvaftertype

**Autofix: No.**

Requires methods to appear after their receiver type when that type is declared
in the same file. Go permits either order; this rule enforces declaration order
as a style convention. Pointer, value, and generic receivers are checked,
including types in grouped declarations and receiver aliases. Methods whose
receiver type is declared in another file are allowed. Test files are checked;
generated files are skipped.

```go
func (s *QueueService[T, F]) PurgeJobs() {} // reported

type QueueService[T any, F any] struct{}
```

Move the method and its comments below the type declaration to resolve the
diagnostic. No automatic fix is offered.

Enable it after rebuilding the custom binary with `golangci-lint custom -v`:

```yaml
linters:
  enable:
    - recvaftertype
  settings:
    custom:
      recvaftertype:
        type: module
        description: reports methods declared before their receiver type in the same file
        original-url: github.com/justtrackio/go-linters
```

## Develop

```sh
go mod tidy
go test ./...
```
