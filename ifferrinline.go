package linters

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("iferrinline", NewIfErrInline)
}

type IfErrInline struct{}

func (p *IfErrInline) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "iferrinline",
			Doc:  "reports error assignments followed by a non-nil check that can be inlined into the if initializer",
			Run:  p.run,
		},
	}, nil
}

func (p *IfErrInline) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (p *IfErrInline) run(pass *analysis.Pass) (any, error) {
	return runInlineCheck(pass, "iferrinline", checkedError, checkedOk)
}

func NewIfErrInline(_ any) (register.LinterPlugin, error) {
	return &IfErrInline{}, nil
}

const errorVariableName = "err"

type candidate struct {
	checkName  string
	assign     *ast.AssignStmt
	ifStmt     *ast.IfStmt
	hoistNames []string
}

type declEdit struct {
	text string
}

// runInlineCheck shares scope preservation and suggested fixes between the
// error and boolean check analyzers.
func runInlineCheck(pass *analysis.Pass, analyzerName string, checkedVariable, conflictingVariable func(*analysis.Pass, ast.Expr) *ast.Ident) (any, error) {
	for _, file := range pass.Files {
		imports := buildImportMap(file)
		originalImports := maps.Clone(imports)
		usedImportNames := collectUsedNamesInFile(file)

		candidates := collectInlineCandidates(pass, file, checkedVariable)
		conflictingCandidates := collectInlineCandidates(pass, file, conflictingVariable)
		fixes := computeInlineFixes(pass, imports, originalImports, usedImportNames, candidates, conflictingCandidates)
		importEdits := buildImportEdits(file, originalImports, imports)
		reportInlineCandidates(pass, analyzerName, candidates, fixes, importEdits)
	}

	return nil, nil
}

func collectInlineCandidates(pass *analysis.Pass, file *ast.File, checkedVariable func(*analysis.Pass, ast.Expr) *ast.Ident) []candidate {
	var candidates []candidate
	ast.Inspect(file, func(n ast.Node) bool {
		var ok bool
		var block *ast.BlockStmt

		if block, ok = n.(*ast.BlockStmt); !ok {
			return true
		}

		for i := 0; i < len(block.List)-1; i++ {
			if c, ok := matchPattern(pass, block, i, checkedVariable); ok {
				candidates = append(candidates, c)
			}
		}

		return true
	})

	return candidates
}

func computeInlineFixes(pass *analysis.Pass, imports, originalImports map[string]string, usedImportNames map[string]bool, candidates, conflictingCandidates []candidate) map[*ast.AssignStmt]analysis.SuggestedFix {
	allCandidates := append(append([]candidate(nil), candidates...), conflictingCandidates...)
	uniqueCandidates := make([]candidate, 0, len(allCandidates))
	seen := make(map[*ast.AssignStmt]bool, len(allCandidates))
	for _, c := range allCandidates {
		if !seen[c.assign] {
			seen[c.assign] = true
			uniqueCandidates = append(uniqueCandidates, c)
		}
	}
	sort.Slice(uniqueCandidates, func(i, j int) bool {
		return uniqueCandidates[i].assign.Pos() > uniqueCandidates[j].assign.Pos()
	})

	currentCandidates := make(map[*ast.AssignStmt]bool, len(candidates))
	for _, c := range candidates {
		currentCandidates[c.assign] = true
	}

	type proposal struct {
		fix             analysis.SuggestedFix
		imports         map[string]string
		renderedImports map[string]string
		importChanges   map[string]string
	}

	proposals := make(map[*ast.AssignStmt]proposal, len(uniqueCandidates))
	for _, c := range uniqueCandidates {
		candidateImports := maps.Clone(imports)
		candidateUsedImportNames := maps.Clone(usedImportNames)
		fix, renderedImports, ok := buildInlineCandidateFix(pass, candidateImports, candidateUsedImportNames, c)
		if !ok {
			continue
		}

		importChanges := make(map[string]string)
		for path := range renderedImports {
			originalAlias, alreadyImported := originalImports[path]
			if !alreadyImported || originalAlias == "_" {
				importChanges[path] = candidateImports[path]
			}
		}
		proposals[c.assign] = proposal{
			fix:             fix,
			imports:         candidateImports,
			renderedImports: renderedImports,
			importChanges:   importChanges,
		}
	}

	conflictedCandidates := make(map[*ast.AssignStmt]bool)
	for _, current := range candidates {
		currentProposal, ok := proposals[current.assign]
		if !ok || len(currentProposal.importChanges) == 0 {
			continue
		}
		for _, conflicting := range conflictingCandidates {
			conflictingProposal, ok := proposals[conflicting.assign]
			if !ok || len(conflictingProposal.importChanges) == 0 ||
				maps.Equal(currentProposal.importChanges, conflictingProposal.importChanges) {
				continue
			}

			conflictedCandidates[current.assign] = true
			conflictedCandidates[conflicting.assign] = true
		}
	}

	accepted := make(map[*ast.AssignStmt]bool, len(uniqueCandidates))
	var acceptedFixes []analysis.SuggestedFix
	for _, c := range uniqueCandidates {
		proposal, ok := proposals[c.assign]
		if !ok || conflictedCandidates[c.assign] || conflictsWithAcceptedFix(pass, proposal.fix, acceptedFixes) {
			continue
		}

		accepted[c.assign] = true
		acceptedFixes = append(acceptedFixes, proposal.fix)
		if !currentCandidates[c.assign] {
			continue
		}

		for path, alias := range proposal.renderedImports {
			if originalAlias, alreadyImported := originalImports[path]; alreadyImported && originalAlias != "_" {
				continue
			}
			imports[path] = proposal.imports[path]
			usedImportNames[alias] = true
		}
	}

	fixes := make(map[*ast.AssignStmt]analysis.SuggestedFix, len(candidates))
	for _, c := range candidates {
		if accepted[c.assign] {
			fixes[c.assign] = proposals[c.assign].fix
		}
	}

	return fixes
}

