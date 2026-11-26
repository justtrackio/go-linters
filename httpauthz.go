package linters

import (
	"go/ast"
	"go/constant"
	"go/types"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

const (
	authzHTTPPackage  = "github.com/gosoline-project/authz/httpserver"
	httpServerPackage = "github.com/gosoline-project/httpserver"
	ginPackage        = "github.com/gin-gonic/gin"
)

func init() {
	register.Plugin("httpauthz", NewHTTPAuthz)
}

// HTTPAuthzPlugin enforces explicit authorization at HTTP route registrations.
type HTTPAuthzPlugin struct{}

func NewHTTPAuthz(_ any) (register.LinterPlugin, error) {
	return &HTTPAuthzPlugin{}, nil
}

func (*HTTPAuthzPlugin) GetLoadMode() string { return register.LoadModeTypesInfo }

func (*HTTPAuthzPlugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{HTTPAuthzAnalyzer()}, nil
}

// HTTPAuthzAnalyzer returns the analyzer shared by the plugin and standalone command.
func HTTPAuthzAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "httpauthz",
		Doc:  "requires explicit authorization on HTTP route registrations",
		Run:  runHTTPAuthz,
	}
}

func runHTTPAuthz(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if inTestFile(pass, file) {
			continue
		}
		ast.PreorderStack(file, nil, func(node ast.Node, stack []ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				checkHTTPAuthzCall(pass, n)
			case *ast.SelectorExpr:
				selection := pass.TypesInfo.Selections[n]
				if selection == nil || (selection.Kind() != types.MethodExpr && isDirectHTTPCall(n, stack)) {
					break
				}
				fn, _ := selection.Obj().(*types.Func)
				if isRouteMethod(fn, httpServerPackage) || isRouteMethod(fn, ginPackage) || isNetHTTPRegistration(fn) || isGinServing(fn) || isHTTPServeHTTP(fn) {
					pass.Reportf(n.Pos(), "opaque HTTP registration method values and expressions are not supported")
				}
			case *ast.Ident:
				fn, _ := pass.TypesInfo.Uses[n].(*types.Func)
				if fn != nil && fn.Signature().Recv() == nil && (isNetHTTPRegistration(fn) || opaqueHTTPFunction(fn) != "") {
					var expr ast.Expr = n
					if len(stack) > 0 {
						if sel, ok := stack[len(stack)-1].(*ast.SelectorExpr); ok && sel.Sel == n {
							expr = sel
							stack = stack[:len(stack)-1]
						}
					}
					if !isDirectHTTPCall(expr, stack) {
						pass.Reportf(n.Pos(), "opaque HTTP serving function values are unsupported")
					}
				}
			}
			return true
		})
	}
	return nil, nil
}

func isDirectHTTPCall(expr ast.Expr, stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		switch n := stack[i].(type) {
		case *ast.CallExpr:
			return uninstantiate(n.Fun) == expr
		case *ast.ParenExpr, *ast.IndexExpr, *ast.IndexListExpr:
			continue
		default:
			return false
		}
	}
	return false
}

func uninstantiate(expr ast.Expr) ast.Expr {
	for {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.IndexListExpr:
			expr = e.X
		default:
			return expr
		}
	}
}

func calledFunc(pass *analysis.Pass, expr ast.Expr) *types.Func {
	switch e := uninstantiate(expr).(type) {
	case *ast.Ident:
		fn, _ := pass.TypesInfo.Uses[e].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pass.TypesInfo.Uses[e.Sel].(*types.Func)
		return fn
	}
	return nil
}

func functionIs(fn *types.Func, pkg, name string) bool {
	return fn != nil && fn.Pkg() != nil && fn.Pkg().Path() == pkg && fn.Name() == name
}

func isRouteMethod(fn *types.Func, pkg string) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != pkg {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	if pkg == httpServerPackage && receiverName(sig.Recv().Type()) != "Router" {
		return false
	}
	if pkg == ginPackage {
		switch receiverName(sig.Recv().Type()) {
		case "Engine", "RouterGroup", "IRouter", "IRoutes":
		default:
			return false
		}
	}
	switch fn.Name() {
	case "Handle", "HandleWith", "GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD", "Any", "Match", "Static", "StaticFS", "StaticFile", "StaticFileFS", "NoRoute", "NoMethod":
		return true
	}
	return false
}

