package linters

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("funcmethod", NewFuncMethod)
}

// FuncMethodPlugin finds private helpers owned by a single receiver type.
type FuncMethodPlugin struct{}

func (p *FuncMethodPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (p *FuncMethodPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "funcmethod",
		Doc:  "reports private functions used only by a single receiver type that should be methods",
		Run:  runFuncMethod,
	}}, nil
}

func NewFuncMethod(_ any) (register.LinterPlugin, error) {
	return &FuncMethodPlugin{}, nil
}

type methodOwnership struct {
	decl       *ast.FuncDecl
	generated  bool
	owners     map[*types.Named]bool
	callees    map[*methodOwnership]bool
	blocked    bool
	constructs bool
}

func runFuncMethod(pass *analysis.Pass) (any, error) {
	functions, ordered := collectMethodOwnership(pass)
	collectMethodCalls(pass, functions)
	propagateMethodOwnership(ordered)
	reportMethodOwnership(pass, ordered)

	return nil, nil
}

func collectMethodOwnership(pass *analysis.Pass) (map[*types.Func]*methodOwnership, []*methodOwnership) {
	var obj *types.Func

	functions := map[*types.Func]*methodOwnership{}
	var ordered []*methodOwnership

	for _, file := range pass.Files {
		if funcMethodTestFile(pass, file) {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			if obj, ok = pass.TypesInfo.Defs[fn.Name].(*types.Func); !ok {
				continue
			}

			state := &methodOwnership{
				decl: fn, generated: ast.IsGenerated(file),
				owners: map[*types.Named]bool{}, callees: map[*methodOwnership]bool{},
			}

			initializeMethodOwnership(pass, obj, state)

			functions[obj] = state
			ordered = append(ordered, state)
		}
	}

	return functions, ordered
}

func initializeMethodOwnership(pass *analysis.Pass, obj *types.Func, state *methodOwnership) {
	if recv := obj.Type().(*types.Signature).Recv(); recv != nil {
		if owner := funcMethodReceiver(recv.Type()); owner != nil {
			state.owners[owner] = true
		}
	} else if obj.Exported() || state.decl.Name.Name == "init" || (state.decl.Name.Name == "main" && pass.Pkg.Name() == "main") {
		// These are entry points whose callers cannot be inferred locally.
		state.blocked = true
	} else if funcMethodConstructs(pass, obj) {
		// Constructors belong to the type they create, not to a calling receiver.
		state.constructs = true
	}
}

func collectMethodCalls(pass *analysis.Pass, functions map[*types.Func]*methodOwnership) {
	for _, file := range pass.Files {
		if funcMethodTestFile(pass, file) {
			continue
		}

		for _, decl := range file.Decls {
			collectDeclarationMethodCalls(pass, decl, functions)
		}
	}
}

func collectDeclarationMethodCalls(pass *analysis.Pass, decl ast.Decl, functions map[*types.Func]*methodOwnership) {
	var caller *methodOwnership
	if fn, ok := decl.(*ast.FuncDecl); ok {
		obj, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
		caller = functions[obj]
	}

	// Identify direct calls separately from function values. Escaped
	// callbacks may run on behalf of callers outside this package.
	direct := directMethodCalls(decl)
	ast.Inspect(decl, func(node ast.Node) bool {
		var ok bool
		var id *ast.Ident

		if id, ok = node.(*ast.Ident); !ok {
			return true
		}

		obj, _ := pass.TypesInfo.Uses[id].(*types.Func)

		callee := functions[obj]
		if callee == nil || callee.decl.Recv != nil {
			return true
		}

		if caller == nil || !direct[id] {
			callee.blocked = true
		} else {
			caller.callees[callee] = true
		}

		return true
	})
}

func directMethodCalls(decl ast.Decl) map[*ast.Ident]bool {
	direct := map[*ast.Ident]bool{}
	ast.Inspect(decl, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if id := funcMethodCallee(call.Fun); id != nil {
				direct[id] = true
			}
		}

		return true
	})

	return direct
}

func propagateMethodOwnership(ordered []*methodOwnership) {
	// Propagate receiver owners and unrestricted entry points through helper
	// chains. Unreachable private functions (including test-only helpers) do
	// not establish production ownership. A fixed point handles recursion.
	for changed := true; changed; {
		changed = false
		for _, caller := range ordered {
			for callee := range caller.callees {
				if caller.blocked && !callee.blocked {
					callee.blocked = true
					changed = true
				}

				for owner := range caller.owners {
					if !callee.owners[owner] {
						callee.owners[owner] = true
						changed = true
					}
				}
			}
		}
	}
}

func reportMethodOwnership(pass *analysis.Pass, ordered []*methodOwnership) {
	for _, state := range ordered {
		if state.decl.Recv != nil || state.generated || state.blocked || state.constructs || len(state.owners) != 1 || state.decl.Type.TypeParams != nil {
			continue
		}

		for owner := range state.owners {
			// Avoid suggesting a method that collides with a field or method.
			if obj, _, _ := types.LookupFieldOrMethod(types.NewPointer(owner), true, pass.Pkg, state.decl.Name.Name); obj != nil {
				continue
			}

			pass.Reportf(state.decl.Name.Pos(), "function %s is used only by %s; make it a method of %s", state.decl.Name.Name, owner.Obj().Name(), owner.Obj().Name())
		}
	}
}

func funcMethodTestFile(pass *analysis.Pass, file *ast.File) bool {
	return strings.HasSuffix(pass.Fset.PositionFor(file.Pos(), false).Filename, "_test.go")
}

func funcMethodReceiver(typ types.Type) *types.Named {
	typ = types.Unalias(typ)
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(ptr.Elem())
	}

	if named, ok := typ.(*types.Named); ok {
		return named.Origin()
	}

	return nil
}

func constructorResult(obj *types.Func) *types.Named {
	results := obj.Type().(*types.Signature).Results()
	if results.Len() == 0 || results.Len() > 2 {
		return nil
	}

	errorType := types.Universe.Lookup("error").Type()
	if results.Len() == 2 && !types.Identical(results.At(1).Type(), errorType) {
		return nil
	}

	typ := types.Unalias(results.At(0).Type())
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(ptr.Elem())
	}

	named, ok := typ.(*types.Named)
	if !ok {
		return nil
	}

	return named
}

func funcMethodConstructs(pass *analysis.Pass, obj *types.Func) bool {
	named := constructorResult(obj)

	return named != nil && named.Obj().Pkg() == pass.Pkg
}

func funcMethodCallee(expr ast.Expr) *ast.Ident {
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return expr
	case *ast.IndexExpr:
		return funcMethodCallee(expr.X)
	case *ast.IndexListExpr:
		return funcMethodCallee(expr.X)
	default:
		return nil
	}
}
