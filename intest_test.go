package linters

import (
	"go/ast"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

func TestInTest(t *testing.T) {
	constructor, err := register.GetPlugin("intest")
	require.NoError(t, err)
	plugin, err := constructor(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)

	for _, tests := range []bool{true, false} {
		name := "without tests"
		if tests {
			name = "with tests"
		}

		t.Run(name, func(t *testing.T) {
			dir := testdataDir(t)
			pkgs, err := packages.Load(&packages.Config{
				Mode:  packages.LoadAllSyntax,
				Dir:   dir,
				Tests: tests,
				Env:   append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOPATH="+dir),
			}, "testlintdata/intest", "testlintdata/intestnotests")
			require.NoError(t, err)
			require.NotEmpty(t, pkgs)
			graph, err := checker.Analyze(analyzers, pkgs, nil)
			require.NoError(t, err)
			checkedTests := false

			// The production variant must stay quiet; only the augmented package
			// has the test references needed to report these declarations.
			for _, action := range graph.Roots {
				require.NoError(t, action.Err)
				if strings.Contains(action.Package.ID, "testlintdata/intest [") {
					checkedTests = true
				}

				checkInTestDiagnostics(t, action)
			}

			require.Equal(t, tests, checkedTests, "must check the package variant containing test files")
		})
	}
}

func checkInTestDiagnostics(t *testing.T, action *checker.Action) {
	t.Helper()
	var expected []token.Pos
	var actual []token.Pos

	if strings.Contains(action.Package.ID, "testlintdata/intest [") {
		for _, file := range action.Package.Syntax {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && inTestExpectedFunction(fn.Name.Name) {
					expected = append(expected, fn.Name.Pos())
				}
			}
		}

		require.Len(t, expected, 7)
	}

	for _, diagnostic := range action.Diagnostics {
		actual = append(actual, diagnostic.Pos)
	}

	require.ElementsMatch(t, expected, actual, "unexpected diagnostics in %s", action.Package.ID)
}

func inTestExpectedFunction(name string) bool {
	switch name {
	case "missingImageReferences", "testChain", "testLeaf", "recursiveA", "recursiveB", "generic", "callback":
		return true
	default:
		return false
	}
}
