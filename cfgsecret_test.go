package linters

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestCfgSecret(t *testing.T) {
	constructor, err := register.GetPlugin("cfgsecret")
	require.NoError(t, err)
	plugin, err := constructor(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	results := analysistest.Run(t, testdataDir(t), analyzers[0], "testlintdata/cfgsecret")

	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			require.Empty(t, diagnostic.SuggestedFixes, "credential classification must not change source")
		}
	}
}
