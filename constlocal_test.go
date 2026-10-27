package linters

import (
	"os"
	"strings"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

func TestConstLocal(t *testing.T) {
	constructor, err := register.GetPlugin("constlocal")
	require.NoError(t, err)
	plugin, err := constructor(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	analysistest.Run(t, testdataDir(t), analyzers[0], "testlintdata/constlocal")
}

func TestConstLocalTestReferences(t *testing.T) {
	plugin, err := NewConstLocal(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	dir := testdataDir(t)
	pkgs, err := packages.Load(&packages.Config{
		Mode:  packages.LoadAllSyntax,
		Dir:   dir,
		Tests: true,
		Env:   append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOPATH="+dir),
	}, "testlintdata/constlocaltests")
	require.NoError(t, err)
	graph, err := checker.Analyze(analyzers, pkgs, nil)
	require.NoError(t, err)
	checkedTests := false

	for _, action := range graph.Roots {
		require.NoError(t, action.Err)
		if strings.Contains(action.Package.ID, "constlocaltests [") {
			checkedTests = true
			require.Empty(t, action.Diagnostics, "test references must count as another function")
		} else if action.Package.PkgPath == "testlintdata/constlocaltests" {
			require.Len(t, action.Diagnostics, 1, "the production variant cannot see test references")
		}
	}

	require.True(t, checkedTests)
}