func buildInlineCandidateFix(pass *analysis.Pass, imports map[string]string, usedImportNames map[string]bool, c candidate) (analysis.SuggestedFix, map[string]string, bool) {
	if len(c.hoistNames) == 0 {
		fix, ok := buildInlineFix(pass, c.assign, c.ifStmt)
		return fix, nil, ok
	}

	renderedImports := make(map[string]string)
	declarations, ok := computeHoistDeclarations(pass, imports, usedImportNames, c, renderedImports)
	if !ok {
		return analysis.SuggestedFix{}, nil, false
	}

	fix, ok := buildHoistFix(pass, c, declarations)
	return fix, renderedImports, ok
}

func conflictsWithAcceptedFix(pass *analysis.Pass, fix analysis.SuggestedFix, accepted []analysis.SuggestedFix) bool {
	for _, existing := range accepted {
		for _, edit := range fix.TextEdits {
			for _, existingEdit := range existing.TextEdits {
				if textEditsOverlap(pass, edit, existingEdit) {
					return true
				}
			}
		}
	}

	return false
}

func textEditsOverlap(pass *analysis.Pass, left, right analysis.TextEdit) bool {
	leftStart, leftEnd := pass.Fset.Position(left.Pos).Offset, pass.Fset.Position(left.End).Offset
	rightStart, rightEnd := pass.Fset.Position(right.Pos).Offset, pass.Fset.Position(right.End).Offset

	if leftStart == leftEnd {
		return leftStart >= rightStart && leftStart <= rightEnd
	}
	if rightStart == rightEnd {
		return rightStart >= leftStart && rightStart <= leftEnd
	}

	return leftStart < rightEnd && rightStart < leftEnd
}

func inlineCandidateMessage(c candidate, fixable bool) string {
	if !fixable {
		return "if " + c.checkName + " cannot be safely inlined automatically"
	}

	if len(c.hoistNames) == 0 {
		return "if " + c.checkName + " can be inlined into the assignment"
	}

	return "if " + c.checkName + " can be inlined by declaring " + strings.Join(c.hoistNames, ", ") +
		" before the assignment and changing := to ="
}

func reportInlineCandidates(pass *analysis.Pass, analyzerName string, candidates []candidate, fixes map[*ast.AssignStmt]analysis.SuggestedFix, importEdits []analysis.TextEdit) {
	for _, c := range candidates {
		fix, fixable := inlineCandidateFix(pass, c, fixes)
		diag := analysis.Diagnostic{
			Pos: c.assign.Pos(), End: c.ifStmt.End(),
			Category: analyzerName, Message: inlineCandidateMessage(c, fixable),
		}

		if fixable {
			if len(c.hoistNames) > 0 {
				fix.TextEdits = append(fix.TextEdits, importEdits...)
				importEdits = nil
			}

			diag.SuggestedFixes = []analysis.SuggestedFix{fix}
		}

		pass.Report(diag)
	}
}