func receiverName(t types.Type) string {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if named, ok := types.Unalias(t).(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

func isNetHTTPRegistration(fn *types.Func) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != "net/http" {
		return false
	}
	switch fn.Name() {
	case "Handle", "HandleFunc", "Serve", "ServeTLS", "ListenAndServe", "ListenAndServeTLS":
		return true
	}
	return false
}

func isGinServing(fn *types.Func) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != ginPackage {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil || receiverName(sig.Recv().Type()) != "Engine" {
		return false
	}
	switch fn.Name() {
	case "Run", "RunTLS", "RunUnix", "RunFd", "RunListener", "RunQUIC", "ServeHTTP", "HandleContext":
		return true
	}
	return false
}

func isHTTPServeHTTP(fn *types.Func) bool {
	if fn == nil || fn.Name() != "ServeHTTP" {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil || sig.Params().Len() != 2 || sig.Results().Len() != 0 {
		return false
	}
	first, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Named)
	if !ok || first.Obj().Pkg() == nil || first.Obj().Pkg().Path() != "net/http" || first.Obj().Name() != "ResponseWriter" {
		return false
	}
	ptr, ok := types.Unalias(sig.Params().At(1).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	second, ok := types.Unalias(ptr.Elem()).(*types.Named)
	return ok && second.Obj().Pkg() != nil && second.Obj().Pkg().Path() == "net/http" && second.Obj().Name() == "Request"
}

func opaqueHTTPFunction(fn *types.Func) string {
	if fn == nil || fn.Pkg() == nil {
		return ""
	}
	switch fn.Pkg().Path() {
	case ginPackage:
		if fn.Name() == "WrapH" || fn.Name() == "WrapF" {
			return "external HTTP handlers cannot be verified"
		}
	case httpServerPackage:
		switch fn.Name() {
		case "NewWithInterfaces", "NewHealthCheckWithInterfaces":
			return "serving an externally configured router is unsupported"
		case "CreateEmbeddedStaticServe":
			return "middleware-based static serving is unsupported; mount an explicit exempt route"
		}
	case "github.com/gosoline-project/sqlh":
		if fn.Name() == "WithCrudHandlers" {
			return "SQLH automatic route mounting is unsupported"
		}
	}
	return ""
}

func passesRouterToExternalFunction(pass *analysis.Pass, call *ast.CallExpr, fn *types.Func) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg() == pass.Pkg || fn.Pkg().Path() == httpServerPackage {
		return false
	}
	for _, arg := range call.Args {
		t := types.Unalias(pass.TypesInfo.TypeOf(arg))
		if ptr, ok := t.(*types.Pointer); ok {
			t = types.Unalias(ptr.Elem())
		}
		if named, ok := t.(*types.Named); ok {
			obj := named.Obj()
			if obj.Pkg() != nil && obj.Pkg().Path() == httpServerPackage && obj.Name() == "Router" {
				return true
			}
		}
	}
	return false
}

func checkHTTPAuthzMiddleware(pass *analysis.Pass, call *ast.CallExpr) {
	for _, arg := range call.Args {
		binder, ok := unparen(arg).(*ast.CallExpr)
		if !ok {
			continue
		}
		fn := calledFunc(pass, binder.Fun)
		if fn == nil || fn.Pkg() == nil {
			continue
		}
		path := fn.Pkg().Path()
		if ((path == httpServerPackage || path == authzHTTPPackage) && strings.HasPrefix(fn.Name(), "Bind")) ||
			functionIs(fn, authzHTTPPackage, "Exempt") {
			pass.Reportf(arg.Pos(), "route handlers must be mounted as routes, not router middleware")
		}
	}
}

func checkHTTPAuthzCall(pass *analysis.Pass, call *ast.CallExpr) {
	fn := calledFunc(pass, call.Fun)
	if functionIs(fn, httpServerPackage, "Use") {
		checkHTTPAuthzMiddleware(pass, call)
	}
	if isGinServing(fn) {
		pass.Reportf(call.Pos(), "independent Gin serving is unsupported")
		return
	}
	if isHTTPServeHTTP(fn) {
		pass.Reportf(call.Pos(), "external HTTP handler serving cannot be verified")
		return
	}
	if isRouteMethod(fn, ginPackage) {
		pass.Reportf(call.Pos(), "raw Gin route registration is unsupported; use httpserver.Router")
		return
	}
	if isNetHTTPRegistration(fn) {
		pass.Reportf(call.Pos(), "independent net/http registration or serving is unsupported")
		return
	}
	if diagnostic := opaqueHTTPFunction(fn); diagnostic != "" {
		pass.Reportf(call.Pos(), "%s", diagnostic)
		return
	}
	if passesRouterToExternalFunction(pass, call, fn) {
		pass.Reportf(call.Pos(), "passing httpserver.Router to an external registration helper is unsupported")
		return
	}
	if !isRouteMethod(fn, httpServerPackage) {
		return
	}
	if sel, ok := uninstantiate(call.Fun).(*ast.SelectorExpr); ok {
		if selection := pass.TypesInfo.Selections[sel]; selection != nil && selection.Kind() == types.MethodExpr {
			return
		}
	}
	if fn.Name() == "HandleWith" {
		checkHTTPAuthzWith(pass, call)
		return
	}
	start := 1
	if fn.Name() == "Handle" {
		start = 2
	}
	if call.Ellipsis.IsValid() || len(call.Args) != start+1 || !approvedHTTPHandler(pass, call.Args[start]) {
		pass.Reportf(call.Pos(), "route must have exactly one direct authzhttp Bind, BindWithOptions, BindN, BindPrepared, or Exempt handler")
	}
}

func checkHTTPAuthzWith(pass *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) == 1 {
		if with, ok := unparen(call.Args[0]).(*ast.CallExpr); ok && functionIs(calledFunc(pass, with.Fun), httpServerPackage, "With") && len(with.Args) == 2 {
			if _, ok := unparen(with.Args[1]).(*ast.FuncLit); ok {
				return
			}
		}
	}
	pass.Reportf(call.Pos(), "HandleWith requires direct httpserver.With with an inline registration callback")
}

func approvedHTTPHandler(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := unparen(expr).(*ast.CallExpr)
	if !ok {
		return false
	}
	fn := calledFunc(pass, call.Fun)
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != authzHTTPPackage {
		return false
	}
	switch fn.Name() {
	case "Bind", "BindWithOptions", "BindN", "BindPrepared":
		return true
	case "Exempt":
		if len(call.Args) != 2 || pass.TypesInfo.Types[call.Args[1]].IsNil() {
			return false
		}
		value := pass.TypesInfo.Types[call.Args[0]].Value
		return value != nil && value.Kind() == constant.String && strings.TrimSpace(constant.StringVal(value)) != ""
	}
	return false
}

func unparen(expr ast.Expr) ast.Expr {
	for {
		p, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = p.X
	}
}
