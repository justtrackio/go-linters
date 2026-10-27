package linters

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("intest", NewInTest)
}

// InTestPlugin finds private production functions reachable only from tests.
type InTestPlugin struct{}

func (*InTestPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (*InTestPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "intest",
		Doc:  "reports private production functions used only by tests",
		Run:  runInTest,
	}}, nil
}

func NewInTest(_ any) (register.LinterPlugin, error) {
	return &InTestPlugin{}, nil
}

type inTestFunction struct {
	decl       *ast.FuncDecl
	production bool
	test       bool
	callees    map[*inTestFunction]bool
}

func runInTest(pass *analysis.Pass) (any, error) {
	functions, ordered := collectInTestFunctions(pass)
	collectInTestReferences(pass, functions)
	propagateInTestReferences(ordered)

	for _, function := range ordered {
		if function.test && !function.production {
			pass.Reportf(function.decl.Name.Pos(), "function %s is used only by tests; move it to a _test.go file or remove it", function.decl.Name.Name)
		}
	}

	return nil, nil
}

func collectInTestFunctions(pass *analysis.Pass) (map[*types.Func]*inTestFunction, []*inTestFunction) {
	const inTestMainName = "main"

	var obj *types.Func

	functions := map[*types.Func]*inTestFunction{}
	var ordered []*inTestFunction

	for _, file := range pass.Files {
		if inTestFile(pass, file) || ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv != nil || fn.Name.IsExported() || fn.Name.Name == "init" || fn.Name.Name == "_" {
				continue
			}

			if fn.Name.Name == inTestMainName && pass.Pkg.Name() == inTestMainName {
				continue
			}

			if obj, ok = pass.TypesInfo.Defs[fn.Name].(*types.Func); !ok {
				continue
			}

			function := &inTestFunction{decl: fn, callees: map[*inTestFunction]bool{}}
			functions[obj.Origin()] = function
			ordered = append(ordered, function)
		}
	}

	return functions, ordered
}

func collectInTestReferences(pass *analysis.Pass, functions map[*types.Func]*inTestFunction) {
	for _, file := range pass.Files {
		test := inTestFile(pass, file)
		for _, decl := range file.Decls {
			var caller *inTestFunction
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func); ok {
					caller = functions[obj.Origin()]
				}
			}

			collectInTestDeclarationReferences(pass, decl, functions, caller, test)
		}
	}
}

func collectInTestDeclarationReferences(pass *analysis.Pass, decl ast.Decl, functions map[*types.Func]*inTestFunction, caller *inTestFunction, test bool) {
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

		if callee := functions[obj.Origin()]; callee != nil {
			switch {
			case test:
				callee.test = true
			case caller != nil:
				caller.callees[callee] = true
			default:
				// Exported functions, methods, init/main, generated code,
				// and package initializers are production entry points.
				callee.production = true
			}
		}

		return true
	})
}

func propagateInTestReferences(functions []*inTestFunction) {
	for changed := true; changed; {
		changed = false
		for _, caller := range functions {
			for callee := range caller.callees {
				if caller.production && !callee.production {
					callee.production = true
					changed = true
				}

				if caller.test && !callee.test {
					callee.test = true
					changed = true
				}
			}
		}
	}
}

func inTestFile(pass *analysis.Pass, file *ast.File) bool {
	return strings.HasSuffix(pass.Fset.PositionFor(file.Pos(), false).Filename, "_test.go")
}