func inlineCandidateFix(_ *analysis.Pass, c candidate, fixes map[*ast.AssignStmt]analysis.SuggestedFix) (analysis.SuggestedFix, bool) {
	fix, ok := fixes[c.assign]
	return fix, ok
}

func matchPattern(pass *analysis.Pass, block *ast.BlockStmt, i int, checkedVariable func(*analysis.Pass, ast.Expr) *ast.Ident) (candidate, bool) {
	var ok bool
	var zero candidate
	var companions []string

	assign, ok := block.List[i].(*ast.AssignStmt)
	if !ok || (assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN) {
		return zero, false
	}

	if len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
		return zero, false
	}

	ifStmt, ok := block.List[i+1].(*ast.IfStmt)
	if !ok || ifStmt.Init != nil {
		return zero, false
	}

	checked := checkedVariable(pass, ifStmt.Cond)
	if checked == nil {
		return zero, false
	}

	checkName := checked.Name

	if companions, ok = assignmentCompanions(pass, assign, checked); !ok {
		return zero, false
	}

	if pass.Fset.Position(assign.Pos()).Line != pass.Fset.Position(assign.End()).Line {
		return zero, false
	}

	// Plain assignments keep the existing bindings, so inlining does not
	// restrict their scope even when the checked variable or companions are used afterward.
	if assign.Tok == token.ASSIGN {
		return candidate{checkName: checkName, assign: assign, ifStmt: ifStmt}, true
	}

	rest := block.List[i+2:]
	needsHoist := false
	for _, name := range companions {
		needsHoist = needsHoist || usedAfter(rest, name)
	}

	// A short declaration that reuses any existing LHS binding must stay an
	// assignment to that binding; moving it into an if initializer would shadow it.
	needsHoist = needsHoist || assignmentHasPartialRedecl(pass, assign)
	if !needsHoist && usedAfter(rest, checkName) {
		return zero, false
	}

	c := candidate{checkName: checkName, assign: assign, ifStmt: ifStmt}
	if !needsHoist {
		return c, true
	}

	c.hoistNames = newAssignmentNames(pass, assign)
	if len(c.hoistNames) == 0 {
		return zero, false
	}

	return c, true
}

func assignmentHasPartialRedecl(pass *analysis.Pass, assign *ast.AssignStmt) bool {
	if pass.TypesInfo == nil {
		return true
	}

	for _, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if ok && id.Name != "_" && pass.TypesInfo.Defs[id] == nil {
			return true
		}
	}

	return false
}

func newAssignmentNames(pass *analysis.Pass, assign *ast.AssignStmt) []string {
	if pass.TypesInfo == nil {
		return nil
	}

	var names []string
	for _, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" || pass.TypesInfo.Defs[id] == nil {
			continue
		}

		names = append(names, id.Name)
	}

	return names
}

func assignmentCompanions(pass *analysis.Pass, assign *ast.AssignStmt, checked *ast.Ident) ([]string, bool) {
	var ok bool
	var id *ast.Ident
	var hasCheck bool
	var companions []string

	for _, lhs := range assign.Lhs {
		if id, ok = lhs.(*ast.Ident); !ok {
			return nil, false
		}

		switch id.Name {
		case checked.Name:
			if pass.TypesInfo == nil || pass.TypesInfo.ObjectOf(id) == nil || pass.TypesInfo.ObjectOf(id) != pass.TypesInfo.ObjectOf(checked) {
				return nil, false
			}

			hasCheck = true
		case "_":
		default:
			companions = append(companions, id.Name)
		}
	}

	return companions, hasCheck
}

