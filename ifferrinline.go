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
	"unicode"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("iferrinline", New)
}

type IfErrInline struct{}

func New(_ any) (register.LinterPlugin, error) {
	return &IfErrInline{}, nil
}

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

type candidate struct {
	checkName  string
	assign     *ast.AssignStmt
	ifStmt     *ast.IfStmt
	hoistNames []string
	fnBody     *ast.BlockStmt
	renames    map[string]string // original LHS name → fresh name (when hoisting would shadow)
}

type declEdit struct {
	pos, end token.Pos
	text     string
}

func (p *IfErrInline) run(pass *analysis.Pass) (any, error) {
	return runInlineCheck(pass, "iferrinline", checkedError)
}

// runInlineCheck shares scope preservation and suggested fixes between the
// error and boolean check analyzers.
func runInlineCheck(pass *analysis.Pass, analyzerName string, checkedVariable func(*analysis.Pass, ast.Expr) *ast.Ident) (any, error) {
	for _, file := range pass.Files {
		imports := buildImportMap(file)
		originalImports := maps.Clone(imports)
		usedImportNames := collectUsedNamesInFile(file)

		var candidates []candidate
		ast.Inspect(file, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i := 0; i < len(block.List)-1; i++ {
				if c, ok := matchPattern(pass, file, block, i, checkedVariable); ok {
					candidates = append(candidates, c)
				}
			}
			return true
		})

		hoistByFn := map[*ast.BlockStmt][]int{}
		for idx, c := range candidates {
			if len(c.hoistNames) > 0 && c.fnBody != nil {
				hoistByFn[c.fnBody] = append(hoistByFn[c.fnBody], idx)
			}
		}

		declEditByFn := map[*ast.BlockStmt]declEdit{}
		declOkByFn := map[*ast.BlockStmt]bool{}
		for _, c := range candidates {
			fnBody := c.fnBody
			if fnBody == nil || declOkByFn[fnBody] {
				continue
			}
			idxs := hoistByFn[fnBody]
			if edit, ok := computeDeclEdit(pass, file, imports, usedImportNames, candidates, idxs); ok {
				declEditByFn[fnBody] = edit
				declOkByFn[fnBody] = true
			}
		}

		importEdits := buildImportEdits(file, originalImports, imports)
		varBlockEmitted := map[*ast.BlockStmt]bool{}
		for _, c := range candidates {
			checkName := c.checkName
			msg := "if " + checkName + " can be inlined into the assignment"
			if len(c.hoistNames) > 0 {
				hoistDisplay := make([]string, len(c.hoistNames))
				for i, name := range c.hoistNames {
					if r, ok := c.renames[name]; ok {
						hoistDisplay[i] = name + " (renamed to " + r + " to avoid shadowing)"
					} else {
						hoistDisplay[i] = name
					}
				}
				msg = "if " + checkName + " can be inlined by hoisting " + strings.Join(hoistDisplay, ", ") +
					" to var declarations at the top of the function and changing := to ="
			}
			diag := analysis.Diagnostic{
				Pos:      c.assign.Pos(),
				End:      c.ifStmt.End(),
				Category: analyzerName,
				Message:  msg,
			}

			if len(c.hoistNames) == 0 {
				if fix, ok := buildInlineFix(pass, c.assign, c.ifStmt); ok {
					diag.SuggestedFixes = []analysis.SuggestedFix{fix}
				}
			} else if declOkByFn[c.fnBody] {
				edit := declEditByFn[c.fnBody]
				includeVarBlock := edit.text != "" && !varBlockEmitted[c.fnBody]
				if fix, ok := buildHoistFix(pass, c, edit, includeVarBlock); ok {
					fix.TextEdits = append(fix.TextEdits, importEdits...)
					importEdits = nil
					diag.SuggestedFixes = []analysis.SuggestedFix{fix}
					if includeVarBlock {
						varBlockEmitted[c.fnBody] = true
					}
				}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

func matchPattern(pass *analysis.Pass, file *ast.File, block *ast.BlockStmt, i int, checkedVariable func(*analysis.Pass, ast.Expr) *ast.Ident) (candidate, bool) {
	var zero candidate

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

	var (
		hasCheck   bool
		companions []string
	)
	for _, e := range assign.Lhs {
		id, ok := e.(*ast.Ident)
		if !ok {
			return zero, false
		}
		switch id.Name {
		case checkName:
			if pass.TypesInfo == nil || pass.TypesInfo.ObjectOf(id) == nil || pass.TypesInfo.ObjectOf(id) != pass.TypesInfo.ObjectOf(checked) {
				return zero, false
			}
			hasCheck = true
		case "_":
		default:
			companions = append(companions, id.Name)
		}
	}
	if !hasCheck {
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
	var hoist []string
	for _, name := range companions {
		if usedAfter(rest, name) {
			hoist = append(hoist, name)
		}
	}
	if len(hoist) == 0 && usedAfter(rest, checkName) {
		return zero, false
	}

	c := candidate{checkName: checkName, assign: assign, ifStmt: ifStmt, hoistNames: hoist}
	if len(hoist) > 0 {
		c.fnBody = enclosingFuncBody(file, assign.Pos())
		if c.fnBody == nil {
			return zero, false
		}
		ownObjs := candidateOwnObjs(pass, assign)
		used := collectUsedNames(c.fnBody)
		for _, name := range hoist {
			if isPartialRedecl(pass, assign, name) {
				// Companion reuses an outer-scope var (e.g. a parameter); the
				// hoist path just converts := to = without introducing a new
				// declaration, so no shadowing is possible.
				continue
			}
			if !hoistWouldShadow(pass, c.fnBody, name, ownObjs) {
				continue
			}
			obj := lhsObj(pass, assign, name)
			newName := pickFreshName(name, obj, used)
			if newName == "" {
				return zero, false
			}
			if c.renames == nil {
				c.renames = map[string]string{}
			}
			c.renames[name] = newName
			used[newName] = true
		}
	}
	return c, true
}

func isPartialRedecl(pass *analysis.Pass, assign *ast.AssignStmt, name string) bool {
	if pass.TypesInfo == nil {
		return false
	}
	for _, e := range assign.Lhs {
		id, ok := e.(*ast.Ident)
		if !ok || id.Name != name {
			continue
		}
		return pass.TypesInfo.Defs[id] == nil
	}
	return false
}

func candidateOwnObjs(pass *analysis.Pass, assign *ast.AssignStmt) map[types.Object]bool {
	objs := map[types.Object]bool{}
	if pass.TypesInfo == nil {
		return objs
	}
	for _, e := range assign.Lhs {
		id, ok := e.(*ast.Ident)
		if !ok {
			continue
		}
		if obj := pass.TypesInfo.Defs[id]; obj != nil {
			objs[obj] = true
		}
	}
	return objs
}

func lhsObj(pass *analysis.Pass, assign *ast.AssignStmt, name string) types.Object {
	if pass.TypesInfo == nil {
		return nil
	}
	for _, e := range assign.Lhs {
		id, ok := e.(*ast.Ident)
		if !ok || id.Name != name {
			continue
		}
		if obj := pass.TypesInfo.Defs[id]; obj != nil {
			return obj
		}
	}
	return nil
}

func collectUsedNames(fnBody *ast.BlockStmt) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(fnBody, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			names[id.Name] = true
		}
		return true
	})
	return names
}

// pickFreshName returns a name that doesn't collide with anything already
// referenced in the function body. It tries a type-derived name first
// (e.g. *tdm.TdmService → tdmService) and falls back to numeric suffixes.
func pickFreshName(original string, obj types.Object, used map[string]bool) string {
	tryName := func(n string) bool {
		if n == "" || n == original || used[n] {
			return false
		}
		return types.Universe.Lookup(n) == nil
	}
	if obj != nil {
		if base := baseTypeName(obj.Type()); tryName(base) {
			return base
		}
	}
	for i := 2; i < 1000; i++ {
		n := original + strconv.Itoa(i)
		if tryName(n) {
			return n
		}
	}
	return ""
}

func baseTypeName(t types.Type) string {
	for i := 0; i < 16 && t != nil; i++ {
		switch x := t.(type) {
		case *types.Pointer:
			t = x.Elem()
		case *types.Named:
			n := x.Obj().Name()
			if n == "" {
				return ""
			}
			r := []rune(n)
			r[0] = unicode.ToLower(r[0])
			return string(r)
		default:
			return ""
		}
	}
	return ""
}

// hoistWouldShadow reports whether introducing `var name ...` at the top of
// fnBody would change the meaning of an existing reference. Hoisting moves a
// binding from the inner :=' scope (effective after the assignment) to the
// outer function-body scope (effective from the very start of the body), so
// any reference to `name` that originally resolved to a parameter, import,
// or other outer-scope binding would now resolve to the new variable.
func hoistWouldShadow(pass *analysis.Pass, fnBody *ast.BlockStmt, name string, ownObjs map[types.Object]bool) bool {
	if pass.TypesInfo == nil {
		return true
	}

	skipSel := map[*ast.Ident]bool{}
	ast.Inspect(fnBody, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel != nil {
			skipSel[sel.Sel] = true
		}
		return true
	})

	var shadow bool
	ast.Inspect(fnBody, func(n ast.Node) bool {
		if shadow {
			return false
		}
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != name || skipSel[id] {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil {
			return true
		}
		if ownObjs[obj] {
			return true
		}
		if v, ok := obj.(*types.Var); ok && v.IsField() {
			return true
		}
		if obj.Pos() < fnBody.Pos() || obj.Pos() >= fnBody.End() {
			shadow = true
			return false
		}
		return true
	})
	return shadow
}

func computeDeclEdit(pass *analysis.Pass, file *ast.File, imports map[string]string, usedImportNames map[string]bool, candidates []candidate, idxs []int) (declEdit, bool) {
	if pass.TypesInfo == nil {
		return declEdit{}, false
	}

	fnBody := candidates[idxs[0]].fnBody
	if fnBody == nil {
		return declEdit{}, false
	}

	allNames, topStmts := collectVarContext(fnBody)

	type entry struct {
		name string
		typ  string
	}
	var (
		seen         = map[string]bool{}
		newCollected []entry
	)
	for name := range allNames {
		seen[name] = true
	}
	for name := range functionScopeNames(file, fnBody) {
		seen[name] = true
	}
	qualPkgs := func(p *types.Package) string {
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
		if alias == "." {
			return ""
		}
		if alias == "" {
			return p.Name()
		}
		return alias
	}

	for _, idx := range idxs {
		c := candidates[idx]
		for _, e := range c.assign.Lhs {
			id, ok := e.(*ast.Ident)
			if !ok {
				return declEdit{}, false
			}
			if id.Name == "_" {
				continue
			}
			name := id.Name
			if r, ok := c.renames[name]; ok {
				name = r
			}
			if seen[name] {
				continue
			}
			obj := pass.TypesInfo.Defs[id]
			if obj == nil {
				continue
			}
			declType, ok := accessibleHoistType(obj.Type(), pass.Pkg)
			if !ok {
				return declEdit{}, false
			}
			typeStr := types.TypeString(declType, qualPkgs)
			seen[name] = true
			newCollected = append(newCollected, entry{name: name, typ: typeStr})
		}
	}

	if len(newCollected) == 0 {
		return declEdit{}, true
	}

	var existingCollected []entry
	for _, stmt := range topStmts {
		decl := stmt.(*ast.DeclStmt)
		gen := decl.Decl.(*ast.GenDecl)
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				obj := pass.TypesInfo.Defs[n]
				if obj == nil {
					continue
				}
				declType, ok := accessibleHoistType(obj.Type(), pass.Pkg)
				if !ok {
					return declEdit{}, false
				}
				typeStr := types.TypeString(declType, qualPkgs)
				existingCollected = append(existingCollected, entry{name: n.Name, typ: typeStr})
			}
		}
	}

	combined := append(existingCollected, newCollected...)
	sort.SliceStable(combined, func(i, j int) bool {
		return varPriority(combined[i].name) < varPriority(combined[j].name)
	})

	var b strings.Builder
	var pos, end token.Pos
	if len(topStmts) > 0 {
		pos = topStmts[0].Pos()
		end = topStmts[len(topStmts)-1].End()
		for i, e := range combined {
			if i > 0 {
				b.WriteString("\n\t")
			}
			b.WriteString("var " + e.name + " " + e.typ)
		}
	} else {
		pos = fnBody.Lbrace + 1
		end = pos
		for _, e := range combined {
			b.WriteString("\n\tvar " + e.name + " " + e.typ)
		}
		b.WriteString("\n")
	}
	return declEdit{pos: pos, end: end, text: b.String()}, true
}

