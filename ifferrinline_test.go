package linters

import (
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/packages"
)

func TestIfErrInline(t *testing.T) {
	newPlugin, err := register.GetPlugin("iferrinline")
	require.NoError(t, err)

	plugin, err := newPlugin(nil)
	require.NoError(t, err)

	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)

	analysistest.RunWithSuggestedFixes(t, testdataDir(t), analyzers[0], "testlintdata/iferrinline", "testlintdata/missingimport", "inlineprivate")
}

func TestInlineUnsafeHoistsDoNotSuggestFix(t *testing.T) {
	var err error
	var loaded []*packages.Package

	testdata := testdataDir(t)
	config := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedImports | packages.NeedDeps,
		Dir:  filepath.Join(testdata, "src", "testlintdata", "inlineunsafe"),
		Env:  append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOPATH="+testdata),
	}
	if loaded, err = packages.Load(config, "testlintdata/inlineunsafe"); err != nil {
		t.Fatal(err)
	}
	require.Len(t, loaded, 1)
	require.Empty(t, loaded[0].Errors)

	ifErrPlugin, err := NewIfErrInline(nil)
	require.NoError(t, err)
	ifErrAnalyzers, err := ifErrPlugin.BuildAnalyzers()
	require.NoError(t, err)

	okPlugin, err := NewOkInline(nil)
	require.NoError(t, err)
	okAnalyzers, err := okPlugin.BuildAnalyzers()
	require.NoError(t, err)

	for i, analyzer := range []*analysis.Analyzer{ifErrAnalyzers[0], okAnalyzers[0]} {
		var diagnostics []analysis.Diagnostic
		_, err = analyzer.Run(&analysis.Pass{
			Fset: loaded[0].Fset, Files: loaded[0].Syntax, Pkg: loaded[0].Types, TypesInfo: loaded[0].TypesInfo,
			ReadFile: os.ReadFile,
			Report:   func(diagnostic analysis.Diagnostic) { diagnostics = append(diagnostics, diagnostic) },
		})
		require.NoError(t, err)
		expected := 8
		if i == 1 {
			expected = 3
		}
		require.Len(t, diagnostics, expected, analyzer.Name)
		for _, diagnostic := range diagnostics {
			require.Empty(t, diagnostic.SuggestedFixes, "%s should withhold unsafe fixes: %s", analyzer.Name, diagnostic.Message)
		}
	}
}

func testdataDir(t *testing.T) string {
	var ok bool
	var testFilename string

	t.Helper()

	if _, testFilename, _, ok = runtime.Caller(1); !ok {
		require.Fail(t, "unable to get current test filename")
	}

	return filepath.Join(filepath.Dir(testFilename), "testdata")
}

// Suggested-fix comparisons alone do not type-check the rewritten source.
func TestIfErrInlineFixedCodeCompiles(t *testing.T) {
	testdata := testdataDir(t)
	gopath := t.TempDir()
	sourceRoot := filepath.Join(gopath, "src")
	fixtures := []struct {
		source      string
		destination string
	}{
		{
			source:      filepath.Join(testdata, "src", "testlintdata", "iferrinline"),
			destination: filepath.Join(sourceRoot, "testlintdata", "iferrinline"),
		},
		{
			source:      filepath.Join(testdata, "src", "inlineprivate"),
			destination: filepath.Join(sourceRoot, "inlineprivate"),
		},
	}
	for _, fixture := range fixtures {
		copyAutofixedFixture(t, fixture.source, fixture.destination)
	}

	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
		Dir:  sourceRoot,
		Env:  append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOPATH="+gopath),
	}, "testlintdata/iferrinline", "inlineprivate")
	require.NoError(t, err)
	require.Len(t, pkgs, len(fixtures))
	for _, pkg := range pkgs {
		require.Empty(t, pkg.Errors, pkg.PkgPath)
	}
}

func copyAutofixedFixture(t *testing.T, source, destination string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		var rel string
		var data []byte

		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if rel, err = filepath.Rel(source, path); err != nil {
			return err
		}
		if _, err := os.Stat(path + ".golden"); err == nil {
			path += ".golden"
		}
		if data, err = os.ReadFile(path); err != nil {
			return err
		}

		target := filepath.Join(destination, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	require.NoError(t, err)
}

func TestImportEditsPreserveGroups(t *testing.T) {
	const source = `package clone

import (
	"bufio"
	"fmt"

	"github.com/charmbracelet/lipgloss" // Keep the comment.
	"github.com/xanzy/go-gitlab"

	"gitlab.justtrack.io/justtrack/devops/tools/poly-repo-helper/internal/cfg"
)
`
	const expected = `package clone

import (
	"bufio"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss" // Keep the comment.
	"github.com/go-git/go-git/v5/plumbing/transport"
	transport2 "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/xanzy/go-gitlab"

	"gitlab.justtrack.io/justtrack/devops/tools/poly-repo-helper/internal/cfg"
)
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "clone.go", source, parser.ParseComments)
	require.NoError(t, err)
	original := buildImportMap(file)
	updated := buildImportMap(file)
	updated["os"] = ""
	updated["github.com/go-git/go-git/v5/plumbing/transport"] = ""
	updated["github.com/go-git/go-git/v5/plumbing/transport/http"] = "transport2"
	edits := buildImportEdits(file, original, updated)
	fixed := source

	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		start, end := fset.Position(edit.Pos).Offset, fset.Position(edit.End).Offset
		fixed = fixed[:start] + string(edit.NewText) + fixed[end:]
	}

	formatted, err := format.Source([]byte(fixed))
	require.NoError(t, err)
	require.Equal(t, expected, string(formatted))
}
