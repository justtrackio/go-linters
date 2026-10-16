package linters

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("methodfilecheck", NewMethodFileCheck)
}

// MethodFileCheckPlugin keeps receiver types, constructors, and methods together.
type MethodFileCheckPlugin struct{}

func (p *MethodFileCheckPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (p *MethodFileCheckPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "methodfilecheck",
		Doc:  "keeps receiver types, constructors, and contiguous method blocks in the same file",
		Run:  runMethodFileCheck,
	}}, nil
}

func NewMethodFileCheck(_ any) (register.LinterPlugin, error) {
	return &MethodFileCheckPlugin{}, nil
}

type methodFileType struct {
	file  *ast.File
	index int
}

func runMethodFileCheck(pass *analysis.Pass) (any, error) {
	locations := make(map[string]methodFileType)
	named := make(map[string]*types.Named)
	guarded := make(map[*ast.File]bool)

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		guarded[file] = methodFileGuarded(file)
		for index, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.TYPE {
				continue
			}

			for _, spec := range group.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				name := typeSpec.Name.Name
				if _, exists := locations[name]; !exists {
					locations[name] = methodFileType{file: file, index: index}
					if typeName, ok := pass.TypesInfo.Defs[typeSpec.Name].(*types.TypeName); ok {
						if n, ok := typeName.Type().(*types.Named); ok {
							named[name] = n.Origin()
						}
					}
				}
			}
		}
	}

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		checkMethodFile(pass, file, locations, named, guarded)
	}

	return nil, nil
}

func checkMethodFile(pass *analysis.Pass, file *ast.File, locations map[string]methodFileType, named map[string]*types.Named, guarded map[*ast.File]bool) {
	first := make(map[string]*ast.FuncDecl)
	previousReceiver := ""

	for index, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}

		receiver := methodFileTypeName(fn.Recv.List[0].Type)
		if receiver == "" {
			continue
		}

		typ, known := locations[receiver]
		if first[receiver] == nil {
			first[receiver] = fn
			if known && typ.file == file && (index < typ.index || !methodFileOnlyConstructors(pass, file.Decls[typ.index+1:index], named[receiver])) {
				pass.Reportf(fn.Pos(), "method %s.%s: keep the receiver type, its constructors, and its first method together", receiver, fn.Name.Name)
			}
		} else if receiver != previousReceiver {
			pass.Reportf(fn.Pos(), "method %s.%s: keep %s methods contiguous; another receiver's methods separate this block", receiver, fn.Name.Name, receiver)
		}

		// Report once per method run, like upstream, rather than once per method.
		if receiver != previousReceiver && known && typ.file != file && !guarded[file] && !guarded[typ.file] {
			filename := filepath.Base(pass.Fset.Position(typ.file.Pos()).Filename)
			pass.Reportf(fn.Pos(), "method %s.%s: must be declared in %s with receiver type %s", receiver, fn.Name.Name, filename, receiver)
		}

		previousReceiver = receiver
	}

	checkMethodFileConstructors(pass, file, locations, named, first)
}

func checkMethodFileConstructors(pass *analysis.Pass, file *ast.File, locations map[string]methodFileType, named map[string]*types.Named, first map[string]*ast.FuncDecl) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil {
			continue
		}

		for name, recv := range named {
			typ, known := locations[name]
			method := first[name]
			if !known || typ.file != file || method == nil || fn.Pos() <= method.Pos() || !methodFileConstructsFor(pass, fn, recv) {
				continue
			}

			pass.Reportf(fn.Pos(), "constructor %s for %s: must be declared before %s methods", fn.Name.Name, name, name)
		}
	}
}

func methodFileOnlyConstructors(pass *analysis.Pass, decls []ast.Decl, recv *types.Named) bool {
	if recv == nil {
		return true
	}

	for _, decl := range decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !methodFileConstructsFor(pass, fn, recv) {
			return false
		}
	}

	return true
}

// A constructor returns the receiver type, or an interface it implements, as
// its first result, alone or alongside a final error.
func methodFileConstructsFor(pass *analysis.Pass, fn *ast.FuncDecl, recv *types.Named) bool {
	if fn.Recv != nil {
		return false
	}

	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}

	named := constructorResult(obj)
	if named == nil {
		return false
	}

	if named.Origin() == recv.Origin() {
		return true
	}

	// Universe interfaces such as error are implemented by every type, so they
	// do not identify a constructed type.
	iface, ok := named.Underlying().(*types.Interface)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}

	return types.Implements(recv, iface) || types.Implements(types.NewPointer(recv), iface)
}

func methodFileTypeName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return methodFileTypeName(expr.X)
	case *ast.IndexExpr:
		return methodFileTypeName(expr.X)
	case *ast.IndexListExpr:
		return methodFileTypeName(expr.X)
	default:
		return ""
	}
}

func methodFileGuarded(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:build") {
				return true
			}
		}
	}

	return false
}