// accessibleHoistType avoids spelling private imported types in generated declarations.
// Only interfaces preserving the complete public method set are considered; a
// narrower interface could remove methods needed by subsequent statements.
func accessibleHoistType(typ types.Type, current *types.Package) (types.Type, bool) {
	if hoistTypeAccessible(typ, current) {
		return typ, true
	}
	base := types.Unalias(typ)
	if ptr, ok := base.(*types.Pointer); ok {
		base = types.Unalias(ptr.Elem())
	}
	named, ok := base.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return nil, false
	}
	methods := types.NewMethodSet(typ)
	publicMethods := 0
	for i := 0; i < methods.Len(); i++ {
		if methods.At(i).Obj().Exported() {
			publicMethods++
		}
	}
	if publicMethods == 0 {
		return nil, false
	}
	var replacement types.Type
	scope := named.Obj().Pkg().Scope()
	for _, name := range scope.Names() {
		obj, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !obj.Exported() || !hoistTypeAccessible(obj.Type(), current) {
			continue
		}
		iface, ok := obj.Type().Underlying().(*types.Interface)
		if !ok || !iface.IsMethodSet() || iface.NumMethods() != publicMethods || !types.Implements(typ, iface) {
			continue
		}
		if replacement != nil && !types.Identical(replacement, obj.Type()) {
			// Multiple public contracts are ambiguous: keep the inferred binding.
			return nil, false
		}
		replacement = obj.Type()
	}
	return replacement, replacement != nil
}

