package linters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

// Run the real suggested edits, rather than a hand-written approximation of the
// hoist. Type checking alone cannot catch discarded, well-typed initializers.
func TestInlineHoistsPreserveInitializedDeclarations(t *testing.T) {
	for _, test := range []struct{ rule, name, newline string }{
		{"iferrinline", "LF", "\n"},
		{"iferrinline", "CRLF", "\r\n"},
		{"okinline", "LF", "\n"},
		{"okinline", "CRLF", "\r\n"},
	} {
		t.Run(test.rule+"/"+test.name, func(t *testing.T) {
			rule := test.rule
			source := strings.ReplaceAll(inlineHoistPreservationProgram(rule), "\n", test.newline)
			originalOutput := runInlineHoistProgram(t, source)
			require.Equal(t, "base 1\noverride 1\nfirst\nsecond\norder 1 2 3\ninferred 5 8 13 1\ntuple-left\ntuple-right\ntuple 3 4 1\nshadow 41 41\nsaved true true 1\ncomments 9 10 1\n", originalOutput)

			for _, mode := range []string{"standalone", "aggregate"} {
				t.Run(mode, func(t *testing.T) {
					pass := inlineTestPass(t, map[string]string{"fixture.go": source})
					var analyzers []*analysis.Analyzer
					var err error

					if mode == "aggregate" {
						analyzers, err = (&JustTrackPlugin{}).BuildAnalyzers()
					} else if rule == "iferrinline" {
						analyzers, err = (&IfErrInline{}).BuildAnalyzers()
					} else {
						analyzers, err = (&OkInline{}).BuildAnalyzers()
					}

					require.NoError(t, err)
					var selected *analysis.Analyzer

					for _, analyzer := range analyzers {
						if analyzer.Name == rule {
							selected = analyzer
							break
						}
					}

					require.NotNil(t, selected)
					diagnostics := runInlineTestAnalyzer(t, pass, selected)
					require.Len(t, diagnostics, 7, "every safe function must receive a fix")
					var edits []analysis.TextEdit

					for _, diagnostic := range diagnostics {
						require.Len(t, diagnostic.SuggestedFixes, 1)
						require.NotEmpty(t, diagnostic.SuggestedFixes[0].TextEdits)
						edits = append(edits, diagnostic.SuggestedFixes[0].TextEdits...)
					}

					fixed := inlineTestEditedSources(t, pass, edits)["fixture.go"]
					require.NotEqual(t, source, fixed)
					require.Equal(t, originalOutput, runInlineHoistProgram(t, fixed))
					// These comments straddle the insertion boundary. Their exact
					// preservation accompanies the compilation and runtime check.
					require.Contains(t, fixed, "var original = 9 // initializer-owned trailing comment")
					require.Contains(t, fixed, strings.ReplaceAll("/* declaration-owned\n       multiline comment */", "\n", test.newline))
					require.Contains(t, fixed, strings.ReplaceAll("var following = original + 1 /* initializer-owned\n       multiline trailing comment */", "\n", test.newline))
					require.Contains(t, fixed, "// candidate-owned leading comment")
				})
			}
		})
	}
}

func runInlineHoistProgram(t *testing.T, source string) string {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "fixture.go"), []byte(source), 0o600))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "fixture.go")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "GO111MODULE=off", "GOPROXY=off", "GOTOOLCHAIN=local", "GOFLAGS=")
	output, err := command.CombinedOutput()
	require.NoError(t, err, "program failed to compile or execute:\n%s\nsource:\n%s", output, source)
	return string(output)
}

func inlineHoistPreservationProgram(rule string) string {
	checked, condition, resultType, result := "err", "err != nil", "error", "nil"

	if rule == "okinline" {
		checked, condition, resultType, result = "ok", "!ok", "bool", "true"
	}

	return strings.NewReplacer("CHECKED", checked, "CONDITION", condition, "RESULTTYPE", resultType, "RESULT", result).Replace(`package main

type renderer interface { render(int) }
type base struct{}
type overrideRenderer struct{}
func (*base) render(value int) { println("base", value) }
func (*overrideRenderer) render(value int) { println("override", value) }
func fetch(value int) (int, RESULTTYPE) { return value, RESULT }
func mark(label string, value int) int { println(label); return value }
func pair() (int, int) { return mark("tuple-left", 3), mark("tuple-right", 4) }
var outer = 40

func initializedInterface(override bool) {
    var selected renderer = &base{}
    if override { selected = &overrideRenderer{} }
    current, CHECKED := fetch(1)
    if CONDITION { return }
    selected.render(current)
}

func initializerOrder() {
    var first = mark("first", 1)
    var second = mark("second", first + 1)
    current, CHECKED := fetch(first + second)
    if CONDITION { return }
    println("order", first, second, current)
}

func inferredGroupedAndMulti() {
    var (
        first = 5
        second int = first + 3
        left, right = first, second
    )
    current, CHECKED := fetch(1)
    if CONDITION { return }
    println("inferred", first, second, left + right, current)
}

func tupleInitializer() {
    var left, right = pair()
    current, CHECKED := fetch(1)
    if CONDITION { return }
    println("tuple", left, right, current)
}

func shadowingInitializer() {
    var outer = outer + 1
    current, CHECKED := fetch(outer)
    if CONDITION { return }
    println("shadow", outer, current)
}

func savedInitializerBindings() {
    var err error
    var savedErr = err
    var ok = true
    var savedOk = ok
    current, CHECKED := fetch(1)
    if CONDITION { return }
    println("saved", savedErr == nil, savedOk, current)
}

func commentedBoundary() {
    /* declaration-owned
       multiline comment */
    var original = 9 // initializer-owned trailing comment
    var following = original + 1 /* initializer-owned
       multiline trailing comment */
    // candidate-owned leading comment
    current, CHECKED := fetch(1)
    if CONDITION { return }
    println("comments", original, following, current)
}

func main() {
    initializedInterface(false)
    initializedInterface(true)
    initializerOrder()
    inferredGroupedAndMulti()
    tupleInitializer()
    shadowingInitializer()
    savedInitializerBindings()
    commentedBoundary()
}
`)
}