// hoistRHSIsSafe reports whether declarations inserted before the assignment
// would capture an outer identifier used by its right-hand side.
func hoistRHSIsSafe(pass *analysis.Pass, assign *ast.AssignStmt) bool {
	if pass.TypesInfo == nil {
		return false
	}

	newVariables := make(map[string]types.Object)
	for _, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}

		if obj := pass.TypesInfo.Defs[id]; obj != nil {
			newVariables[id.Name] = obj
		}
	}

	for _, rhs := range assign.Rhs {
		selectors := map[*ast.Ident]bool{}
		ast.Inspect(rhs, func(n ast.Node) bool {
			if selector, ok := n.(*ast.SelectorExpr); ok && selector.Sel != nil {
				selectors[selector.Sel] = true
			}

			return true
		})

		safe := true
		ast.Inspect(rhs, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || selectors[id] {
				return true
			}

			newVariable := newVariables[id.Name]
			usedVariable := pass.TypesInfo.Uses[id]
			if newVariable == nil || usedVariable == nil {
				return true
			}

			if isStrictScopeDescendant(usedVariable.Parent(), newVariable.Parent()) {
				return true
			}

			safe = false
			return false
		})

		if !safe {
			return false
		}
	}

	return true
}

func isStrictScopeDescendant(scope, ancestor *types.Scope) bool {
	if scope == nil || ancestor == nil || scope == ancestor {
		return false
	}

	for scope = scope.Parent(); scope != nil; scope = scope.Parent() {
		if scope == ancestor {
			return true
		}
	}

	return false
}

func computeHoistDeclarations(pass *analysis.Pass, imports map[string]string, usedImportNames map[string]bool, c candidate, renderedImports map[string]string) (declEdit, bool) {
	if pass.TypesInfo == nil || !hoistRHSIsSafe(pass, c.assign) {
		return declEdit{}, false
	}

	qualifier := hoistPackageQualifier(pass, imports, usedImportNames, renderedImports)
	declarations, ok := collectNewHoistDeclarations(pass, c, qualifier, imports)
	scope := assignmentScope(pass, c.assign)
	if !ok || len(declarations) == 0 || scope == nil || !hoistDeclarationsPreserveTypes(pass, c, scope, declarations) {
		return declEdit{}, false
	}

	return renderHoistDeclarations(declarations), true
}

func assignmentScope(pass *analysis.Pass, assign *ast.AssignStmt) *types.Scope {
	for _, lhs := range assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok || id.Name == "_" {
			continue
		}

		if obj := pass.TypesInfo.Defs[id]; obj != nil {
			return obj.Parent()
		}
	}

	return nil
}

type hoistDeclaration struct {
	name string
	typ  string
}

func hoistPackageQualifier(pass *analysis.Pass, imports map[string]string, usedImportNames map[string]bool, renderedImports map[string]string) types.Qualifier {
	return func(p *types.Package) string {
		if p == nil || p == pass.Pkg {
			return ""
		}

		alias, ok := imports[p.Path()]
		if !ok || alias == "_" {
			alias = p.Name()
			for suffix := 2; usedImportNames[alias] || types.Universe.Lookup(alias) != nil; suffix++ {
				alias = p.Name() + strconv.Itoa(suffix)
			}

			usedImportNames[alias] = true
			imports[p.Path()] = alias

			if alias == p.Name() {
				imports[p.Path()] = ""
			}
		}

		localName := alias
		if localName == "" {
			localName = p.Name()
		}
		renderedImports[p.Path()] = localName

		if alias == "." {
			return ""
		}

		if alias == "" {
			return p.Name()
		}

		return alias
	}
}

func collectNewHoistDeclarations(pass *analysis.Pass, c candidate, qualifier types.Qualifier, imports map[string]string) ([]hoistDeclaration, bool) {
	var collected []hoistDeclaration
	scope := assignmentScope(pass, c.assign)
	if scope == nil {
		return nil, false
	}

	for _, lhs := range c.assign.Lhs {
		id, ok := lhs.(*ast.Ident)
		if !ok {
			return nil, false
		}

		if id.Name == "_" {
			continue
		}

		obj := pass.TypesInfo.Defs[id]
		if obj == nil {
			continue
		}

		if hoistTypeHasUnsafeStructure(obj.Type(), pass.Pkg, imports, scope, c.assign.Pos()) {
			return nil, false
		}

		collected = append(collected, hoistDeclaration{
			name: id.Name,
			typ:  types.TypeString(obj.Type(), qualifier),
		})
	}

	return collected, true
}

