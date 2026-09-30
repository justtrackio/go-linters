package linters

import (
	"go/ast"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("noanonstruct", NewNoAnonStruct)
}

// NoAnonStructPlugin requires names for non-empty struct types outside tests.
type NoAnonStructPlugin struct{}

func NewNoAnonStruct(_ any) (register.LinterPlugin, error) {
	return &NoAnonStructPlugin{}, nil
}

func (p *NoAnonStructPlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (p *NoAnonStructPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "noanonstruct",
		Doc:  "reports non-empty anonymous structs outside test files",
		Run:  runNoAnonStruct,
	}}, nil
}

func runNoAnonStruct(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.PositionFor(file.Pos(), false).Filename, "_test.go") {
			continue
		}

		// Exempt the struct directly declared by a type specification, including
		// aliases. Nested anonymous structs still require their own names.
		declared := make(map[*ast.StructType]bool)
		ast.Inspect(file, func(node ast.Node) bool {
			if spec, ok := node.(*ast.TypeSpec); ok {
				if st, ok := ast.Unparen(spec.Type).(*ast.StructType); ok {
					declared[st] = true
				}
			}
			return true
		})
		ast.Inspect(file, func(node ast.Node) bool {
			st, ok := node.(*ast.StructType)
			if ok && !declared[st] && st.Fields != nil && len(st.Fields.List) > 0 {
				pass.Reportf(st.Pos(), "non-empty anonymous struct: define a named type instead")
			}
			return true
		})
	}
	return nil, nil
}
