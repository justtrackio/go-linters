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

	analysistest.RunWithSuggestedFixes(t, testdataDir(t), analyzers[0], "testlintdata/iferrinline", "testlintdata/missingimport")
}

func testdataDir(t *testing.T) string {
	t.Helper()
	_, testFilename, _, ok := runtime.Caller(1)
	if !ok {
		require.Fail(t, "unable to get current test filename")
	}
	return filepath.Join(filepath.Dir(testFilename), "testdata")
}

// Suggested-fix comparisons alone do not type-check the rewritten source.
func TestIfErrInlineFixedCodeCompiles(t *testing.T) {
	source := filepath.Join(testdataDir(t), "src", "testlintdata", "iferrinline")
	gopath := t.TempDir()
	destination := filepath.Join(gopath, "src", "testlintdata", "iferrinline")
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path + ".golden"); err == nil {
			path += ".golden"
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	require.NoError(t, err)
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps,
		Dir:  destination,
		Env:  append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOPATH="+gopath),
	}, "testlintdata/iferrinline")
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	require.Empty(t, pkgs[0].Errors)
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