func hoistTypeAccessible(typ types.Type, current *types.Package) bool {
	// Parsing the rendered type also covers nested pointers, containers,
	// function signatures, aliases, and generic type arguments.
	expr, err := parser.ParseExpr(types.TypeString(typ, func(pkg *types.Package) string {
		if pkg == current {
			return ""
		}
		return "external"
	}))
	if err != nil {
		return false
	}
	accessible := true
	ast.Inspect(expr, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && !selector.Sel.IsExported() {
			accessible = false
		}
		return accessible
	})
	return accessible
}

func collectVarContext(fnBody *ast.BlockStmt) (allNames map[string]bool, topStmts []ast.Stmt) {
	allNames = map[string]bool{}

	for _, stmt := range fnBody.List {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			continue
		}
		gen, ok := decl.Decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				allNames[n.Name] = true
			}
		}
	}

	for _, stmt := range fnBody.List {
		decl, ok := stmt.(*ast.DeclStmt)
		if !ok {
			break
		}
		gen, ok := decl.Decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			break
		}
		topStmts = append(topStmts, stmt)
	}
	return
}

func buildInlineFix(pass *analysis.Pass, assign *ast.AssignStmt, ifStmt *ast.IfStmt) (analysis.SuggestedFix, bool) {
	if pass.ReadFile == nil {
		return analysis.SuggestedFix{}, false
	}
	filename := pass.Fset.Position(assign.Pos()).Filename
	src, err := pass.ReadFile(filename)
	if err != nil {
		return analysis.SuggestedFix{}, false
	}
	startOff := pass.Fset.Position(assign.Pos()).Offset
	endOff := pass.Fset.Position(assign.End()).Offset
	if startOff < 0 || endOff > len(src) || startOff >= endOff {
		return analysis.SuggestedFix{}, false
	}
	assignText := append([]byte(nil), src[startOff:endOff]...)
	insert := append(assignText, []byte("; ")...)
	return analysis.SuggestedFix{
		Message: "inline assignment into the if statement",
		TextEdits: []analysis.TextEdit{
			{Pos: assign.Pos(), End: ifStmt.Pos()},
			{Pos: ifStmt.Cond.Pos(), End: ifStmt.Cond.Pos(), NewText: insert},
		},
	}, true
}

