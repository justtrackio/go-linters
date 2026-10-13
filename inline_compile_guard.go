package linters

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// The prerequisite shares package checking, not transformation state: each rule
// still generates precisely its original declaration, rename and import edits.
var inlineCompilePlanAnalyzer = &analysis.Analyzer{
	Name:       "inlinecompileplan",
	Doc:        "checks the original inline suggested fixes for compiler safety",
	Run:        buildInlineCompilePlan,
	ResultType: reflect.TypeOf((*inlineCompilePlan)(nil)),
}

type inlineCompilePlan struct {
	diagnostics map[string][]analysis.Diagnostic
}

type inlineFixGroup struct {
	rule     string
	indices  []int
	edits    []analysis.TextEdit
	conflict bool
}

func buildInlineCompilePlan(pass *analysis.Pass) (any, error) {
	// Reuse the driver's overlay bytes for staging and all validation passes.
	if pass.ReadFile != nil {
		originalRead := pass.ReadFile
		type readResult struct {
			data []byte
			err  error
		}
		cache := map[string]readResult{}
		cachedPass := *pass
		cachedPass.ReadFile = func(name string) ([]byte, error) {
			if result, ok := cache[name]; ok {
				return result.data, result.err
			}
			data, err := originalRead(name)
			cache[name] = readResult{data, err}
			return data, err
		}
		pass = &cachedPass
	}
	plan := &inlineCompilePlan{diagnostics: map[string][]analysis.Diagnostic{}}
	var groups []*inlineFixGroup
	for _, rule := range []struct {
		name  string
		check func(*analysis.Pass, ast.Expr) *ast.Ident
	}{{"iferrinline", checkedError}, {"okinline", checkedOk}} {
		for _, file := range pass.Files {
			imports := buildImportMap(file)
			originalImports := maps.Clone(imports)
			candidates := collectInlineCandidates(pass, file, rule.check)
			declarations := computeInlineDeclarations(pass, file, imports, collectUsedNamesInFile(file), candidates)
			importEdits := buildImportEdits(file, originalImports, imports)
			// Capture the entire original reporting sequence before accepting or
			// rejecting anything, including the original shared-edit owners.
			stagingPass := *pass
			start := len(plan.diagnostics[rule.name])
			stagingPass.Report = func(d analysis.Diagnostic) {
				plan.diagnostics[rule.name] = append(plan.diagnostics[rule.name], d)
			}
			reportInlineCandidates(&stagingPass, rule.name, candidates, declarations, importEdits)
			hoists := map[*ast.BlockStmt]*inlineFixGroup{}
			var importGroup *inlineFixGroup
			for i, c := range candidates {
				d := plan.diagnostics[rule.name][start+i]
				var group *inlineFixGroup
				if len(c.hoistNames) > 0 {
					if len(importEdits) > 0 {
						group = importGroup
					} else {
						group = hoists[c.fnBody]
					}
				}
				if group == nil {
					group = &inlineFixGroup{rule: rule.name}
					groups = append(groups, group)
				}
				if len(c.hoistNames) > 0 {
					hoists[c.fnBody] = group
					if len(importEdits) > 0 {
						importGroup = group
					}
				}
				group.indices = append(group.indices, start+i)
				if len(d.SuggestedFixes) == 0 {
					// A missing member cannot leave the original shared var/import
					// edit attached to surviving members.
					group.conflict = true
				} else {
					group.edits = append(group.edits, d.SuggestedFixes[0].TextEdits...)
				}
			}
		}
	}
	hasEdits := false
	for _, group := range groups {
		hasEdits = hasEdits || len(group.edits) > 0
	}
	if !hasEdits {
		for rule, diagnostics := range plan.diagnostics {
			for i := range diagnostics {
				diagnostics[i].Message += "; manual fix required (original automatic edits unavailable)"
			}
			plan.diagnostics[rule] = diagnostics
		}
		return plan, nil
	}
	checker, err := newInlinePackageChecker(pass)
	if err == nil {
		err = checker.check(nil)
	}
	if err != nil {
		for _, group := range groups {
			group.conflict = true
		}
	} else {
		for i, group := range groups {
			if inlineEditsConflict(pass.Fset, group.edits, group.edits, true) {
				group.conflict = true
			}
			for _, other := range groups[:i] {
				if inlineEditsConflict(pass.Fset, group.edits, other.edits, false) {
					group.conflict, other.conflict = true, true
				}
			}
		}
		var eligible []*inlineFixGroup
		for _, group := range groups {
			if !group.conflict {
				eligible = append(eligible, group)
			}
		}
		if !inlinePlansCompile(checker, eligible) {
			// The common case checks each complete rule plan and their union.
			// Only a failed plan pays for dependency-group isolation.
			var accepted []*inlineFixGroup
			for _, group := range eligible {
				trial := append(accepted, group)
				if inlinePlansCompile(checker, trial) {
					accepted = trial
				} else {
					group.conflict = true
				}
			}
		}
	}
	for _, group := range groups {
		for _, i := range group.indices {
			d := &plan.diagnostics[group.rule][i]
			if group.conflict {
				d.SuggestedFixes = nil
			}
			if len(d.SuggestedFixes) == 0 {
				d.Message += "; manual fix required (original automatic edits could not be proven compiler-safe)"
			}
		}
	}
	return plan, nil
}

