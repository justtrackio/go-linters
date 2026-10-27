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
	register.Plugin("declfilecheck", NewDeclFileCheck)
}

// DeclFileCheckPlugin places private declarations with their production users.
type DeclFileCheckPlugin struct{}

func (*DeclFileCheckPlugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (*DeclFileCheckPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "declfilecheck",
		Doc:  "reports private types, constants, and variables owned by one other production file",
		Run:  runDeclFileCheck,
	}}, nil
}

func NewDeclFileCheck(_ any) (register.LinterPlugin, error) {
	return &DeclFileCheckPlugin{}, nil
}

type fileDeclaration struct {
	file    *ast.File
	nodes   []ast.Node
	names   []*ast.Ident
	kind    token.Token
	movable bool
	owners  map[*ast.File]bool
	users   map[*fileDeclaration]bool
}

func runDeclFileCheck(pass *analysis.Pass) (any, error) {
	declarations, objects := collectFileDeclarations(pass)
	collectFileDeclarationOwners(pass, declarations, objects)
	propagateFileDeclarationOwners(declarations)
	reportFileDeclarations(pass, declarations)

	return nil, nil
}

func collectFileDeclarationOwners(pass *analysis.Pass, declarations []*fileDeclaration, objects map[types.Object]*fileDeclaration) {
	nodes := make(map[ast.Node]*fileDeclaration)

	for _, declaration := range declarations {
		for _, node := range declaration.nodes {
			nodes[node] = declaration
		}
	}

	for _, file := range pass.Files {
		if inTestFile(pass, file) {
			continue
		}

		for _, decl := range file.Decls {
			var group *ast.GenDecl
			var ok bool

			if group, ok = decl.(*ast.GenDecl); !ok {
				collectFileDeclarationUses(pass, decl, file, nil, objects)

				continue
			}

			for _, spec := range group.Specs {
				collectFileDeclarationUses(pass, spec, file, nodes[spec], objects)
			}
		}
	}
}

func propagateFileDeclarationOwners(declarations []*fileDeclaration) {
	// A declaration inherits the owners of declarations that reference it.
	// Iterate to a fixed point so chains and recursive type groups agree.
	for changed := true; changed; {
		changed = false
		for _, declaration := range declarations {
			for user := range declaration.users {
				for owner := range user.owners {
					if !declaration.owners[owner] {
						declaration.owners[owner] = true
						changed = true
					}
				}
			}
		}
	}
}

func reportFileDeclarations(pass *analysis.Pass, declarations []*fileDeclaration) {
	collections := sharedTypeCollections(declarations)

	for _, declaration := range declarations {
		if !declaration.movable || collections[declaration.file] || len(declaration.owners) != 1 {
			continue
		}

		for owner := range declaration.owners {
			if owner == declaration.file || ast.IsGenerated(owner) || declarationFileGuarded(pass, owner) {
				continue
			}

			filename := filepath.Base(pass.Fset.PositionFor(owner.Pos(), false).Filename)
			for _, name := range declaration.names {
				pass.Reportf(name.Pos(), "%s %s is used only by %s; move it to that file", declaration.kind, name.Name, filename)
			}
		}
	}
}

func sharedTypeCollections(declarations []*fileDeclaration) map[*ast.File]bool {
	collections := make(map[*ast.File]bool)

	for _, declaration := range declarations {
		if declaration.kind != token.TYPE {
			continue
		}

		externalOwners := len(declaration.owners)
		if declaration.owners[declaration.file] {
			externalOwners--
		}

		if externalOwners > 1 {
			collections[declaration.file] = true
		}
	}

	return collections
}

func collectFileDeclarations(pass *analysis.Pass) (declarations []*fileDeclaration, objects map[types.Object]*fileDeclaration) {
	objects = make(map[types.Object]*fileDeclaration)

	for _, file := range pass.Files {
		if inTestFile(pass, file) || ast.IsGenerated(file) || declarationFileGuarded(pass, file) {
			continue
		}

		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || (group.Tok != token.TYPE && group.Tok != token.CONST && group.Tok != token.VAR) {
				continue
			}

			if group.Tok == token.CONST && constantBlockDependsOnPosition(pass, group) {
				declarations = addFileDeclaration(pass, file, group.Tok, group.Specs, declarations, objects)

				continue
			}

			for _, spec := range group.Specs {
				declarations = addFileDeclaration(pass, file, group.Tok, []ast.Spec{spec}, declarations, objects)
			}
		}
	}

	return declarations, objects
}

func constantBlockDependsOnPosition(pass *analysis.Pass, group *ast.GenDecl) bool {
	for _, spec := range group.Specs {
		if len(spec.(*ast.ValueSpec).Values) == 0 {
			return true
		}
	}

	depends := false
	ast.Inspect(group, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && pass.TypesInfo.Uses[ident] == types.Universe.Lookup("iota") {
			depends = true
		}

		return !depends
	})

	return depends
}

func addFileDeclaration(pass *analysis.Pass, file *ast.File, kind token.Token, specs []ast.Spec, declarations []*fileDeclaration, objects map[types.Object]*fileDeclaration) []*fileDeclaration {
	declaration := &fileDeclaration{file: file, kind: kind, movable: true, owners: make(map[*ast.File]bool), users: make(map[*fileDeclaration]bool)}
	for _, spec := range specs {
		declaration.nodes = append(declaration.nodes, spec)
		var names []*ast.Ident

		switch spec := spec.(type) {
		case *ast.TypeSpec:
			names = []*ast.Ident{spec.Name}
		case *ast.ValueSpec:
			names = spec.Names
		}

		for _, name := range names {
			if name.Name == "_" || (ast.IsExported(name.Name) && kind != token.TYPE) {
				return declarations
			}

			if ast.IsExported(name.Name) {
				declaration.movable = false
				declaration.owners[file] = true
			}

			declaration.names = append(declaration.names, name)
		}
	}

	for _, name := range declaration.names {
		objects[pass.TypesInfo.Defs[name]] = declaration
	}

	return append(declarations, declaration)
}

func collectFileDeclarationUses(pass *analysis.Pass, node ast.Node, file *ast.File, user *fileDeclaration, objects map[types.Object]*fileDeclaration) {
	ast.Inspect(node, func(node ast.Node) bool {
		var ident *ast.Ident
		var ok bool

		if ident, ok = node.(*ast.Ident); !ok {
			return true
		}

		declaration := objects[pass.TypesInfo.Uses[ident]]
		if declaration == nil {
			return true
		}

		if user == nil {
			declaration.owners[file] = true
		} else if user != declaration {
			declaration.users[user] = true
		}

		return true
	})
}

func declarationFileGuarded(pass *analysis.Pass, file *ast.File) bool {
	if methodFileGuarded(file) {
		return true
	}

	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "// +build") {
				return true
			}
		}
	}

	filename := filepath.Base(pass.Fset.PositionFor(file.Pos(), false).Filename)
	parts := strings.Split(strings.TrimSuffix(filename, ".go"), "_")
	platforms := " aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos " +
		"386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm "

	return len(parts) > 1 && strings.Contains(platforms, " "+parts[len(parts)-1]+" ")
}
