package linters

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/ast/inspector"
)

// checkGosoLoggerFormats delegates fmt semantics to Go's printf analyzer.
// Its isolated syntax view represents logging calls as fmt.Errorf calls:
// this permits %w to be type-checked, while the separate error check owns
// which log levels may use it. Neither the source AST nor printf's global
// function registry is modified, so enabling the standard printf linter too
// does not change the behavior of either analyzer.
func checkGosoLoggerFormats(pass *analysis.Pass, calls map[*ast.File][]gosoLoggerCall) error {
	var ok bool
	var signature *types.Signature

	if len(calls) == 0 {
		return nil
	}

	info := *pass.TypesInfo
	info.Types = maps.Clone(info.Types)
	info.FileVersions = make(map[*ast.File]string, len(pass.TypesInfo.FileVersions))
	maps.Copy(info.FileVersions, pass.TypesInfo.FileVersions)
	info.Uses = maps.Clone(info.Uses)
	info.Selections = make(map[*ast.SelectorExpr]*types.Selection)

	// Nested ordinary calls are arguments, not additional checking targets.
	// Keep variable uses for printf's recursive Stringer check.
	for ident, object := range info.Uses {
		if _, ok := object.(*types.Func); ok {
			delete(info.Uses, ident)
		}
	}

	var files []*ast.File
	var targets []gosoLoggerCall
	fmtPackage := types.NewPackage("fmt", "fmt")

	for _, file := range pass.Files {
		var values []ast.Expr

		for _, call := range calls[file] {
			value := info.Types[call.expr.Args[call.formatIndex]].Value
			if value == nil || value.Kind() != constant.String {
				continue
			}

			if signature, ok = info.TypeOf(call.expr.Fun).(*types.Signature); !ok {
				continue
			}

			function := &ast.Ident{Name: "Errorf", NamePos: call.expr.Fun.Pos()}
			// Retain the original parameter layout (context, and explicit
			// receiver for method expressions) so format argument indexes and
			// source positions still refer to the original call.
			signature = types.NewSignatureType(nil, nil, nil, signature.Params(), signature.Results(), signature.Variadic())
			info.Uses[function] = types.NewFunc(function.Pos(), fmtPackage, "Errorf", signature)
			info.Types[function] = types.TypeAndValue{Type: signature}
			expr := *call.expr
			expr.Fun = function
			values = append(values, &expr)
			targets = append(targets, call)
		}

		if len(values) == 0 {
			continue
		}

		view := *file
		view.Decls = []ast.Decl{&ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Values: values}}}}
		info.FileVersions[&view] = info.FileVersions[file]
		files = append(files, &view)
	}

	if len(files) == 0 {
		return nil
	}

	printfPass := *pass
	printfPass.Analyzer = printf.Analyzer
	printfPass.Files = files
	printfPass.TypesInfo = &info
	printfPass.ResultOf = map[*analysis.Analyzer]any{inspect.Analyzer: inspector.New(files)}
	printfPass.ImportObjectFact = func(types.Object, analysis.Fact) bool { return false }
	printfPass.ExportObjectFact = func(types.Object, analysis.Fact) {}
	printfPass.Report = func(diagnostic analysis.Diagnostic) {
		// Pick the smallest enclosing call when logging calls are nested.
		var target *gosoLoggerCall

		for i := range targets {
			call := &targets[i]
			if call.expr.Pos() <= diagnostic.Pos && diagnostic.Pos < call.expr.End() &&
				(target == nil || call.expr.End()-call.expr.Pos() < target.expr.End()-target.expr.Pos()) {
				target = call
			}
		}

		if target == nil {
			return
		}

		diagnostic.Category = "format"
		diagnostic.Message = strings.Replace(diagnostic.Message, "fmt.Errorf", "logger."+target.level, 1)
		pass.Report(diagnostic)
	}
	_, err := printf.Analyzer.Run(&printfPass)
	return err
}