func hoistTypeHasUnsafeStructure(typ types.Type, local *types.Package, imports map[string]string, scope *types.Scope, pos token.Pos) bool {
	switch typ := typ.(type) {
	case *types.Alias:
		return hoistTypeNameIsShadowed(typ.Obj(), imports, scope, pos) ||
			typeArgumentsHaveUnsafeStructure(typ.TypeArgs(), local, imports, scope, pos)
	case *types.Named:
		return hoistTypeNameIsShadowed(typ.Obj(), imports, scope, pos) ||
			typeArgumentsHaveUnsafeStructure(typ.TypeArgs(), local, imports, scope, pos)
	case *types.Pointer:
		return hoistTypeHasUnsafeStructure(typ.Elem(), local, imports, scope, pos)
	case *types.Array:
		return hoistTypeHasUnsafeStructure(typ.Elem(), local, imports, scope, pos)
	case *types.Slice:
		return hoistTypeHasUnsafeStructure(typ.Elem(), local, imports, scope, pos)
	case *types.Map:
		return hoistTypeHasUnsafeStructure(typ.Key(), local, imports, scope, pos) ||
			hoistTypeHasUnsafeStructure(typ.Elem(), local, imports, scope, pos)
	case *types.Chan:
		return hoistTypeHasUnsafeStructure(typ.Elem(), local, imports, scope, pos)
	case *types.Signature:
		return tupleHasUnsafeStructure(typ.Params(), local, imports, scope, pos) ||
			tupleHasUnsafeStructure(typ.Results(), local, imports, scope, pos)
	case *types.Struct:
		for i := 0; i < typ.NumFields(); i++ {
			field := typ.Field(i)
			if hasForeignUnexportedName(field, local) ||
				hoistTypeHasUnsafeStructure(field.Type(), local, imports, scope, pos) {
				return true
			}
		}
	case *types.Interface:
		typ.Complete()
		for i := 0; i < typ.NumExplicitMethods(); i++ {
			method := typ.ExplicitMethod(i)
			if hasForeignUnexportedName(method, local) ||
				hoistTypeHasUnsafeStructure(method.Type(), local, imports, scope, pos) {
				return true
			}
		}
		for i := 0; i < typ.NumEmbeddeds(); i++ {
			if hoistTypeHasUnsafeStructure(typ.EmbeddedType(i), local, imports, scope, pos) {
				return true
			}
		}
	case *types.Union:
		for i := 0; i < typ.Len(); i++ {
			if hoistTypeHasUnsafeStructure(typ.Term(i).Type(), local, imports, scope, pos) {
				return true
			}
		}
	}

	return false
}

func hoistTypeNameIsShadowed(name *types.TypeName, imports map[string]string, scope *types.Scope, pos token.Pos) bool {
	if name.Pkg() == nil || imports[name.Pkg().Path()] != "." {
		return false
	}

	_, object := scope.LookupParent(name.Name(), pos-1)
	typeName, ok := object.(*types.TypeName)
	return !ok || !types.Identical(typeName.Type(), name.Type())
}

func typeArgumentsHaveUnsafeStructure(args *types.TypeList, local *types.Package, imports map[string]string, scope *types.Scope, pos token.Pos) bool {
	if args == nil {
		return false
	}

	for i := 0; i < args.Len(); i++ {
		if hoistTypeHasUnsafeStructure(args.At(i), local, imports, scope, pos) {
			return true
		}
	}

	return false
}

func tupleHasUnsafeStructure(tuple *types.Tuple, local *types.Package, imports map[string]string, scope *types.Scope, pos token.Pos) bool {
	if tuple == nil {
		return false
	}

	for i := 0; i < tuple.Len(); i++ {
		if hoistTypeHasUnsafeStructure(tuple.At(i).Type(), local, imports, scope, pos) {
			return true
		}
	}

	return false
}

func hasForeignUnexportedName(object types.Object, local *types.Package) bool {
	if object.Exported() {
		return false
	}

	owner := object.Pkg()
	return owner == nil || local == nil || owner.Path() != local.Path()
}

