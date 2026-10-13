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
	plan := newInlineCompilePlanAnalyzer(inlineRule{"okinline", checkedOk})
	return []*analysis.Analyzer{newOkInlineAnalyzer(plan), plan}, nil
}

func newOkInlineAnalyzer(plan *analysis.Analyzer) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "okinline",
		Doc:      "reports ok assignments followed by `if !ok` or `if ok` that can be inlined into the if initializer",
		Run:      func(pass *analysis.Pass) (any, error) { return runInlineCheck(pass, "okinline", plan) },
		Requires: []*analysis.Analyzer{plan},
	}
}

func (p *OkInline) GetLoadMode() string {
	return register.LoadModeTypesInfo
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
