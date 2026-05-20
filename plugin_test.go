package linters

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestIfErrInline(t *testing.T) {
	newPlugin, err := register.GetPlugin("iferrinline")
	require.NoError(t, err)

	plugin, err := newPlugin(nil)
	require.NoError(t, err)

	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)

	analysistest.RunWithSuggestedFixes(t, testdataDir(t), analyzers[0], "testlintdata/iferrinline")
}

func testdataDir(t *testing.T) string {
	t.Helper()
	_, testFilename, _, ok := runtime.Caller(1)
	if !ok {
		require.Fail(t, "unable to get current test filename")
	}
	return filepath.Join(filepath.Dir(testFilename), "testdata")
}
