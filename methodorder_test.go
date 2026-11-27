package linters

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestMethodOrder(t *testing.T) {
	constructor, err := register.GetPlugin("methodorder")
	require.NoError(t, err)
	plugin, err := constructor(nil)
	require.NoError(t, err)
	require.Equal(t, register.LoadModeTypesInfo, plugin.GetLoadMode())
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	require.Len(t, analyzers, 1)
	analysistest.Run(t, testdataDir(t), analyzers[0], "testlintdata/methodorder", "testlintdata/contextkeyplacement")
}

func TestSplitContextKeyPlacement(t *testing.T) {
	analyzer := &analysis.Analyzer{
		Name: "contextkeyplacement",
		Doc:  "checks that declaration and accessor placement agree",
		Run: func(pass *analysis.Pass) (any, error) {
			if _, err := runDeclFileCheck(pass); err != nil {
				return nil, err
			}

			return runMethodOrder(pass)
		},
	}

	analysistest.Run(t, testdataDir(t), analyzer, "testlintdata/contextkeyplacement/split")
}
