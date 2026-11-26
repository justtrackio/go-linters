package linters

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestHTTPAuthz(t *testing.T) {
	plugin, err := NewHTTPAuthz(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	require.Len(t, analyzers, 1)
	analysistest.Run(t, testdataDir(t), analyzers[0], "testlintdata/httpauthz")
}
