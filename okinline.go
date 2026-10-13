package linters

import (
	"go/ast"
	"go/token"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("okinline", NewOkInline)
}

type OkInline struct{}

func (p *OkInline) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name:     "okinline",
		Doc:      "reports ok assignments followed by `if !ok` or `if ok` that can be inlined into the if initializer",
		Run:      p.run,
		Requires: []*analysis.Analyzer{inlineCompilePlanAnalyzer},
	}}, nil
}

func (p *OkInline) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (p *OkInline) run(pass *analysis.Pass) (any, error) {
	return runInlineCheck(pass, "okinline", checkedOk)
}

func NewOkInline(_ any) (register.LinterPlugin, error) {
	return &OkInline{}, nil
}

func checkedOk(_ *analysis.Pass, expr ast.Expr) *ast.Ident {
	expr = ast.Unparen(expr)
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
		expr = ast.Unparen(unary.X)
	}

	id, ok := expr.(*ast.Ident)
	if !ok || id.Name != "ok" {
		return nil
	}

	return id
}
