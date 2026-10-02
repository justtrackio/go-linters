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

func NewFuncMethod(_ any) (register.LinterPlugin, error) {
	return &FuncMethodPlugin{}, nil
}

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

type methodOwnership struct {
	decl      *ast.FuncDecl
	generated bool
	owners    map[*types.Named]bool
	callees   map[*methodOwnership]bool
	blocked   bool
}

func runFuncMethod(pass *analysis.Pass) (any, error) {
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
			obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			state := &methodOwnership{
				decl: fn, generated: ast.IsGenerated(file),
				owners: map[*types.Named]bool{}, callees: map[*methodOwnership]bool{},
			}
			if recv := obj.Type().(*types.Signature).Recv(); recv != nil {
				if owner := funcMethodReceiver(recv.Type()); owner != nil {
					state.owners[owner] = true
				}
			} else if obj.Exported() || fn.Name.Name == "init" || (fn.Name.Name == "main" && pass.Pkg.Name() == "main") {
				// These are entry points whose callers cannot be inferred locally.
				state.blocked = true
			}
			functions[obj] = state
			ordered = append(ordered, state)
		}
	}

	for _, file := range pass.Files {
		if funcMethodTestFile(pass, file) {
			continue
		}
		for _, decl := range file.Decls {
			var caller *methodOwnership
			if fn, ok := decl.(*ast.FuncDecl); ok {
				obj, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
				caller = functions[obj]
			}
			// Identify direct calls separately from function values. Escaped
			// callbacks may run on behalf of callers outside this package.
			direct := map[*ast.Ident]bool{}
			ast.Inspect(decl, func(node ast.Node) bool {
				if call, ok := node.(*ast.CallExpr); ok {
					if id := funcMethodCallee(call.Fun); id != nil {
						direct[id] = true
					}
				}
				return true
			})
			ast.Inspect(decl, func(node ast.Node) bool {
				id, ok := node.(*ast.Ident)
				if !ok {
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
	}

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
	for _, state := range ordered {
		if state.decl.Recv != nil || state.generated || state.blocked || len(state.owners) != 1 || state.decl.Type.TypeParams != nil {
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
	return nil, nil
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