func buildHoistFix(pass *analysis.Pass, c candidate, edit declEdit, includeVarBlock bool) (analysis.SuggestedFix, bool) {
	if pass.ReadFile == nil {
		return analysis.SuggestedFix{}, false
	}
	filename := pass.Fset.Position(c.assign.Pos()).Filename
	src, err := pass.ReadFile(filename)
	if err != nil {
		return analysis.SuggestedFix{}, false
	}
	startOff := pass.Fset.Position(c.assign.Pos()).Offset
	tokOff := pass.Fset.Position(c.assign.TokPos).Offset
	endOff := pass.Fset.Position(c.assign.End()).Offset
	if startOff < 0 || endOff > len(src) || tokOff+2 > endOff || tokOff < startOff {
		return analysis.SuggestedFix{}, false
	}

	var lhsText []byte
	if len(c.renames) == 0 {
		lhsText = src[startOff:tokOff]
	} else {
		parts := make([]string, 0, len(c.assign.Lhs))
		for _, e := range c.assign.Lhs {
			id, ok := e.(*ast.Ident)
			if !ok {
				return analysis.SuggestedFix{}, false
			}
			n := id.Name
			if r, ok := c.renames[n]; ok {
				n = r
			}
			parts = append(parts, n)
		}
		lhsText = []byte(strings.Join(parts, ", ") + " ")
	}
	rhsText := src[tokOff+2 : endOff]

	insert := make([]byte, 0, len(lhsText)+1+len(rhsText)+2)
	insert = append(insert, lhsText...)
	insert = append(insert, '=')
	insert = append(insert, rhsText...)
	insert = append(insert, []byte("; ")...)

	edits := make([]analysis.TextEdit, 0, 8)
	if includeVarBlock && edit.text != "" {
		edits = append(edits, analysis.TextEdit{
			Pos:     edit.pos,
			End:     edit.end,
			NewText: []byte(edit.text),
		})
	}
	edits = append(edits,
		analysis.TextEdit{Pos: c.assign.Pos(), End: c.ifStmt.Pos()},
		analysis.TextEdit{Pos: c.ifStmt.Cond.Pos(), End: c.ifStmt.Cond.Pos(), NewText: insert},
		analysis.TextEdit{Pos: c.ifStmt.End(), End: c.ifStmt.End(), NewText: []byte("\n")},
	)

	if len(c.renames) > 0 && pass.TypesInfo != nil {
		renameByObj := map[types.Object]string{}
		for orig, newName := range c.renames {
			if obj := lhsObj(pass, c.assign, orig); obj != nil {
				renameByObj[obj] = newName
			}
		}
		assignStart := c.assign.Pos()
		assignEnd := c.assign.End()
		ast.Inspect(c.fnBody, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if id.Pos() >= assignStart && id.End() <= assignEnd {
				return true
			}
			obj := pass.TypesInfo.Uses[id]
			if obj == nil {
				return true
			}
			if newName, ok := renameByObj[obj]; ok {
				edits = append(edits, analysis.TextEdit{
					Pos:     id.Pos(),
					End:     id.End(),
					NewText: []byte(newName),
				})
			}
			return true
		})
	}

	return analysis.SuggestedFix{
		Message:   "hoist to var declarations and inline assignment",
		TextEdits: edits,
	}, true
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
	m := make(map[string]string, len(file.Imports))
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
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

