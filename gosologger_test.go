package linters

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestGosoLogger(t *testing.T) {
	constructor, err := register.GetPlugin("gosologger")
	require.NoError(t, err)
	plugin, err := constructor(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	analysistest.RunWithSuggestedFixes(t, testdataDir(t), analyzers[0], "testlintdata/gosologger/...")
}
