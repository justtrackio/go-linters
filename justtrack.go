package linters

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("justtrack", NewJustTrack)
}

type JustTrackPlugin struct{}

func (*JustTrackPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (*JustTrackPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	var plugin register.LinterPlugin
	var pluginAnalyzers []*analysis.Analyzer
	var err error

	constructors := []func(any) (register.LinterPlugin, error){
		NewCfgSecret,
		NewNoAnonStruct,
		NewFuncMethod,
		NewMethodFileCheck,
		NewMultilineBlock,
		NewRecvAfterType,
	}
	analyzers := make([]*analysis.Analyzer, 0, len(constructors)+2)

	for _, constructor := range constructors {
		if plugin, err = constructor(nil); err != nil {
			return nil, err
		}

		if pluginAnalyzers, err = plugin.BuildAnalyzers(); err != nil {
			return nil, err
		}

		analyzers = append(analyzers, pluginAnalyzers...)
	}

	plan := newInlineCompilePlanAnalyzer(inlineRule{"iferrinline", checkedError}, inlineRule{"okinline", checkedOk})
	analyzers = append(analyzers, newIfErrInlineAnalyzer(plan), newOkInlineAnalyzer(plan), plan)

	return analyzers, nil
}

func NewJustTrack(_ any) (register.LinterPlugin, error) {
	return &JustTrackPlugin{}, nil
}