func inlinePlansCompile(checker *inlinePackageChecker, groups []*inlineFixGroup) bool {
	var combined []analysis.TextEdit
	byRule := map[string][]analysis.TextEdit{}
	for _, group := range groups {
		combined = append(combined, group.edits...)
		byRule[group.rule] = append(byRule[group.rule], group.edits...)
	}
	if len(combined) == 0 {
		return true
	}
	if checker.check(combined) != nil {
		return false
	}
	if len(byRule) > 1 {
		for _, rule := range []string{"iferrinline", "okinline"} {
			if len(byRule[rule]) > 0 && checker.check(byRule[rule]) != nil {
				return false
			}
		}
	}
	return true
}

// Positions are meaningful only within their token.File, not package-wide
// byte offsets. Even identical edits are competing owners, not mergeable fixes.
func inlineEditsConflict(fset *token.FileSet, left, right []analysis.TextEdit, same bool) bool {
	for i, a := range left {
		for j, b := range right {
			if same && j <= i {
				continue
			}
			if fset.File(a.Pos) != fset.File(b.Pos) {
				continue
			}
			if (!same && a.Pos == a.End && b.Pos == b.End && a.Pos == b.Pos) || (a.Pos < a.End && b.Pos < b.End && a.Pos < b.End && b.Pos < a.End) ||
				(a.Pos == a.End && b.Pos < a.Pos && a.Pos < b.End) ||
				(b.Pos == b.End && a.Pos < b.Pos && b.Pos < a.End) {
				return true
			}
		}
	}
	return false
}

type inlineSourceFile struct {
	file *token.File
	name string
	data []byte
}

type inlinePackageChecker struct {
	path            string
	version         string
	sizes           types.Sizes
	files           []inlineSourceFile
	imports         inlineOriginalImporter
	originalImports map[string]bool
}

type inlineOriginalImporter map[string]*types.Package

func (imports inlineOriginalImporter) Import(path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if pkg := imports[path]; pkg != nil {
		return pkg, nil
	}
	return nil, fmt.Errorf("original type graph has no unambiguous import %q", path)
}