// functionScopeNames returns names declared at the function-body scope of
// fnBody — i.e. receiver, parameters, and named return values. Per the Go
// spec, these share a block with the function body, so introducing a `var X`
// at the top of the body for any of those names would be a redeclaration.
func functionScopeNames(file *ast.File, fnBody *ast.BlockStmt) map[string]bool {
	names := map[string]bool{}
	var (
		recv    *ast.FieldList
		params  *ast.FieldList
		results *ast.FieldList
		found   bool
	)
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body == fnBody {
				recv = x.Recv
				if x.Type != nil {
					params = x.Type.Params
					results = x.Type.Results
				}
				found = true
				return false
			}
		case *ast.FuncLit:
			if x.Body == fnBody {
				if x.Type != nil {
					params = x.Type.Params
					results = x.Type.Results
				}
				found = true
				return false
			}
		}
		return true
	})
	add := func(list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, field := range list.List {
			for _, ident := range field.Names {
				if ident.Name != "" && ident.Name != "_" {
					names[ident.Name] = true
				}
			}
		}
	}
	add(recv)
	add(params)
	add(results)
	return names
}

func enclosingFuncBody(file *ast.File, pos token.Pos) *ast.BlockStmt {
	var result *ast.BlockStmt
	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt
		switch x := n.(type) {
		case *ast.FuncDecl:
			body = x.Body
		case *ast.FuncLit:
			body = x.Body
		default:
			return true
		}
		if body == nil {
			return true
		}
		if body.Pos() <= pos && pos < body.End() {
			result = body
			return true
		}
		return false
	})
	return result
}