func hoistDeclarationsPreserveTypes(pass *analysis.Pass, c candidate, scope *types.Scope, declarations []hoistDeclaration) bool {
	declaredNames := make(map[string]bool, len(declarations))
	for _, declaration := range declarations {
		expr, err := parser.ParseExpr(declaration.typ)
		if err != nil {
			return false
		}

		ignored := make(map[*ast.Ident]bool)
		selectorBases := make(map[*ast.Ident]bool)
		accessible := true
		ast.Inspect(expr, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.SelectorExpr:
				ignored[node.Sel] = true
				if !node.Sel.IsExported() {
					accessible = false
				}
				if base, ok := node.X.(*ast.Ident); ok {
					selectorBases[base] = true
				}
			case *ast.Field:
				for _, name := range node.Names {
					ignored[name] = true
				}
			}

			return true
		})
		if !accessible {
			return false
		}

		safe := true
		ast.Inspect(expr, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok || ignored[id] {
				return true
			}

			if declaredNames[id.Name] {
				safe = false
				return false
			}

			_, object := scope.LookupParent(id.Name, c.assign.Pos()-1)
			if selectorBases[id] {
				if object == nil {
					// Newly-added import aliases are unique across the file.
					return true
				}

				if _, ok := object.(*types.PkgName); !ok {
					safe = false
					return false
				}

				return true
			}

			typeName, ok := object.(*types.TypeName)
			if !ok {
				safe = false
				return false
			}

			if packageName := pass.Pkg.Scope().Lookup(id.Name); packageName != nil && packageName != typeName {
				safe = false
				return false
			}

			if universeName := types.Universe.Lookup(id.Name); universeName != nil && universeName != typeName {
				safe = false
				return false
			}

			return true
		})
		if !safe {
			return false
		}

		declaredNames[declaration.name] = true
	}

	return true
}

func renderHoistDeclarations(declarations []hoistDeclaration) declEdit {
	var b strings.Builder
	for i, declaration := range declarations {
		if i > 0 {
			b.WriteByte('\n')
		}

		b.WriteString("var ")
		b.WriteString(declaration.name)
		b.WriteByte(' ')
		b.WriteString(declaration.typ)
	}

	return declEdit{text: b.String()}
}

func buildInlineFix(pass *analysis.Pass, assign *ast.AssignStmt, ifStmt *ast.IfStmt) (analysis.SuggestedFix, bool) {
	var err error
	var src []byte

	if pass.ReadFile == nil {
		return analysis.SuggestedFix{}, false
	}

	filename := pass.Fset.Position(assign.Pos()).Filename

	if src, err = pass.ReadFile(filename); err != nil {
		return analysis.SuggestedFix{}, false
	}

	startOff := pass.Fset.Position(assign.Pos()).Offset

	endOff := pass.Fset.Position(assign.End()).Offset
	if startOff < 0 || endOff > len(src) || startOff >= endOff {
		return analysis.SuggestedFix{}, false
	}

	assignText := make([]byte, 0, endOff-startOff+2)
	assignText = append(assignText, src[startOff:endOff]...)
	assignText = append(assignText, ';', ' ')

	return analysis.SuggestedFix{
		Message: "inline assignment into the if statement",
		TextEdits: []analysis.TextEdit{
			{Pos: assign.Pos(), End: ifStmt.Pos()},
			{Pos: ifStmt.Cond.Pos(), End: ifStmt.Cond.Pos(), NewText: assignText},
		},
	}, true
}

