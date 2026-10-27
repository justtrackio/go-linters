package linters

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("methodorder", NewMethodOrder)
}

// MethodOrderPlugin orders receiver types, callers, and private helpers.
type MethodOrderPlugin struct{}

func (p *MethodOrderPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (p *MethodOrderPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "methodorder",
		Doc:  "reports methods before their receiver type and private helpers before their sole caller in the same file",
		Run:  runMethodOrder,
	}}, nil
}

func NewMethodOrder(_ any) (register.LinterPlugin, error) {
	return &MethodOrderPlugin{}, nil
}

func runMethodOrder(pass *analysis.Pass) (any, error) {
	var method *ast.FuncDecl
	var ok bool

	users := methodOrderUsers(pass)
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		typePositions := receiverTypePositions(file)

		for _, decl := range file.Decls {
			if method, ok = decl.(*ast.FuncDecl); !ok {
				continue
			}

			reportMethodOrderType(pass, method, typePositions)

			reportMethodOrderCaller(pass, file, method, users)
		}
	}

	return nil, nil
}

func reportMethodOrderType(pass *analysis.Pass, method *ast.FuncDecl, typePositions map[string]token.Pos) {
	if method.Recv == nil || len(method.Recv.List) != 1 {
		return
	}

	receiver := methodOrderName(method.Recv.List[0].Type)
	if receiver == nil {
		return
	}

	if pos, exists := typePositions[receiver.Name]; exists && method.Pos() < pos {
		pass.Reportf(method.Name.Pos(), "method %s must be declared after receiver type %s in the same file", method.Name.Name, receiver.Name)
	}
}

func reportMethodOrderCaller(pass *analysis.Pass, file *ast.File, method *ast.FuncDecl, users map[*types.Func]map[*ast.FuncDecl]bool) {
	obj, _ := pass.TypesInfo.Defs[method.Name].(*types.Func)
	if obj == nil || obj.Exported() {
		return
	}

	caller := methodOrderSoleCaller(pass, users, obj)
	if caller != nil && method.Pos() < caller.Pos() && file.Pos() <= caller.Pos() && caller.End() <= file.End() {
		kind := "function"
		if method.Recv != nil {
			kind = "method"
		}

		pass.Reportf(method.Name.Pos(), "%s %s is used only by %s; declare it after %s in the same file", kind, method.Name.Name, caller.Name.Name, caller.Name.Name)
	}
}

// Include generated files when counting users. Test references do not establish
// production ownership, but still count for helpers declared in test files.
func methodOrderUsers(pass *analysis.Pass) map[*types.Func]map[*ast.FuncDecl]bool {
	users := make(map[*types.Func]map[*ast.FuncDecl]bool)
	for _, file := range pass.Files {
		testFile := methodOrderTestFile(pass, file.Pos())
		for _, decl := range file.Decls {
			collectMethodOrderUsers(pass, decl, testFile, users)
		}
	}

	return users
}

func collectMethodOrderUsers(pass *analysis.Pass, decl ast.Decl, testFile bool, users map[*types.Func]map[*ast.FuncDecl]bool) {
	caller, _ := decl.(*ast.FuncDecl)
	ast.Inspect(decl, func(node ast.Node) bool {
		var id *ast.Ident
		var obj *types.Func
		var ok bool

		if id, ok = node.(*ast.Ident); !ok {
			return true
		}

		if obj, ok = pass.TypesInfo.Uses[id].(*types.Func); !ok {
			return true
		}

		obj = obj.Origin()
		if testFile && !methodOrderTestFile(pass, obj.Pos()) {
			return true
		}

		// Recursion does not introduce another user.
		if caller != nil && pass.TypesInfo.Defs[caller.Name] == obj {
			return true
		}

		if users[obj] == nil {
			users[obj] = make(map[*ast.FuncDecl]bool)
		}

		users[obj][caller] = true

		return true
	})
}

func methodOrderTestFile(pass *analysis.Pass, pos token.Pos) bool {
	return strings.HasSuffix(pass.Fset.PositionFor(pos, false).Filename, "_test.go")
}

// Cycles cannot satisfy caller-before-helper ordering, so leave them alone.
func methodOrderSoleCaller(pass *analysis.Pass, users map[*types.Func]map[*ast.FuncDecl]bool, obj *types.Func) *ast.FuncDecl {
	var first *ast.FuncDecl
	seen := make(map[*types.Func]bool)

	for obj != nil && len(users[obj]) == 1 {
		if seen[obj] {
			return nil
		}

		seen[obj] = true

		var caller *ast.FuncDecl
		for user := range users[obj] {
			caller = user
		}

		if caller == nil {
			break
		}

		if first == nil {
			first = caller
		}

		obj, _ = pass.TypesInfo.Defs[caller.Name].(*types.Func)
	}

	return first
}

func receiverTypePositions(file *ast.File) map[string]token.Pos {
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

	return types
}

func methodOrderName(expr ast.Expr) *ast.Ident {
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return expr
	case *ast.StarExpr:
		return methodOrderName(expr.X)
	case *ast.IndexExpr:
		return methodOrderName(expr.X)
	case *ast.IndexListExpr:
		return methodOrderName(expr.X)
	default:
		return nil
	}
}