func varPriority(name string) int {
	switch name {
	case "ok":
		return 0
	case "err":
		return 1
	default:
		return 2
	}
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
			for _, imp := range file.Imports {
				if imp.Path.Value == strconv.Quote(path) {
					edits = append(edits, analysis.TextEdit{Pos: imp.Name.Pos(), End: imp.Name.End(), NewText: []byte(alias)})
				}
			}
		}
	}
	sort.Strings(paths)
	if len(paths) > 0 {
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
			cgo := false
			for _, spec := range gen.Specs {
				if spec.(*ast.ImportSpec).Path.Value == strconv.Quote("C") {
					cgo = true
				}
			}
			if cgo {
				continue
			}
			if gen.Lparen.IsValid() {
				insertions := map[token.Pos]string{}
				for _, path := range paths {
					pos := gen.Lparen + 1
					bestScore := -1
					for _, spec := range gen.Specs {
						imp := spec.(*ast.ImportSpec)
						existing, _ := strconv.Unquote(imp.Path.Value)
						score := importMatchScore(path, existing)
						if score > bestScore {
							bestScore = score
							pos = imp.End()
							if imp.Comment != nil {
								pos = imp.Comment.End()
							}
						}
					}
					insertions[pos] += "\n\t" + importText(path, updated[path])
				}
				var positions []token.Pos
				for pos := range insertions {
					positions = append(positions, pos)
				}
				sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
				for _, pos := range positions {
					edits = append(edits, analysis.TextEdit{Pos: pos, End: pos, NewText: []byte(insertions[pos])})
				}
			} else {
				spec := gen.Specs[0].(*ast.ImportSpec)
				end := spec.End()
				if spec.Comment != nil {
					end = spec.Comment.End()
				}
				edits = append(edits,
					analysis.TextEdit{Pos: spec.Pos(), End: spec.Pos(), NewText: []byte("(" + additions.String())},
					analysis.TextEdit{Pos: end, End: end, NewText: []byte("\n)")},
				)
			}
			return edits
		}
		edits = append(edits, analysis.TextEdit{Pos: file.Name.End(), End: file.Name.End(), NewText: []byte("\n\nimport (" + additions.String() + ")")})
	}
	return edits
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
