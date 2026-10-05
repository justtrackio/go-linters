package linters

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("justtrack", NewJustTrack)
}

type JustTrackPlugin struct{}

func NewJustTrack(_ any) (register.LinterPlugin, error) {
	return &JustTrackPlugin{}, nil
}

func (*JustTrackPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (*JustTrackPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	noAnonStruct, err := NewNoAnonStruct(nil)
	if err != nil {
		return nil, err
	}
	noAnonStructAnalyzers, err := noAnonStruct.BuildAnalyzers()
	if err != nil {
		return nil, err
	}

	ifErrInline, err := New(nil)
	if err != nil {
		return nil, err
	}
	ifErrInlineAnalyzers, err := ifErrInline.BuildAnalyzers()
	if err != nil {
		return nil, err
	}

	analyzers := make([]*analysis.Analyzer, 0, len(noAnonStructAnalyzers)+len(ifErrInlineAnalyzers))
	analyzers = append(analyzers, noAnonStructAnalyzers...)
	analyzers = append(analyzers, ifErrInlineAnalyzers...)

	return analyzers, nil
}
