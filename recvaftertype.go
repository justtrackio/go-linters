package linters

import (
	"go/ast"
	"go/token"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("recvaftertype", NewRecvAfterType)
}

// RecvAfterTypePlugin requires methods to follow their receiver type in the same file.
type RecvAfterTypePlugin struct{}

func NewRecvAfterType(_ any) (register.LinterPlugin, error) {
	return &RecvAfterTypePlugin{}, nil
}

func (p *RecvAfterTypePlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (p *RecvAfterTypePlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "recvaftertype",
		Doc:  "reports methods declared before their receiver type in the same file",
		Run:  runRecvAfterType,
	}}, nil
}

func runRecvAfterType(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		types := make(map[string]token.Pos)
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typ := spec.(*ast.TypeSpec)
				types[typ.Name.Name] = typ.Pos()
			}
		}

		for _, decl := range file.Decls {
			method, ok := decl.(*ast.FuncDecl)
			if !ok || method.Recv == nil || len(method.Recv.List) != 1 {
				continue
			}
			receiver := recvAfterTypeName(method.Recv.List[0].Type)
			if receiver == nil {
				continue
			}
			if pos, exists := types[receiver.Name]; exists && method.Pos() < pos {
				pass.Reportf(method.Name.Pos(), "method %s must be declared after receiver type %s in the same file", method.Name.Name, receiver.Name)
			}
		}
	}
	return nil, nil
}

func recvAfterTypeName(expr ast.Expr) *ast.Ident {
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return expr
	case *ast.StarExpr:
		return recvAfterTypeName(expr.X)
	case *ast.IndexExpr:
		return recvAfterTypeName(expr.X)
	case *ast.IndexListExpr:
		return recvAfterTypeName(expr.X)
	default:
		return nil
	}
}
