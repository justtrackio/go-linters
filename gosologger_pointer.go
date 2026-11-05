package linters

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// checkGosoLoggerPointers checks logged arguments even when the format is
// dynamic. The logger receiver and context are not logged arguments.
func checkGosoLoggerPointers(pass *analysis.Pass, call gosoLoggerCall) {
	args := call.expr.Args[call.formatIndex+1:]
	if call.expr.Ellipsis.IsValid() {
		// Literal expansions expose their element types without dataflow.
		expansion := args[len(args)-1]
		args = args[: len(args)-1 : len(args)-1]

		if literal, ok := ast.Unparen(expansion).(*ast.CompositeLit); ok {
			for _, element := range literal.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					element = pair.Value
				}

				args = append(args, element)
			}
		}
	}

	for _, arg := range args {
		typ := pass.TypesInfo.TypeOf(arg)
		if typ == nil {
			continue
		}

		if isLogPointer(typ) {
			pass.Report(analysis.Diagnostic{
				Pos: arg.Pos(), End: arg.End(), Category: "pointer",
				Message: "logger." + call.level + " must not log pointer argument of type " + types.TypeString(typ, nil) + "; log a value instead",
			})
		}
	}

	value := pass.TypesInfo.Types[call.expr.Args[call.formatIndex]].Value
	if value == nil || value.Kind() != constant.String {
		return
	}

	verbs, _ := parseLogFormat(constant.StringVal(value))
	for _, verb := range verbs {
		if verb.verb == 'p' {
			pass.Report(analysis.Diagnostic{
				Pos: call.expr.Args[call.formatIndex].Pos(), End: call.expr.Args[call.formatIndex].End(), Category: "pointer",
				Message: "logger." + call.level + " must not use pointer formatting %p; log a value instead",
			})
			break
		}
	}
}

func isLogPointer(typ types.Type) bool {
	if param, ok := types.Unalias(typ).(*types.TypeParam); ok {
		constraint := param.Constraint().Underlying().(*types.Interface)
		return logConstraintHasPointer(constraint, constraint)
	}

	switch typ := typ.Underlying().(type) {
	case *types.Pointer:
		return true
	case *types.Basic:
		return typ.Kind() == types.UnsafePointer
	}

	return false
}

// Check pointer terms against the whole constraint so an intersection that
// excludes pointers does not produce a false positive.
func logConstraintHasPointer(typ types.Type, constraint *types.Interface) bool {
	candidate := typ
	switch typ := typ.Underlying().(type) {
	case *types.Interface:
		for i := 0; i < typ.NumEmbeddeds(); i++ {
			if logConstraintHasPointer(typ.EmbeddedType(i), constraint) {
				return true
			}
		}
	case *types.Union:
		for i := 0; i < typ.Len(); i++ {
			if logConstraintHasPointer(typ.Term(i).Type(), constraint) {
				return true
			}
		}
	case *types.Pointer:
		return types.Satisfies(candidate, constraint)
	case *types.Basic:
		return typ.Kind() == types.UnsafePointer && types.Satisfies(candidate, constraint)
	}

	return false
}
