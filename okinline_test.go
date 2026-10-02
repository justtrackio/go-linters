package linters

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestOkInline(t *testing.T) {
	newPlugin, err := register.GetPlugin("okinline")
	require.NoError(t, err)
	plugin, err := newPlugin(nil)
	require.NoError(t, err)
	require.Equal(t, register.LoadModeTypesInfo, plugin.GetLoadMode())
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	require.Len(t, analyzers, 1)
	require.Equal(t, "okinline", analyzers[0].Name)

	analysistest.RunWithSuggestedFixes(t, testdataDir(t), analyzers[0], "testlintdata/okinline")
}
