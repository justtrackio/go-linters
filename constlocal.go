package linters

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("constlocal", NewConstLocal)
}

// ConstLocalPlugin keeps private constants with their sole function owner.
type ConstLocalPlugin struct{}

func (*ConstLocalPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (*ConstLocalPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "constlocal",
		Doc:  "reports function-owned package constants and local constants used once",
		Run:  runConstLocal,
	}}, nil
}

func NewConstLocal(_ any) (register.LinterPlugin, error) {
	return &ConstLocalPlugin{}, nil
}

type localConstant struct {
	name         *ast.Ident
	owner        *ast.FuncDecl
	blocked      bool
	local        bool
	uses         int
	multiline    bool
	callArgument bool
}

func runConstLocal(pass *analysis.Pass) (any, error) {
	constants, ordered := collectLocalConstants(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			collectLocalConstantUses(pass, file, decl, constants)
		}
	}

	for _, constant := range ordered {
		switch {
		case constant.local && constant.uses == 1 && !constant.callArgument:
			pass.Reportf(constant.name.Pos(), "local constant %s is used once; inline its value", constant.name.Name)
		case !constant.local && !constant.blocked && constant.owner != nil:
			pass.Reportf(constant.name.Pos(), "constant %s is used only by function %s; move it into that function", constant.name.Name, constant.owner.Name.Name)
		}
	}

	return nil, nil
}

func collectLocalConstantUses(pass *analysis.Pass, file *ast.File, decl ast.Decl, constants map[types.Object]*localConstant) {
	owner, _ := decl.(*ast.FuncDecl)
	ast.PreorderStack(decl, nil, func(node ast.Node, stack []ast.Node) bool {
		var id *ast.Ident
		var ok bool

		if id, ok = node.(*ast.Ident); !ok {
			return true
		}

		constant := constants[pass.TypesInfo.Uses[id]]
		if constant == nil {
			return true
		}

		constant.uses++
		if constant.multiline && localConstantCallArgument(id, stack) {
			constant.callArgument = true
		}

		switch {
		case owner == nil || owner.Body == nil || id.Pos() < owner.Body.Pos() || ast.IsGenerated(file):
			constant.blocked = true
		case constant.owner != nil && constant.owner != owner:
			constant.blocked = true
		default:
			constant.owner = owner
		}

		return true
	})
}

func localConstantCallArgument(id *ast.Ident, stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		switch node := stack[i].(type) {
		case *ast.CallExpr:
			return id.Pos() > node.Lparen && id.End() <= node.Rparen
		case ast.Stmt, *ast.FuncLit:
			return false
		}
	}

	return false
}

func collectLocalConstants(pass *analysis.Pass) (map[types.Object]*localConstant, []*localConstant) {
	constants := make(map[types.Object]*localConstant)
	var ordered []*localConstant

	for _, file := range pass.Files {
		if ast.IsGenerated(file) || declarationFileGuarded(pass, file) {
			continue
		}

		ast.Inspect(file, func(node ast.Node) bool {
			group, ok := node.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST || constantBlockDependsOnPosition(pass, group) {
				return true
			}

			for _, constant := range localConstantValues(pass, group) {
				object := pass.TypesInfo.Defs[constant.name]

				local := object.Parent() != pass.Pkg.Scope()
				if !local && constant.name.IsExported() {
					continue
				}

				constant.local = local
				constants[object] = constant
				ordered = append(ordered, constant)
			}

			return false
		})
	}

	return constants, ordered
}

func localConstantValues(pass *analysis.Pass, group *ast.GenDecl) []*localConstant {
	var constants []*localConstant
	for _, spec := range group.Specs {
		value := spec.(*ast.ValueSpec)
		if value.Type != nil {
			continue
		}

		for i, name := range value.Names {
			if name.Name != "_" {
				expr := value.Values[i]
				constants = append(constants, &localConstant{
					name:      name,
					multiline: pass.Fset.PositionFor(expr.Pos(), false).Line != pass.Fset.PositionFor(expr.End(), false).Line,
				})
			}
		}
	}

	return constants
}
