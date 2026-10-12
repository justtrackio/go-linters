package linters

import (
	"testing"

	"github.com/golangci/plugin-module-register/register"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestRecvAfterType(t *testing.T) {
	constructor, err := register.GetPlugin("recvaftertype")
	require.NoError(t, err)
	plugin, err := constructor(nil)
	require.NoError(t, err)
	require.Equal(t, register.LoadModeSyntax, plugin.GetLoadMode())
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	require.Len(t, analyzers, 1)
	analysistest.Run(t, testdataDir(t), analyzers[0], "testlintdata/recvaftertype")
}
