package linters

import (
	"go/ast"
	"go/token"
	"go/types"
	"maps"

	"golang.org/x/tools/go/analysis"
)

type contextKeyGroup struct {
	key         *types.TypeName
	file        *ast.File
	declaration *ast.GenDecl
	accessors   []*ast.FuncDecl
	hasReader   bool
	hasWriter   bool
}

func collectContextKeyAccessors(pass *analysis.Pass) map[*ast.FuncDecl]*contextKeyGroup {
	accessors := make(map[*ast.FuncDecl]*contextKeyGroup)
	for _, group := range collectContextKeyGroups(pass) {
		for _, accessor := range group.accessors {
			accessors[accessor] = group
		}
	}

	return accessors
}

func collectContextKeyGroups(pass *analysis.Pass) map[*types.TypeName]*contextKeyGroup {
	var contextPackage *types.Package
	var fn *ast.FuncDecl
	var ok bool

	for _, imported := range pass.Pkg.Imports() {
		if imported.Path() == "context" {
			contextPackage = imported

			break
		}
	}

	if contextPackage == nil {
		return nil
	}

	groups := contextKeyDeclarations(pass)
	if len(groups) == 0 {
		return nil
	}

	ctx := contextPackage.Scope().Lookup("Context")
	read, _, _ := types.LookupFieldOrMethod(ctx.Type(), false, contextPackage, "Value")
	write := contextPackage.Scope().Lookup("WithValue")
	guarded := make(map[*ast.File]bool)

	for _, file := range pass.Files {
		guarded[file] = declarationFileGuarded(pass, file)
	}

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			if fn, ok = decl.(*ast.FuncDecl); !ok {
				continue
			}

			key, writer := contextAccessorKey(pass, fn, read, write)

			group := groups[key]
			if group == nil || inTestFile(pass, file) != inTestFile(pass, group.file) || (file != group.file && (guarded[file] || guarded[group.file])) {
				continue
			}

			group.accessors = append(group.accessors, fn)
			group.hasReader = group.hasReader || !writer
			group.hasWriter = group.hasWriter || writer
		}
	}

	maps.DeleteFunc(groups, func(_ *types.TypeName, group *contextKeyGroup) bool {
		return !group.hasReader || !group.hasWriter
	})

	return groups
}

func contextKeyDeclarations(pass *analysis.Pass) map[*types.TypeName]*contextKeyGroup {
	groups := make(map[*types.TypeName]*contextKeyGroup)
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}

			for _, spec := range group.Specs {
				key, ok := pass.TypesInfo.Defs[spec.(*ast.TypeSpec).Name].(*types.TypeName)
				if !ok || key.Exported() || key.IsAlias() {
					continue
				}

				named := key.Type().(*types.Named)
				if underlying, ok := named.Underlying().(*types.Struct); ok && named.TypeParams().Len() == 0 && underlying.NumFields() == 0 {
					groups[key] = &contextKeyGroup{
						key:         key,
						file:        file,
						declaration: group,
					}
				}
			}
		}
	}

	return groups
}

func contextAccessorKey(pass *analysis.Pass, fn *ast.FuncDecl, read, write types.Object) (*types.TypeName, bool) {
	var call *ast.CallExpr
	var ok bool

	result := contextAccessorResult(pass, fn)

	value, asserted := result.(*ast.TypeAssertExpr)
	if asserted {
		result = ast.Unparen(value.X)
	}

	if call, ok = result.(*ast.CallExpr); !ok {
		return nil, false
	}

	var callee *ast.Ident
	switch expr := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		callee = expr
	case *ast.SelectorExpr:
		callee = expr.Sel
		if selection := pass.TypesInfo.Selections[expr]; selection != nil && selection.Obj() == read {
			switch {
			case selection.Kind() == types.MethodVal && len(call.Args) == 1:
				return contextKeyLiteral(pass, call.Args[0]), false
			case selection.Kind() == types.MethodExpr && len(call.Args) == 2:
				return contextKeyLiteral(pass, call.Args[1]), false
			}
		}
	}

	if !asserted && callee != nil && pass.TypesInfo.Uses[callee] == write && len(call.Args) == 3 {
		return contextKeyLiteral(pass, call.Args[1]), true
	}

	return nil, false
}

func contextKeyLiteral(pass *analysis.Pass, expr ast.Expr) *types.TypeName {
	expr = ast.Unparen(expr)
	if _, ok := expr.(*ast.CompositeLit); !ok {
		return nil
	}

	if named, ok := types.Unalias(pass.TypesInfo.TypeOf(expr)).(*types.Named); ok {
		return named.Obj()
	}

	return nil
}

// An accessor returns the context operation directly or assigns it to the results it returns.
func contextAccessorResult(pass *analysis.Pass, fn *ast.FuncDecl) ast.Expr {
	var ret *ast.ReturnStmt
	var ok bool

	if fn.Recv != nil || fn.Body == nil || len(fn.Body.List) == 0 || len(fn.Body.List) > 2 {
		return nil
	}

	statements := fn.Body.List
	if ret, ok = statements[len(statements)-1].(*ast.ReturnStmt); !ok {
		return nil
	}

	if len(statements) == 1 {
		if len(ret.Results) == 1 {
			return ast.Unparen(ret.Results[0])
		}

		return nil
	}

	assign, ok := statements[0].(*ast.AssignStmt)
	if !ok || (assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE) || len(assign.Rhs) != 1 || !contextAccessorReturnsAssignment(pass, fn, assign, ret) {
		return nil
	}

	return ast.Unparen(assign.Rhs[0])
}

func contextAccessorReturnsAssignment(pass *analysis.Pass, fn *ast.FuncDecl, assign *ast.AssignStmt, ret *ast.ReturnStmt) bool {
	var returned *ast.Ident
	var ok bool

	signature, ok := pass.TypesInfo.TypeOf(fn.Name).(*types.Signature)
	if !ok || len(assign.Lhs) != signature.Results().Len() || (len(ret.Results) != 0 && len(ret.Results) != len(assign.Lhs)) {
		return false
	}

	for i, lhs := range assign.Lhs {
		ident, ok := ast.Unparen(lhs).(*ast.Ident)
		if !ok || ident.Name == "_" {
			return false
		}

		var expected types.Object = signature.Results().At(i)
		if len(ret.Results) > 0 {
			if returned, ok = ast.Unparen(ret.Results[i]).(*ast.Ident); !ok {
				return false
			}

			expected = pass.TypesInfo.ObjectOf(returned)
		}

		if expected == nil || pass.TypesInfo.ObjectOf(ident) != expected {
			return false
		}
	}

	return true
}
