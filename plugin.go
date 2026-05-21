package linters

import (
	"go/ast"
	"go/token"
	"go/types"
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

type Plugin struct{}

func New(_ any) (register.LinterPlugin, error) {
	return &Plugin{}, nil
}

func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		{
			Name: "iferrinline",
			Doc:  "reports err assignments followed by `if err != nil` that can be inlined as `if err := ...; err != nil`",
			Run:  p.run,
		},
	}, nil
}

func (p *Plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

type candidate struct {
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

func (p *Plugin) run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		imports := buildImportMap(file)

		var candidates []candidate
		ast.Inspect(file, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			for i := 0; i < len(block.List)-1; i++ {
				if c, ok := matchPattern(pass, file, block, i); ok {
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
		for fnBody, idxs := range hoistByFn {
			if edit, ok := computeDeclEdit(pass, imports, candidates, idxs); ok {
				declEditByFn[fnBody] = edit
				declOkByFn[fnBody] = true
			}
		}

		varBlockEmitted := map[*ast.BlockStmt]bool{}
		for _, c := range candidates {
			msg := "if err can be inlined into the assignment"
			if len(c.hoistNames) > 0 {
				hoistDisplay := make([]string, len(c.hoistNames))
				for i, name := range c.hoistNames {
					if r, ok := c.renames[name]; ok {
						hoistDisplay[i] = name + " (renamed to " + r + " to avoid shadowing)"
					} else {
						hoistDisplay[i] = name
					}
				}
				msg = "if err can be inlined by hoisting " + strings.Join(hoistDisplay, ", ") +
					" to var declarations at the top of the function and changing := to ="
			}
			diag := analysis.Diagnostic{
				Pos:      c.assign.Pos(),
				End:      c.ifStmt.End(),
				Category: "iferrinline",
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

func matchPattern(pass *analysis.Pass, file *ast.File, block *ast.BlockStmt, i int) (candidate, bool) {
	var zero candidate

	assign, ok := block.List[i].(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE {
		return zero, false
	}
	if len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
		return zero, false
	}

	var (
		hasErr     bool
		companions []string
	)
	for _, e := range assign.Lhs {
		id, ok := e.(*ast.Ident)
		if !ok {
			return zero, false
		}
		switch id.Name {
		case "err":
			hasErr = true
		case "_":
		default:
			companions = append(companions, id.Name)
		}
	}
	if !hasErr {
		return zero, false
	}
	if pass.Fset.Position(assign.Pos()).Line != pass.Fset.Position(assign.End()).Line {
		return zero, false
	}

	ifStmt, ok := block.List[i+1].(*ast.IfStmt)
	if !ok || ifStmt.Init != nil {
		return zero, false
	}
	if !isErrNotNil(ifStmt.Cond) {
		return zero, false
	}

	rest := block.List[i+2:]
	var hoist []string
	for _, name := range companions {
		if usedAfter(rest, name) {
			hoist = append(hoist, name)
		}
	}
	if len(hoist) == 0 && usedAfter(rest, "err") {
		return zero, false
	}

	c := candidate{assign: assign, ifStmt: ifStmt, hoistNames: hoist}
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

func computeDeclEdit(pass *analysis.Pass, imports map[string]string, candidates []candidate, idxs []int) (declEdit, bool) {
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
		missing      bool
	)
	for name := range allNames {
		seen[name] = true
	}
	qualPkgs := func(p *types.Package) string {
		if p == nil || p == pass.Pkg {
			return ""
		}
		alias, ok := imports[p.Path()]
		if !ok {
			missing = true
			return p.Name()
		}
		if alias == "" || alias == "." {
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
			typeStr := types.TypeString(obj.Type(), qualPkgs)
			if missing {
				return declEdit{}, false
			}
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
				typeStr := types.TypeString(obj.Type(), qualPkgs)
				if missing {
					return declEdit{}, false
				}
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

func isErrNotNil(expr ast.Expr) bool {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	x, xok := bin.X.(*ast.Ident)
	y, yok := bin.Y.(*ast.Ident)
	if !xok || !yok {
		return false
	}
	return x.Name == "err" && y.Name == "nil"
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
