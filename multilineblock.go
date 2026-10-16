package linters

import (
	"bytes"
	"go/ast"
	"go/token"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("multilineblock", NewMultilineBlock)
}

// MultilineBlockPlugin separates multiline statement groups from control blocks.
type MultilineBlockPlugin struct{}

func NewMultilineBlock(_ any) (register.LinterPlugin, error) {
	return &MultilineBlockPlugin{}, nil
}

func (p *MultilineBlockPlugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (p *MultilineBlockPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "multilineblock",
		Doc:  "requires blank lines after blocks and before control blocks following multiline statement groups",
		Run:  runMultilineBlock,
	}}, nil
}

func runMultilineBlock(pass *analysis.Pass) (any, error) {
	var err error
	var source []byte

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		tokenFile := pass.Fset.File(file.Pos())

		if source, err = pass.ReadFile(tokenFile.Name()); err != nil {
			return nil, err
		}

		lines := bytes.Split(source, []byte("\n"))
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.BlockStmt:
				checkMultilineStatements(pass, tokenFile, lines, node.List)
			case *ast.CaseClause:
				checkMultilineStatements(pass, tokenFile, lines, node.Body)
			case *ast.CommClause:
				checkMultilineStatements(pass, tokenFile, lines, node.Body)
			}

			return true
		})
	}

	return nil, nil
}

func checkMultilineStatements(pass *analysis.Pass, tokenFile *token.File, lines [][]byte, statements []ast.Stmt) {
	groupStart := 0
	for i, statement := range statements {
		start := tokenFile.Line(statement.Pos())
		if i == 0 {
			groupStart = start

			continue
		}

		previousEnd := tokenFile.Line(statements[i-1].End() - 1)
		if multilineStatementsSeparated(lines, previousEnd, start) {
			groupStart = start

			continue
		}

		previous := statements[i-1]

		afterBlock := isMultilineBlock(previous)
		if afterBlock && isMultilineBlockCleanup(statements, i) {
			groupStart = start

			continue
		}

		beforeBlock := isMultilineBlock(statement) && previousEnd > groupStart && start > previousEnd
		if beforeBlock && !afterBlock && assignmentCheckedByIf(previous, statement) && tokenFile.Line(previous.Pos()) == groupStart {
			// A multiline assignment and the if checking its result form one group.
			continue
		}

		if afterBlock || beforeBlock {
			reportMultilineBoundary(pass, tokenFile, statements, i, afterBlock)
			groupStart = start
		}
	}
}

func multilineStatementsSeparated(lines [][]byte, previousEnd, start int) bool {
	for line := previousEnd + 1; line < start; line++ {
		if len(bytes.TrimSpace(lines[line-1])) == 0 {
			return true
		}
	}

	return false
}

func reportMultilineBoundary(pass *analysis.Pass, tokenFile *token.File, statements []ast.Stmt, index int, afterBlock bool) {
	statement := statements[index]
	previous := statements[index-1]
	reportPos := statement.Pos()
	boundaryEnd := tokenFile.Line(previous.End() - 1)
	boundaryStart := tokenFile.Line(statement.Pos())
	message := "missing blank line before control block after multiline statement group"

	if afterBlock {
		message = "missing blank line after block"
	} else if index >= 2 && assignmentCheckedByIf(previous, statement) {
		// Keep the assignment with the if checking its result.
		reportPos = previous.Pos()
		boundaryStart = tokenFile.Line(previous.Pos())
		boundaryEnd = tokenFile.Line(statements[index-2].End() - 1)
		message = "missing blank line before assignment checked by if"
	}

	// Insert after the previous statement's entire line, keeping
	// trailing comments there and leading comments with the block.
	pos := reportPos
	newline := []byte("\n\n")

	if boundaryStart > boundaryEnd {
		pos = tokenFile.LineStart(boundaryEnd + 1)
		newline = []byte("\n")
	}

	pass.Report(analysis.Diagnostic{
		Pos:     reportPos,
		Message: message,
		SuggestedFixes: []analysis.SuggestedFix{{
			Message:   "Insert blank line between statements",
			TextEdits: []analysis.TextEdit{{Pos: pos, End: pos, NewText: newline}},
		}},
	})
}

func assignmentCheckedByIf(previous, statement ast.Stmt) bool {
	var ok bool
	var assignment *ast.AssignStmt

	if assignment, ok = previous.(*ast.AssignStmt); !ok {
		return false
	}

	check, ok := statement.(*ast.IfStmt)
	// An if initializer may shadow names assigned above; keep its spacing normal.
	if !ok || check.Init != nil {
		return false
	}

	assigned := make(map[string]bool)
	for _, lhs := range assignment.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
			assigned[id.Name] = true
		}
	}

	return multilineBlockUsesNames(check.Cond, assigned)
}

func multilineBlockUsesNames(node ast.Node, names map[string]bool) bool {
	used := false
	var inspect func(ast.Node) bool
	inspect = func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			ast.Inspect(node.X, inspect)

			return false
		case *ast.Ident:
			used = used || names[node.Name]
		}

		return true
	}
	ast.Inspect(node, inspect)

	return used
}

func isMultilineBlock(statement ast.Stmt) bool {
	if labeled, ok := statement.(*ast.LabeledStmt); ok {
		return isMultilineBlock(labeled.Stmt)
	}

	switch statement.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt, *ast.BlockStmt:
		return true
	}

	return false
}

// Keep resource acquisition, its error check, and deferred cleanup together.
func isMultilineBlockCleanup(statements []ast.Stmt, index int) bool {
	var ok bool
	var deferred *ast.DeferStmt
	var assignment *ast.AssignStmt

	if deferred, ok = statements[index].(*ast.DeferStmt); !ok {
		return false
	}

	check, ok := statements[index-1].(*ast.IfStmt)
	if !ok || check.Else != nil {
		return false
	}

	condition, ok := ast.Unparen(check.Cond).(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}

	left, leftOK := ast.Unparen(condition.X).(*ast.Ident)

	right, rightOK := ast.Unparen(condition.Y).(*ast.Ident)
	if !leftOK || !rightOK {
		return false
	}

	checksError := (left.Name == errorVariableName && right.Name == "nil") || (left.Name == "nil" && right.Name == errorVariableName)
	if !checksError {
		return false
	}

	acquisition := check.Init
	if acquisition == nil && index >= 2 {
		acquisition = statements[index-2]
	}

	if assignment, ok = acquisition.(*ast.AssignStmt); !ok {
		return false
	}

	assigned := make(map[string]bool)
	for _, lhs := range assignment.Lhs {
		if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" && id.Name != errorVariableName {
			assigned[id.Name] = true
		}
	}

	return multilineBlockUsesNames(deferred.Call, assigned)
}
