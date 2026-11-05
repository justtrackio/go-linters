package linters

import (
	"go/ast"
	"go/types"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() { register.Plugin("gosologger", NewGosoLogger) }

// GosoLoggerPlugin validates Gosoline logging arguments and error directives.
type GosoLoggerPlugin struct{}

func NewGosoLogger(_ any) (register.LinterPlugin, error) { return &GosoLoggerPlugin{}, nil }

func (*GosoLoggerPlugin) GetLoadMode() string { return register.LoadModeTypesInfo }

func (*GosoLoggerPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "gosologger",
		Doc:  "checks Gosoline format types, forbids pointer arguments, and enforces error directives by level",
		Run:  runGosoLogger,
	}}, nil
}

type gosoLoggerCall struct {
	expr        *ast.CallExpr
	formatIndex int
	level       string
}

func runGosoLogger(pass *analysis.Pass) (any, error) {
	calls := make(map[*ast.File][]gosoLoggerCall)
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		ast.Inspect(file, func(node ast.Node) bool {
			if expr, ok := node.(*ast.CallExpr); ok {
				if call, ok := identifyGosoLoggerCall(pass, expr); ok {
					calls[file] = append(calls[file], call)
					checkGosoLoggerErrors(pass, call)
					checkGosoLoggerPointers(pass, call)
				}
			}

			return true
		})
	}

	return nil, checkGosoLoggerFormats(pass, calls)
}

func identifyGosoLoggerCall(pass *analysis.Pass, expr *ast.CallExpr) (gosoLoggerCall, bool) {
	var ok bool
	var selector *ast.SelectorExpr

	if selector, ok = ast.Unparen(expr.Fun).(*ast.SelectorExpr); !ok {
		return gosoLoggerCall{}, false
	}

	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() == types.FieldVal || selection.Obj().Pkg() == nil || selection.Obj().Pkg().Path() != "github.com/justtrackio/gosoline/pkg/log" {
		return gosoLoggerCall{}, false
	}

	switch selector.Sel.Name {
	case "Error", "Warn", "Info", "Debug", "Trace":
	default:
		return gosoLoggerCall{}, false
	}

	// A method expression has an explicit receiver before the context.
	formatIndex := 1
	if selection.Kind() == types.MethodExpr {
		formatIndex++
	}

	if len(expr.Args) <= formatIndex {
		return gosoLoggerCall{}, false
	}

	return gosoLoggerCall{expr: expr, formatIndex: formatIndex, level: selector.Sel.Name}, true
}
