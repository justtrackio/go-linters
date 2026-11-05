package linters

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
)

// checkGosoLoggerErrors owns the log-level policy and its literal-only fix.
func checkGosoLoggerErrors(pass *analysis.Pass, call gosoLoggerCall) {
	if call.expr.Ellipsis.IsValid() {
		return
	}

	wanted := byte('s')
	if call.level == "Error" {
		wanted = 'w'
	}

	formatExpr := call.expr.Args[call.formatIndex]

	value := pass.TypesInfo.Types[formatExpr].Value
	if value == nil || value.Kind() != constant.String {
		return
	}

	format := constant.StringVal(value)

	verbs, valid := parseLogFormat(format)
	if !valid {
		return
	}

	errorType := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
	args := call.expr.Args[call.formatIndex+1:]
	corrected := []byte(format)
	changed := false

	for _, verb := range verbs {
		if verb.argument < 0 || verb.argument >= len(args) || verb.verb == rune(wanted) {
			continue
		}

		argType := pass.TypesInfo.TypeOf(args[verb.argument])
		if argType == nil || !types.Implements(argType, errorType) {
			continue
		}

		// Replacing the verb leaves indexes, flags, width, and precision intact.
		_, size := utf8.DecodeRuneInString(format[verb.offset:])
		if size != 1 {
			return // An invalid multibyte verb cannot be replaced byte-for-byte.
		}

		corrected[verb.offset] = wanted
		changed = true
	}

	if !changed {
		return
	}

	diagnostic := analysis.Diagnostic{
		Pos:      formatExpr.Pos(),
		End:      formatExpr.End(),
		Category: "error-directive",
		Message:  "error arguments to logger." + call.level + " must use %" + string(wanted),
	}

	if literal, ok := ast.Unparen(formatExpr).(*ast.BasicLit); ok && literal.Kind == token.STRING {
		diagnostic.SuggestedFixes = []analysis.SuggestedFix{{
			Message: "Use %" + string(wanted) + " for error arguments",
			TextEdits: []analysis.TextEdit{{
				Pos: literal.Pos(), End: literal.End(), NewText: []byte(strconv.Quote(string(corrected))),
			}},
		}}
	}

	pass.Report(diagnostic)
}

type logFormatVerb struct {
	argument int
	offset   int
	verb     rune
}

// parseLogFormat follows fmt's argument cursor, including indexes and the
// arguments consumed by * width and precision. %% never consumes a value.
func parseLogFormat(format string) ([]logFormatVerb, bool) {
	var verbs []logFormatVerb
	argument := 0

	for offset := 0; offset < len(format); {
		if format[offset] != '%' {
			offset++
			continue
		}

		offset++
		for offset < len(format) && strings.ContainsRune("+#- 0", rune(format[offset])) {
			offset++
		}

		if !logFormatIndex(format, &offset, &argument) {
			return nil, false
		}

		logFormatWidth(format, &offset, &argument)
		if offset < len(format) && format[offset] == '.' {
			offset++
			if !logFormatIndex(format, &offset, &argument) {
				return nil, false
			}

			logFormatWidth(format, &offset, &argument)
		}

		if !logFormatIndex(format, &offset, &argument) || offset >= len(format) {
			return nil, false
		}

		verb, size := utf8.DecodeRuneInString(format[offset:])
		if verb != '%' {
			verbs = append(verbs, logFormatVerb{argument: argument, offset: offset, verb: verb})
			argument++
		}

		offset += size
	}

	return verbs, true
}

func logFormatIndex(format string, offset, argument *int) bool {
	if *offset >= len(format) || format[*offset] != '[' {
		return true
	}

	end := strings.IndexByte(format[*offset:], ']')
	if end < 0 {
		return false
	}

	index, err := strconv.Atoi(format[*offset+1 : *offset+end])
	if err != nil || index <= 0 {
		return false
	}

	*argument = index - 1
	*offset += end + 1
	return true
}

func logFormatWidth(format string, offset, argument *int) {
	if *offset < len(format) && format[*offset] == '*' {
		*argument++
		*offset++
		return
	}

	for *offset < len(format) && format[*offset] >= '0' && format[*offset] <= '9' {
		*offset++
	}
}
