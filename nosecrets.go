package linters

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() { register.Plugin("nosecrets", NewNoSecrets) }

// NoSecretsPlugin rejects obvious confidential inputs to diagnostic construction.
type NoSecretsPlugin struct{}

func NewNoSecrets(_ any) (register.LinterPlugin, error) { return &NoSecretsPlugin{}, nil }
func (*NoSecretsPlugin) GetLoadMode() string            { return register.LoadModeTypesInfo }
func (*NoSecretsPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: "nosecrets",
		Doc:  "prevents obvious confidential values in error and log construction; not whole-program taint analysis",
		Run:  runNoSecrets,
	}}, nil
}

func runNoSecrets(pass *analysis.Pass) (any, error) {
	state := secretAnalysis{pass: pass, aliases: make(map[types.Object][]ast.Expr)}
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch statement := node.(type) {
			case *ast.AssignStmt:
				for i, left := range statement.Lhs {
					if identifier, ok := left.(*ast.Ident); ok && i < len(statement.Rhs) {
						object := pass.TypesInfo.ObjectOf(identifier)
						state.aliases[object] = append(state.aliases[object], statement.Rhs[i])
					}
				}
			case *ast.ValueSpec:
				for i, name := range statement.Names {
					if i < len(statement.Values) {
						object := pass.TypesInfo.ObjectOf(name)
						state.aliases[object] = append(state.aliases[object], statement.Values[i])
					}
				}
			}

			return true
		})
	}

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				state.check(call)
			}

			return true
		})
	}

	return nil, nil
}

type secretAnalysis struct {
	pass    *analysis.Pass
	aliases map[types.Object][]ast.Expr
}

func (s secretAnalysis) check(call *ast.CallExpr) {
	var ok bool
	var start int

	if start, ok = s.sink(call.Fun, 0); !ok {
		return
	}

	for _, argument := range call.Args[start:] {
		if s.confidential(argument, 0) {
			s.pass.Reportf(argument.Pos(), "confidential value must not be included in an error or log; use a safe diagnostic projection or explicit redaction")
		}
	}
}

func (s secretAnalysis) sink(expression ast.Expr, depth int) (int, bool) {
	if depth > 16 {
		return 0, false
	}

	switch expr := ast.Unparen(expression).(type) {
	case *ast.Ident:
		for _, original := range s.aliases[s.pass.TypesInfo.ObjectOf(expr)] {
			if start, ok := s.sink(original, depth+1); ok {
				return start, true
			}
		}
	case *ast.SelectorExpr:
		if selected := s.pass.TypesInfo.Selections[expr]; selected != nil {
			if selected.Obj().Pkg() == nil || selected.Obj().Pkg().Path() != "github.com/justtrackio/gosoline/pkg/log" {
				return 0, false
			}

			switch expr.Sel.Name {
			case "WithFields":
				return 0, true
			case "Trace", "Debug", "Info", "Warn", "Error":
				if selected.Kind() == types.MethodExpr {
					return 2, true
				}

				return 1, true
			}
		} else if function, ok := s.pass.TypesInfo.ObjectOf(expr.Sel).(*types.Func); ok && function.Pkg() != nil {
			path, name := function.Pkg().Path(), function.Name()
			return 0, path == "fmt" && name == "Errorf" || path == "errors" && name == "New" ||
				path == "log" && (name == "Print" || name == "Printf" || name == "Println" || name == "Fatalf" || name == "Panicf")
		}
	}

	return 0, false
}

func (s secretAnalysis) confidential(expression ast.Expr, depth int) bool {
	if depth > 16 {
		return false
	}

	typ := s.pass.TypesInfo.TypeOf(expression)
	if s.safeSecretType(typ) || s.implementsError(typ) {
		return false
	}

	if s.confidentialStruct(typ, 0) {
		return true
	}

	switch expr := ast.Unparen(expression).(type) {
	case *ast.Ident:
		if s.confidentialName(expr.Name) && s.stringLike(typ) {
			return true
		}

		for _, original := range s.aliases[s.pass.TypesInfo.ObjectOf(expr)] {
			if s.confidential(original, depth+1) {
				return true
			}
		}
	case *ast.SelectorExpr:
		return s.confidentialName(expr.Sel.Name) && s.stringLike(typ)
	case *ast.BinaryExpr:
		return s.confidential(expr.X, depth+1) || s.confidential(expr.Y, depth+1)
	case *ast.CompositeLit:
		for _, item := range expr.Elts {
			if s.confidential(item, depth+1) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		key := s.pass.TypesInfo.Types[expr.Key].Value
		if key != nil && key.Kind() == constant.String && s.confidentialName(constant.StringVal(key)) &&
			s.stringLike(s.pass.TypesInfo.TypeOf(expr.Value)) && !s.safeSecretType(s.pass.TypesInfo.TypeOf(expr.Value)) {
			return true
		}

		return s.confidential(expr.Value, depth+1)
	case *ast.CallExpr:
		// An explicit conversion back to string defeats Secret's safe formatter.
		if s.stringLike(typ) {
			for _, arg := range expr.Args {
				if s.safeSecretType(s.pass.TypesInfo.TypeOf(arg)) || s.confidential(arg, depth+1) {
					return true
				}
			}
		}
	}

	return false
}

func (s secretAnalysis) confidentialName(name string) bool {
	name = strings.ToLower(strings.ReplaceAll(name, "_", ""))
	for _, suffix := range []string{"password", "passwd", "passphrase", "apikey", "clientsecret", "signingsecret", "privatekey", "accesstoken", "refreshtoken", "idtoken", "sessiontoken", "googleidtoken", "codeverifier", "authorization", "cookie"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}

	return name == "secret" || name == "token" || name == "credentials"
}

func (s secretAnalysis) safeSecretType(typ types.Type) bool {
	if typ == nil {
		return false
	}

	if named, ok := types.Unalias(typ).(*types.Named); ok {
		return named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "github.com/justtrackio/gosoline/pkg/log" && named.Obj().Name() == "Secret"
	}

	return false
}

func (s secretAnalysis) implementsError(typ types.Type) bool {
	return typ != nil && types.Implements(typ, types.Universe.Lookup("error").Type().Underlying().(*types.Interface))
}

func (s secretAnalysis) stringLike(typ types.Type) bool {
	if typ == nil {
		return false
	}

	switch underlying := typ.Underlying().(type) {
	case *types.Basic:
		return underlying.Info()&types.IsString != 0
	case *types.Slice:
		basic, ok := underlying.Elem().Underlying().(*types.Basic)
		return ok && basic.Kind() == types.Uint8
	}

	return false
}

func (s secretAnalysis) confidentialStruct(typ types.Type, depth int) bool {
	if typ == nil || depth > 6 || s.safeSecretType(typ) {
		return false
	}

	if pointer, ok := typ.Underlying().(*types.Pointer); ok {
		return s.confidentialStruct(pointer.Elem(), depth+1)
	}

	if structure, ok := typ.Underlying().(*types.Struct); ok {
		for i := 0; i < structure.NumFields(); i++ {
			field := structure.Field(i)
			if !s.safeSecretType(field.Type()) && s.confidentialName(field.Name()) && s.stringLike(field.Type()) || s.confidentialStruct(field.Type(), depth+1) {
				return true
			}
		}
	}

	return false
}
