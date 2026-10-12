package linters

import (
	"go/ast"
	"go/token"
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
	return register.LoadModeSyntax
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
	types := make(map[string]methodFileType)
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
				name := spec.(*ast.TypeSpec).Name.Name
				if _, exists := types[name]; !exists {
					types[name] = methodFileType{file: file, index: index}
				}
			}
		}
	}

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		checkMethodFile(pass, file, types, guarded)
	}

	return nil, nil
}

func checkMethodFile(pass *analysis.Pass, file *ast.File, types map[string]methodFileType, guarded map[*ast.File]bool) {
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

		typ, known := types[receiver]
		if first[receiver] == nil {
			first[receiver] = fn
			if known && typ.file == file && (index < typ.index || !methodFileOnlyConstructors(file.Decls[typ.index+1:index], receiver)) {
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

	checkMethodFileConstructors(pass, file, types, first)
}

func checkMethodFileConstructors(pass *analysis.Pass, file *ast.File, types map[string]methodFileType, first map[string]*ast.FuncDecl) {
	for _, decl := range file.Decls {
		var fn *ast.FuncDecl
		if candidate, ok := decl.(*ast.FuncDecl); ok {
			fn = candidate
		} else {
			continue
		}

		receiver := methodFileConstructedType(fn)
		typ, known := types[receiver]

		if method := first[receiver]; known && typ.file == file && method != nil && fn.Pos() > method.Pos() {
			pass.Reportf(fn.Pos(), "constructor %s for %s: must be declared before %s methods", fn.Name.Name, receiver, receiver)
		}
	}
}

func methodFileOnlyConstructors(decls []ast.Decl, receiver string) bool {
	for _, decl := range decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || methodFileConstructedType(fn) != receiver {
			return false
		}
	}

	return true
}

// A constructor is any free function whose first result is T or *T,
// regardless of its name or the number of additional results.
func methodFileConstructedType(fn *ast.FuncDecl) string {
	if fn.Recv != nil || fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return ""
	}

	return methodFileTypeName(fn.Type.Results.List[0].Type)
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