func buildHoistFix(pass *analysis.Pass, c candidate, edit declEdit) (analysis.SuggestedFix, bool) {
	var err error
	var src []byte

	if pass.ReadFile == nil {
		return analysis.SuggestedFix{}, false
	}

	filename := pass.Fset.Position(c.assign.Pos()).Filename
	if src, err = pass.ReadFile(filename); err != nil {
		return analysis.SuggestedFix{}, false
	}

	startOff := pass.Fset.Position(c.assign.Pos()).Offset
	tokOff := pass.Fset.Position(c.assign.TokPos).Offset
	endOff := pass.Fset.Position(c.assign.End()).Offset
	ifStartOff := pass.Fset.Position(c.ifStmt.Pos()).Offset
	if startOff < 0 || tokOff < startOff || tokOff+2 > endOff || ifStartOff < endOff || ifStartOff > len(src) {
		return analysis.SuggestedFix{}, false
	}

	gap := src[endOff:ifStartOff]
	gapComments, ok := splitHoistGapComments(gap)
	if !ok {
		return analysis.SuggestedFix{}, false
	}

	lhsText := src[startOff:tokOff]
	rhsText := src[tokOff+2 : endOff]
	insert := make([]byte, 0, len(lhsText)+len(rhsText)+2)
	insert = append(insert, lhsText...)
	insert = append(insert, '=')
	insert = append(insert, rhsText...)
	insert = append(insert, ';', ' ')

	lineStart := startOff
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	indent := src[lineStart:startOff]
	for _, b := range indent {
		if b != ' ' && b != '\t' {
			return analysis.SuggestedFix{}, false
		}
	}

	indentCount := 0
	for i := 0; i < len(edit.text); i++ {
		if edit.text[i] == '\n' {
			indentCount++
		}
	}

	declarationText := make([]byte, 0, len(edit.text)+indentCount*len(indent)+len(gap))
	for i := 0; i < len(edit.text); i++ {
		declarationText = append(declarationText, edit.text[i])
		if edit.text[i] == '\n' {
			declarationText = append(declarationText, indent...)
		}
	}
	if len(gapComments) == 0 {
		declarationText = append(declarationText, gap...)
	} else {
		ifLineStart := ifStartOff
		for ifLineStart > 0 && src[ifLineStart-1] != '\n' {
			ifLineStart--
		}

		ifIndent := src[ifLineStart:ifStartOff]
		for _, b := range ifIndent {
			if b != ' ' && b != '\t' {
				return analysis.SuggestedFix{}, false
			}
		}

		for _, comment := range gapComments {
			declarationText = append(declarationText, '\n')
			declarationText = append(declarationText, ifIndent...)
			declarationText = append(declarationText, comment...)
		}
		declarationText = append(declarationText, '\n')
		declarationText = append(declarationText, ifIndent...)
	}

	return analysis.SuggestedFix{
		Message: "declare assignment variables in their original block and inline the assignment",
		TextEdits: []analysis.TextEdit{
			{Pos: c.assign.Pos(), End: c.ifStmt.Pos(), NewText: declarationText},
			{Pos: c.ifStmt.Cond.Pos(), End: c.ifStmt.Cond.Pos(), NewText: insert},
			{Pos: c.ifStmt.End(), End: c.ifStmt.End(), NewText: []byte("\n")},
		},
	}, true
}

func splitHoistGapComments(gap []byte) ([][]byte, bool) {
	var comments [][]byte

	for i := 0; i < len(gap); {
		switch gap[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '/':
			start := i
			if i+1 >= len(gap) {
				return nil, false
			}

			switch gap[i+1] {
			case '/':
				i += 2
				for i < len(gap) && gap[i] != '\n' {
					i++
				}
				end := i
				if end > start && gap[end-1] == '\r' {
					end--
				}
				comments = append(comments, gap[start:end])
			case '*':
				i += 2
				for i+1 < len(gap) && (gap[i] != '*' || gap[i+1] != '/') {
					i++
				}
				if i+1 >= len(gap) {
					return nil, false
				}
				i += 2
				comments = append(comments, gap[start:i])
			default:
				return nil, false
			}
		default:
			return nil, false
		}
	}

	return comments, true
}

func checkedError(pass *analysis.Pass, expr ast.Expr) *ast.Ident {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return nil
	}

	x, xok := bin.X.(*ast.Ident)

	y, yok := bin.Y.(*ast.Ident)
	if !xok || !yok || pass.TypesInfo == nil {
		return nil
	}

	if pass.TypesInfo.ObjectOf(y) != types.Universe.Lookup("nil") {
		return nil
	}

	obj, ok := pass.TypesInfo.ObjectOf(x).(*types.Var)
	if !ok || !types.Implements(obj.Type(), types.Universe.Lookup("error").Type().Underlying().(*types.Interface)) {
		return nil
	}

	return x
}

func buildImportMap(file *ast.File) map[string]string {
	var err error
	var path string

	m := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		if path, err = strconv.Unquote(imp.Path.Value); err != nil {
			continue
		}

		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}

		m[path] = alias
	}

	return m
}

func usedAfter(stmts []ast.Stmt, name string) bool {
	for _, s := range stmts {
		var found bool
		ast.Inspect(s, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
				found = true

				return false
			}

			return true
		})

		if found {
			return true
		}
	}

	return false
}

