package linters

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

func inlineTestPass(t *testing.T, sources map[string]string) *analysis.Pass {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	// Fixed ordering makes ownership and package versions reproducible.
	for _, name := range []string{"fixture.go", "extra.go"} {
		if source, ok := sources[name]; ok {
			file, err := parser.ParseFile(fset, name, source, parser.ParseComments)
			require.NoError(t, err)
			files = append(files, file)
		}
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Implicits: map[ast.Node]types.Object{}, Scopes: map[ast.Node]*types.Scope{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
	sizes := types.SizesFor("gc", "386")
	config := types.Config{Sizes: sizes, GoVersion: "go1.23"}
	pkg, err := config.Check("fixture", fset, files, info)
	require.NoError(t, err)
	return &analysis.Pass{Fset: fset, Files: files, Pkg: pkg, TypesInfo: info, TypesSizes: sizes,
		ReadFile: func(name string) ([]byte, error) {
			if source, ok := sources[name]; ok {
				return []byte(source), nil
			}
			return nil, fmt.Errorf("missing source %s", name)
		}, Report: func(analysis.Diagnostic) {}}
}

func originalInlineDiagnostics(pass *analysis.Pass, rule string) []analysis.Diagnostic {
	var diagnostics []analysis.Diagnostic
	staging := *pass
	staging.Report = func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }
	check := checkedError
	if rule == "okinline" {
		check = checkedOk
	}
	for _, file := range pass.Files {
		imports := buildImportMap(file)
		original := maps.Clone(imports)
		candidates := collectInlineCandidates(pass, file, check)
		decls := computeInlineDeclarations(pass, file, imports, collectUsedNamesInFile(file), candidates)
		reportInlineCandidates(&staging, rule, candidates, decls, buildImportEdits(file, original, imports))
	}
	return diagnostics
}

func TestInlineCompileGuardPreservesOriginalSafeFixes(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": `package fixture
func pair() (int,error) { return 1,nil }
func lookup() (int,bool) { return 1,true }
func errorsOnly() { value,err := pair(); if err != nil { return }; _ = value }
func booleansOnly() { value,ok := lookup(); if !ok { return }; _ = value }
func ordinary() { err := error(nil); if err != nil { return } }
`, "extra.go": `package fixture
func more() { err := error(nil); if err != nil { return } }
`})
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	plan := result.(*inlineCompilePlan)
	var combined []analysis.TextEdit
	for _, rule := range []string{"iferrinline", "okinline"} {
		original := originalInlineDiagnostics(pass, rule)
		require.Equal(t, original, plan.diagnostics[rule], "safe output must preserve original bytes and owners")
		for _, d := range plan.diagnostics[rule] {
			combined = append(combined, d.SuggestedFixes[0].TextEdits...)
		}
	}
	checker, err := newInlinePackageChecker(pass)
	require.NoError(t, err)
	require.NoError(t, checker.check(combined))
}

func TestInlineCompileGuardWithholdsWholeFunctionKeepsDisjointFix(t *testing.T) {
	for _, rule := range []string{"iferrinline", "okinline"} {
		t.Run(rule, func(t *testing.T) {
			checked, condition, typ, result := "err", "err != nil", "error", "nil"
			if rule == "okinline" {
				checked, condition, typ, result = "ok", "!ok", "bool", "true"
			}
			source := fmt.Sprintf(`package fixture
func pair() (int,%s) { return 1,%s }
func unsafe() {
 { value,%s := pair(); if %s { return }; _ = value }
 %s := %s(%s); _ = %s
 { other,%s := pair(); if %s { return }; _ = other }
}
func safe() { %s := %s(%s); if %s { return } }
`, typ, result, checked, condition, checked, typ, result, checked, checked, condition, checked, typ, result, condition)
			pass := inlineTestPass(t, map[string]string{"fixture.go": source})
			original := originalInlineDiagnostics(pass, rule)
			require.Len(t, original, 3)
			checker, err := newInlinePackageChecker(pass)
			require.NoError(t, err)
			var originalEdits []analysis.TextEdit
			for _, d := range original {
				originalEdits = append(originalEdits, d.SuggestedFixes[0].TextEdits...)
			}
			err = checker.check(originalEdits)
			require.ErrorContains(t, err, "no new variables")
			planResult, err := buildInlineCompilePlan(pass)
			require.NoError(t, err)
			diagnostics := planResult.(*inlineCompilePlan).diagnostics[rule]
			for _, d := range diagnostics[:2] {
				require.Empty(t, d.SuggestedFixes)
				require.Contains(t, d.Message, "manual fix required")
			}
			require.Equal(t, original[2], diagnostics[2])
			require.NoError(t, checker.check(diagnostics[2].SuggestedFixes[0].TextEdits))
		})
	}
}

func TestInlineCompileGuardRejectsCombinedOriginalVarOwners(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": `package fixture
func pair() (int,error) { return 1,nil }
func lookup() (int,bool) { return 1,true }
func conflict() {
 value,err := pair(); if err != nil { return }; _ = value
 other,ok := lookup(); if !ok { return }; _ = other
}
func safe() { err := error(nil); if err != nil { return } }
`})
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	plan := result.(*inlineCompilePlan)
	for _, rule := range []string{"iferrinline", "okinline"} {
		require.Empty(t, plan.diagnostics[rule][0].SuggestedFixes)
		require.Contains(t, plan.diagnostics[rule][0].Message, "manual fix required")
	}
	require.NotEmpty(t, plan.diagnostics["iferrinline"][1].SuggestedFixes)
}

func TestInlineCompileGuardFailsClosed(t *testing.T) {
	for _, failure := range []string{"read", "sizes", "baseline"} {
		t.Run(failure, func(t *testing.T) {
			pass := inlineTestPass(t, map[string]string{"fixture.go": "package fixture\nfunc f() { err := error(nil); if err != nil { return } }\n"})
			switch failure {
			case "read":
				pass.ReadFile = func(string) ([]byte, error) { return nil, fmt.Errorf("overlay unavailable") }
			case "sizes":
				pass.TypesSizes = nil
			case "baseline":
				originalRead := pass.ReadFile
				pass.ReadFile = func(name string) ([]byte, error) {
					data, err := originalRead(name)
					return []byte(strings.ReplaceAll(string(data), "error(nil)", "wrong(nil)")), err
				}
			}
			result, err := buildInlineCompilePlan(pass)
			require.NoError(t, err)
			d := result.(*inlineCompilePlan).diagnostics["iferrinline"][0]
			require.Empty(t, d.SuggestedFixes)
			require.Contains(t, d.Message, "manual fix required")
		})
	}
}

func TestInlinePackageCheckerCompilerFailureClasses(t *testing.T) {
	for _, test := range []struct{ source, replacement, message string }{
		{"package fixture\nfunc f() { _ = 1 }\n", "package fixture\nfunc f() { _ = err }\n", "undefined: err"},
		{"package fixture\nfunc f() { _ = 1 }\n", "package fixture\nfunc f() { var err error }\n", "declared and not used: err"},
		{"package fixture\nfunc f() { _ = 1 }\n", "package fixture\nfunc f() { var ok bool }\n", "declared and not used: ok"},
	} {
		pass := inlineTestPass(t, map[string]string{"fixture.go": test.source})
		checker, err := newInlinePackageChecker(pass)
		require.NoError(t, err)
		file := pass.Fset.File(pass.Files[0].Pos())
		err = checker.check([]analysis.TextEdit{{Pos: file.Pos(0), End: file.Pos(file.Size()), NewText: []byte(test.replacement)}})
		require.ErrorContains(t, err, test.message)
	}
}

func TestInlineCompileGuardOriginalUndefinedAndUnusedBindings(t *testing.T) {
	for _, test := range []struct{ rule, body, message string }{
		{"iferrinline", `{ value,err := pair(); if err != nil { return }; _ = value }; var err error; _ = err`, "undefined: err"},
		{"iferrinline", `var err error; var saved = err; _ = saved; value,failure := pair(); if failure != nil { return }; _ = value`, "declared and not used: err"},
		{"iferrinline", `var ok = true; var saved = ok; _ = saved; value,err := pair(); if err != nil { return }; _ = value`, "declared and not used: ok"},
		{"okinline", `var err error; var saved = err; _ = saved; value,ok := lookup(); if !ok { return }; _ = value`, "declared and not used: err"},
		{"okinline", `{ value,ok := lookup(); if !ok { return }; _ = value }; var ok bool; _ = ok`, "undefined: ok"},
	} {
		pass := inlineTestPass(t, map[string]string{"fixture.go": "package fixture\nfunc pair()(int,error){return 1,nil}\nfunc lookup()(int,bool){return 1,true}\nfunc f(){" + test.body + "}\n"})
		original := originalInlineDiagnostics(pass, test.rule)
		require.Len(t, original, 1)
		require.Len(t, original[0].SuggestedFixes, 1)
		checker, err := newInlinePackageChecker(pass)
		require.NoError(t, err)
		require.ErrorContains(t, checker.check(original[0].SuggestedFixes[0].TextEdits), test.message)
		result, err := buildInlineCompilePlan(pass)
		require.NoError(t, err)
		diagnostics := result.(*inlineCompilePlan).diagnostics[test.rule]
		require.Len(t, diagnostics, 1)
		require.Empty(t, diagnostics[0].SuggestedFixes)
		require.Equal(t, original[0].Pos, diagnostics[0].Pos)
		require.Equal(t, original[0].End, diagnostics[0].End)
	}
}

func TestInlineCompileGuardSharedImportOwnerAndTransitiveTypes(t *testing.T) {
	testdata := testdataDir(t)
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedTypesSizes | packages.NeedSyntax | packages.NeedImports | packages.NeedDeps,
		Dir:  filepath.Join(testdata, "src"),
		Env:  append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOPATH="+testdata),
	}, "testlintdata/inlineguardimports")
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.Empty(t, loaded[0].Errors)
	pkg := loaded[0]
	pass := &analysis.Pass{Fset: pkg.Fset, Files: pkg.Syntax, Pkg: pkg.Types, TypesInfo: pkg.TypesInfo, TypesSizes: pkg.TypesSizes, ReadFile: os.ReadFile, Report: func(analysis.Diagnostic) {}}
	original := originalInlineDiagnostics(pass, "iferrinline")
	require.Len(t, original, 2)
	checker, err := newInlinePackageChecker(pass)
	require.NoError(t, err)
	require.NoError(t, checker.check(original[0].SuggestedFixes[0].TextEdits), "safe original import owner uses a newly emitted transitive import")
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	for _, diagnostic := range result.(*inlineCompilePlan).diagnostics["iferrinline"] {
		require.Empty(t, diagnostic.SuggestedFixes, "the unsafe consumer must withhold its shared import owner too")
	}
}

func TestInlineCompileGuardPreservesCompatibleBoundaryOwners(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": `package fixture; func pair()(int,error){return 1,nil}; func f(){err:=error(nil);if err!=nil{return};value,otherErr:=pair();if otherErr!=nil{return};_=value}`})
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	require.Equal(t, originalInlineDiagnostics(pass, "iferrinline"), result.(*inlineCompilePlan).diagnostics["iferrinline"])
}

func TestInlinePackageCheckerPreservesLiteralPackageIdentity(t *testing.T) {
	imported := types.NewPackage("vendor/source/literal", "foreign")
	name := types.NewTypeName(token.NoPos, imported, "Value", nil)
	types.NewNamed(name, types.NewStruct(nil, nil), nil)
	imported.Scope().Insert(name)
	imported.MarkComplete()
	fset := token.NewFileSet()
	source := "package fixture\nimport alias \"source/literal\"\nvar _ alias.Value\n"
	file, err := parser.ParseFile(fset, "fixture.go", source, parser.ParseComments)
	require.NoError(t, err)
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Implicits: map[ast.Node]types.Object{}}
	config := types.Config{Importer: inlineOriginalImporter{"source/literal": imported}, Sizes: types.SizesFor("gc", "386"), GoVersion: "go1.23"}
	pkg, err := config.Check("fixture", fset, []*ast.File{file}, info)
	require.NoError(t, err)
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: config.Sizes, ReadFile: func(string) ([]byte, error) { return []byte(source), nil }}
	checker, err := newInlinePackageChecker(pass)
	require.NoError(t, err)
	resolved, err := checker.imports.Import("source/literal")
	require.NoError(t, err)
	require.Same(t, imported, resolved)
	require.NoError(t, checker.check(nil))
	delete(checker.imports, "source/literal")
	require.Error(t, checker.check(nil), "missing importer identity must fail closed")
}

func TestInlinePackageCheckerTargetSizesAndLanguageVersion(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": "package fixture\nvar number int = 1\n"})
	checker, err := newInlinePackageChecker(pass)
	require.NoError(t, err)
	file := pass.Fset.File(pass.Files[0].Pos())
	replace := func(source string) error {
		return checker.check([]analysis.TextEdit{{Pos: file.Pos(0), End: file.Pos(file.Size()), NewText: []byte(source)}})
	}
	require.Error(t, replace("package fixture\nvar number int = 1 << 40\n"), "386 target overflow must be checked, not host sizes")
	checker.version = "go1.17"
	require.Error(t, replace("package fixture\nfunc identity[T any](v T) T { return v }\n"), "original language version must reject newer syntax")
}

func TestInlineCompileGuardNoCandidatesDoesNotReadSources(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": "package fixture\nfunc f() {}\n"})
	pass.ReadFile = func(string) ([]byte, error) { t.Fatal("empty plan must not read sources"); return nil, nil }
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	require.Empty(t, result.(*inlineCompilePlan).diagnostics["iferrinline"])
	require.Empty(t, result.(*inlineCompilePlan).diagnostics["okinline"])
}

func TestInlineCompileGuardGeneratedImportVisibility(t *testing.T) {
	for _, test := range []struct {
		imported, client string
		safe             bool
	}{
		{"example.com/provider/internal/value", "example.com/client", false},
		{"example.com/provider/internal/value", "example.com/provider/client", true},
		{"example.com/provider/vendor/value", "example.com/client", false},
		{"example.com/provider/vendor", "example.com/client", true},
		{"_/private/local/value", "example.com/client", false},
	} {
		valuePackage := types.NewPackage(test.imported, "value")
		name := types.NewTypeName(token.NoPos, valuePackage, "Value", nil)
		valueType := types.NewNamed(name, types.NewStruct(nil, nil), nil)
		valuePackage.Scope().Insert(name)
		valuePackage.MarkComplete()
		provider := types.NewPackage("example.com/provider", "provider")
		signature := types.NewSignatureType(nil, nil, nil, types.NewTuple(), types.NewTuple(
			types.NewVar(token.NoPos, provider, "", valueType),
			types.NewVar(token.NoPos, provider, "", types.Universe.Lookup("error").Type()),
		), false)
		provider.Scope().Insert(types.NewFunc(token.NoPos, provider, "New", signature))
		provider.SetImports([]*types.Package{valuePackage})
		provider.MarkComplete()
		source := "package fixture\nimport \"example.com/provider\"\nfunc f(){value,err:=provider.New();if err!=nil{return};_=value}\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", source, parser.ParseComments)
		require.NoError(t, err)
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Implicits: map[ast.Node]types.Object{}, Scopes: map[ast.Node]*types.Scope{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}
		config := types.Config{Importer: inlineOriginalImporter{"example.com/provider": provider}, Sizes: types.SizesFor("gc", "amd64"), GoVersion: "go1.23"}
		pkg, err := config.Check(test.client, fset, []*ast.File{file}, info)
		require.NoError(t, err)
		pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: config.Sizes, ReadFile: func(string) ([]byte, error) { return []byte(source), nil }, Report: func(analysis.Diagnostic) {}}
		original := originalInlineDiagnostics(pass, "iferrinline")
		require.Len(t, original, 1)
		require.NotEmpty(t, original[0].SuggestedFixes)
		result, err := buildInlineCompilePlan(pass)
		require.NoError(t, err)
		diagnostic := result.(*inlineCompilePlan).diagnostics["iferrinline"][0]
		if test.safe {
			require.Equal(t, original[0], diagnostic)
		} else {
			require.Empty(t, diagnostic.SuggestedFixes)
			require.Contains(t, diagnostic.Message, "manual fix required")
		}
	}
}

func TestInlineCompileGuardNamedReturnsAndParametersPreserveOriginal(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": `package fixture
func pair()(int,error){return 1,nil}
func parameter(value int) error { value,err:=pair();if err!=nil{return err};_=value;return nil }
func named()(value int,err error){value,err=pair();if err!=nil{return};return}
`})
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	require.Equal(t, originalInlineDiagnostics(pass, "iferrinline"), result.(*inlineCompilePlan).diagnostics["iferrinline"])
}

func TestInlineCompileGuardNestedCrossRuleEditsAreWithheld(t *testing.T) {
	pass := inlineTestPass(t, map[string]string{"fixture.go": `package fixture
func f(flag bool){ value,err:=func()(int,error){ok:=flag;if !ok{return 0,nil};return 1,nil}();if err!=nil{return};_=value }
`})
	result, err := buildInlineCompilePlan(pass)
	require.NoError(t, err)
	for _, rule := range []string{"iferrinline", "okinline"} {
		diagnostics := result.(*inlineCompilePlan).diagnostics[rule]
		require.Len(t, diagnostics, 1)
		require.Empty(t, diagnostics[0].SuggestedFixes)
		require.Contains(t, diagnostics[0].Message, "manual fix required")
	}
}