func newInlinePackageChecker(pass *analysis.Pass) (*inlinePackageChecker, error) {
	if pass.Pkg == nil || pass.TypesInfo == nil || pass.ReadFile == nil || pass.TypesSizes == nil {
		return nil, fmt.Errorf("missing original package checking state")
	}
	checker := &inlinePackageChecker{path: pass.Pkg.Path(), version: pass.Pkg.GoVersion(), sizes: pass.TypesSizes, imports: inlineOriginalImporter{}, originalImports: map[string]bool{}}
	if checker.version == "" && pass.Module != nil && pass.Module.GoVersion != "" {
		checker.version = pass.Module.GoVersion
		if !strings.HasPrefix(checker.version, "go") {
			checker.version = "go" + checker.version
		}
	}
	seen := map[*types.Package]bool{}
	ambiguous := map[string]bool{}
	bind := func(path string, pkg *types.Package) {
		if ambiguous[path] {
			return
		}
		if previous := checker.imports[path]; previous != nil && previous != pkg {
			delete(checker.imports, path)
			ambiguous[path] = true
		} else {
			checker.imports[path] = pkg
		}
	}
	var visit func(*types.Package)
	visit = func(pkg *types.Package) {
		if seen[pkg] {
			return
		}
		seen[pkg] = true
		bind(pkg.Path(), pkg)
		for _, dependency := range pkg.Imports() {
			visit(dependency)
		}
	}
	for _, pkg := range pass.Pkg.Imports() {
		visit(pkg)
	}
	for _, file := range pass.Files {
		tf := pass.Fset.File(file.Pos())
		if tf == nil {
			return nil, fmt.Errorf("missing selected token file")
		}
		data, err := pass.ReadFile(tf.Name())
		if err != nil {
			return nil, err
		}
		if len(data) != tf.Size() {
			return nil, fmt.Errorf("source differs from selected token file")
		}
		checker.files = append(checker.files, inlineSourceFile{tf, tf.Name(), data})
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return nil, err
			}
			checker.originalImports[path] = true
			var obj types.Object
			if spec.Name != nil {
				obj = pass.TypesInfo.Defs[spec.Name]
			} else {
				obj = pass.TypesInfo.Implicits[spec]
			}
			if name, ok := obj.(*types.PkgName); ok {
				bind(path, name.Imported())
			}
		}
	}
	return checker, nil
}

func (checker *inlinePackageChecker) check(edits []analysis.TextEdit) error {
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(checker.files))
	applied := 0
	for _, original := range checker.files {
		var selected []analysis.TextEdit
		for _, edit := range edits {
			if edit.Pos >= token.Pos(original.file.Base()) && edit.Pos <= token.Pos(original.file.Base()+original.file.Size()) {
				if edit.End < edit.Pos || edit.End > token.Pos(original.file.Base()+original.file.Size()) {
					return fmt.Errorf("invalid edit range")
				}
				selected = append(selected, edit)
			}
		}
		sort.SliceStable(selected, func(i, j int) bool {
			if selected[i].Pos != selected[j].Pos {
				return selected[i].Pos < selected[j].Pos
			}
			return selected[i].End < selected[j].End
		})
		data := original.data
		if len(selected) > 0 {
			size := len(original.data)
			for _, edit := range selected {
				size += len(edit.NewText) - int(edit.End-edit.Pos)
			}
			if size < 0 {
				return fmt.Errorf("invalid combined edit size")
			}
			data = make([]byte, 0, size)
			cursor := 0
			for _, edit := range selected {
				start, end := original.file.Offset(edit.Pos), original.file.Offset(edit.End)
				if start < cursor || end > len(original.data) {
					return fmt.Errorf("overlapping edits or source differs from original token file")
				}
				data = append(data, original.data[cursor:start]...)
				data = append(data, edit.NewText...)
				cursor = end
				applied++
			}
			data = append(data, original.data[cursor:]...)
		}
		file, err := parser.ParseFile(fset, original.name, data, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if !checker.originalImports[path] && !inlineGeneratedImportAllowed(checker.path, path) {
				return fmt.Errorf("original generated import %q is not accessible from %q", path, checker.path)
			}
		}
		files = append(files, file)
	}
	if applied != len(edits) {
		return fmt.Errorf("edit outside selected package files")
	}
	config := types.Config{Importer: checker.imports, Sizes: checker.sizes, GoVersion: checker.version}
	_, err := config.Check(checker.path, fset, files, &types.Info{})
	return err
}

// go/types checks identities and declarations, but not cmd/go's import path
// visibility rules. These checks only withhold edits; they never change paths.
func inlineGeneratedImportAllowed(importer, imported string) bool {
	if strings.HasPrefix(imported, "_/") {
		return false
	}
	parts := strings.Split(imported, "/")
	for _, part := range parts[:len(parts)-1] {
		if part == "vendor" {
			return false
		}
	}
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "internal" {
			continue
		}
		parent := strings.Join(parts[:i], "/")
		if parent == "" {
			return false
		}
		return importer == parent || strings.HasPrefix(importer, parent+"/")
	}
	return true
}
