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

type inlineRule struct {
	name  string
	check func(*analysis.Pass, ast.Expr) *ast.Ident
}

// Each standalone rule checks only its own original edits. The aggregate
// explicitly supplies both rules to a shared prerequisite.
func newInlineCompilePlanAnalyzer(rules ...inlineRule) *analysis.Analyzer {
	name := "inlinecompileplan"
	if len(rules) == 1 {
		name += "_" + rules[0].name
	}
	return &analysis.Analyzer{
		Name:       name,
		Doc:        "checks the original inline suggested fixes for compiler safety",
		Run:        func(pass *analysis.Pass) (any, error) { return buildInlineCompilePlanForRules(pass, rules) },
		ResultType: reflect.TypeOf((*inlineCompilePlan)(nil)),
	}
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

func buildInlineCompilePlanForRules(pass *analysis.Pass, rules []inlineRule) (any, error) {
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
	for _, rule := range rules {
		for _, file := range pass.Files {
			imports := buildImportMap(file)
			originalImports := maps.Clone(imports)
			candidates := collectInlineCandidates(pass, file, rule.check)
			declarations := computeInlineDeclarations(pass, file, imports, collectUsedNamesInFile(file), candidates)
			importEdits, importEncodingErr := encodeInlineImportOwner(pass, file, buildImportEdits(file, originalImports, imports))
			sharedInsertions := map[token.Pos]bool{}
			for _, declaration := range declarations {
				if declaration.pos == declaration.end && declaration.text != "" {
					sharedInsertions[declaration.pos] = true
				}
			}
			// Capture the entire original reporting sequence before accepting or
			// rejecting anything, including the original shared-edit owners.
			stagingPass := *pass
			start := len(plan.diagnostics[rule.name])
			stagingPass.Report = func(d analysis.Diagnostic) {
				if importEncodingErr != nil {
					d.SuggestedFixes = nil
				}
				for _, fix := range d.SuggestedFixes {
					for i := range fix.TextEdits {
						edit := &fix.TextEdits[i]
						if edit.Pos == edit.End && sharedInsertions[edit.Pos] {
							if err := anchorInlineSharedInsertion(pass, edit); err != nil {
								d.SuggestedFixes = nil
								break
							}
						}
					}
				}
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

// Distinct zero-width insertions do not conflict in the driver's fixer. Own
// the preceding original byte as well so separately enabled rules cannot both
// rewrite the same function declaration owner. Applying this replacement emits
// exactly the original insertion bytes, and the plan checks the encoded edits.
func anchorInlineSharedInsertion(pass *analysis.Pass, edit *analysis.TextEdit) error {
	file := pass.Fset.File(edit.Pos)
	if file == nil || pass.ReadFile == nil {
		return fmt.Errorf("missing shared insertion source")
	}
	source, err := pass.ReadFile(file.Name())
	if err != nil {
		return err
	}
	offset := file.Offset(edit.Pos)
	if len(source) != file.Size() || offset <= 0 || offset > len(source) {
		return fmt.Errorf("invalid shared insertion source")
	}
	text := make([]byte, 1, len(edit.NewText)+1)
	text[0] = source[offset-1]
	text = append(text, edit.NewText...)
	edit.Pos--
	edit.NewText = text
	return nil
}

// Imports share file-wide namespace ownership, even when the original policy
// inserts new paths in different groups or changes aliases in separate imports.
// Encode their original edits as one replacement of the existing import region;
// comments, ordering and all generated bytes are preserved exactly.
func encodeInlineImportOwner(pass *analysis.Pass, file *ast.File, edits []analysis.TextEdit) ([]analysis.TextEdit, error) {
	if len(edits) == 0 {
		return edits, nil
	}
	tokenFile := pass.Fset.File(file.Pos())
	if tokenFile == nil || pass.ReadFile == nil {
		return edits, fmt.Errorf("missing import owner source")
	}
	source, err := pass.ReadFile(tokenFile.Name())
	if err != nil {
		return edits, err
	}
	if len(source) != tokenFile.Size() {
		return edits, fmt.Errorf("import owner source differs from selected token file")
	}
	start, end := edits[0].Pos, edits[0].End
	for _, declaration := range file.Decls {
		if declaration, ok := declaration.(*ast.GenDecl); ok && declaration.Tok == token.IMPORT {
			start = min(start, declaration.Pos())
			end = max(end, declaration.End())
		}
	}
	size := 0
	for _, edit := range edits {
		if pass.Fset.File(edit.Pos) != tokenFile || edit.End < edit.Pos || edit.End > tokenFile.Pos(tokenFile.Size()) {
			return edits, fmt.Errorf("invalid import owner edit")
		}
		start = min(start, edit.Pos)
		end = max(end, edit.End)
		size += len(edit.NewText) - int(edit.End-edit.Pos)
	}
	if start == end {
		edit := edits[0]
		if len(edits) != 1 {
			return edits, fmt.Errorf("ambiguous import owner insertion")
		}
		if err := anchorInlineSharedInsertion(pass, &edit); err != nil {
			return edits, err
		}
		return []analysis.TextEdit{edit}, nil
	}
	size += int(end - start)
	if size < 0 {
		return edits, fmt.Errorf("invalid import owner replacement size")
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Pos != edits[j].Pos {
			return edits[i].Pos < edits[j].Pos
		}
		return edits[i].End < edits[j].End
	})
	text := make([]byte, 0, size)
	cursor := tokenFile.Offset(start)
	for _, edit := range edits {
		editStart, editEnd := tokenFile.Offset(edit.Pos), tokenFile.Offset(edit.End)
		if editStart < cursor {
			return edits, fmt.Errorf("overlapping original import edits")
		}
		text = append(text, source[cursor:editStart]...)
		text = append(text, edit.NewText...)
		cursor = editEnd
	}
	text = append(text, source[cursor:tokenFile.Offset(end)]...)
	return []analysis.TextEdit{{Pos: start, End: end, NewText: text}}, nil
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