// Reserve identifiers throughout the file so new package qualifiers cannot
// shadow existing bindings or be shadowed at a generated declaration.
func collectUsedNamesInFile(file *ast.File) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			names[id.Name] = true
		}

		return true
	})

	return names
}

func buildImportEdits(file *ast.File, original, updated map[string]string) []analysis.TextEdit {
	var edits []analysis.TextEdit
	var paths []string

	for path, alias := range updated {
		old, exists := original[path]
		if !exists {
			paths = append(paths, path)

			continue
		}

		if old == "_" && alias != old {
			edits = append(edits, importAliasEdits(file, path, alias)...)
		}
	}

	sort.Strings(paths)
	if len(paths) == 0 {
		return edits
	}

	return append(edits, importAdditionEdits(file, paths, updated)...)
}

func importAliasEdits(file *ast.File, path, alias string) []analysis.TextEdit {
	var edits []analysis.TextEdit
	for _, imp := range file.Imports {
		if imp.Path.Value == strconv.Quote(path) {
			edits = append(edits, analysis.TextEdit{Pos: imp.Name.Pos(), End: imp.Name.End(), NewText: []byte(alias)})
		}
	}

	return edits
}

func importAdditionEdits(file *ast.File, paths []string, updated map[string]string) []analysis.TextEdit {
	var additions strings.Builder
	for _, path := range paths {
		additions.WriteString("\n" + importText(path, updated[path]))
	}

	additions.WriteString("\n")
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}

		// Keep cgo's import and its preamble together in their own declaration.
		if isCgoImport(gen) {
			continue
		}

		if gen.Lparen.IsValid() {
			return groupedImportEdits(gen, paths, updated)
		}

		spec := gen.Specs[0].(*ast.ImportSpec)
		end := importSpecEnd(spec)

		return []analysis.TextEdit{
			{Pos: spec.Pos(), End: spec.Pos(), NewText: []byte("(" + additions.String())},
			{Pos: end, End: end, NewText: []byte("\n)")},
		}
	}

	return []analysis.TextEdit{{Pos: file.Name.End(), End: file.Name.End(), NewText: []byte("\n\nimport (" + additions.String() + ")")}}
}

func isCgoImport(gen *ast.GenDecl) bool {
	for _, spec := range gen.Specs {
		if spec.(*ast.ImportSpec).Path.Value == strconv.Quote("C") {
			return true
		}
	}

	return false
}

func groupedImportEdits(gen *ast.GenDecl, paths []string, updated map[string]string) []analysis.TextEdit {
	insertions := map[token.Pos]string{}
	for _, path := range paths {
		pos := importInsertionPosition(gen, path)
		insertions[pos] += "\n\t" + importText(path, updated[path])
	}

	var positions []token.Pos
	for pos := range insertions {
		positions = append(positions, pos)
	}

	sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
	var edits []analysis.TextEdit

	for _, pos := range positions {
		edits = append(edits, analysis.TextEdit{Pos: pos, End: pos, NewText: []byte(insertions[pos])})
	}

	return edits
}

func importInsertionPosition(gen *ast.GenDecl, path string) token.Pos {
	var err error
	var existing string

	pos := gen.Lparen + 1
	bestScore := -1

	for _, spec := range gen.Specs {
		imp := spec.(*ast.ImportSpec)
		if existing, err = strconv.Unquote(imp.Path.Value); err != nil {
			continue
		}

		score := importMatchScore(path, existing)
		if score > bestScore {
			bestScore = score
			pos = importSpecEnd(imp)
		}
	}

	return pos
}

func importSpecEnd(imp *ast.ImportSpec) token.Pos {
	if imp.Comment != nil {
		return imp.Comment.End()
	}

	return imp.End()
}

func importText(path, alias string) string {
	if alias == "" {
		return strconv.Quote(path)
	}

	return alias + " " + strconv.Quote(path)
}

// Prefer the longest shared path prefix, then an import of the same kind.
// Taking the first match on ties keeps local import groups separate.
func importMatchScore(path, existing string) int {
	score := 0
	if strings.Contains(strings.Split(path, "/")[0], ".") == strings.Contains(strings.Split(existing, "/")[0], ".") {
		score = 1
	}

	left, right := strings.Split(path, "/"), strings.Split(existing, "/")
	for i := 0; i < len(left) && i < len(right) && left[i] == right[i]; i++ {
		score += 2
	}

	return score
}
