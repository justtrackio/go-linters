package linters

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestDeclFileCheck(t *testing.T) {
	plugin, err := NewDeclFileCheck(nil)
	require.NoError(t, err)
	analyzers, err := plugin.BuildAnalyzers()
	require.NoError(t, err)
	analysistest.Run(t, testdataDir(t), analyzers[0], "testlintdata/declfilecheck", "testlintdata/declfilecollection")
}
