package methodorder

import ctxpkg "context"

type clientIPKey struct{}

func withClientIP(ctx ctxpkg.Context, ip string) ctxpkg.Context {
	return ctxpkg.WithValue(ctx, clientIPKey{}, ip)
}

func clientIPFromContext(ctx ctxpkg.Context) (ip string, ok bool) {
	ip, ok = ctx.Value(clientIPKey{}).(string)

	return ip, ok
}

func NewMiddleware(ctx ctxpkg.Context) func(string) ctxpkg.Context {
	return func(ip string) ctxpkg.Context { return withClientIP(ctx, ip) }
}

func ClientIP(ctx ctxpkg.Context) (string, bool) { return clientIPFromContext(ctx) }

type splitContextKey struct{}

func readSplitContext(ctx ctxpkg.Context) (value any) {
	value = ctx.Value(splitContextKey{})
	return
}

func SplitContext(ctx ctxpkg.Context) ctxpkg.Context { return writeSplitContext(ctx) }

func writeSplitContext(ctx ctxpkg.Context) ctxpkg.Context { // want "keep context accessor writeSplitContext with the other accessors directly after context key type splitContextKey in contextkeys.go"
	return ctxpkg.WithValue(ctx, splitContextKey{}, true)
}

type crossFileContextKey struct{}

func readCrossFileContext(ctx ctxpkg.Context) any {
	return ctxpkg.Context.Value(ctx, crossFileContextKey{})
}

type firstContextKey struct{}

type secondContextKey struct{}

// want +1 "keep context accessor firstContextValue.*after context key type firstContextKey"
func firstContextValue(ctx ctxpkg.Context) any  { return ctx.Value(firstContextKey{}) }
func secondContextValue(ctx ctxpkg.Context) any { return ctx.Value(secondContextKey{}) }

// want +1 "keep context accessor writeFirstContext.*after context key type firstContextKey"
func writeFirstContext(ctx ctxpkg.Context) ctxpkg.Context {
	return ctxpkg.WithValue(ctx, firstContextKey{}, true)
}

func writeSecondContext(ctx ctxpkg.Context) ctxpkg.Context {
	return ctxpkg.WithValue(ctx, secondContextKey{}, true)
}

// Both pairs can follow the same type block, but each pair has to stay together.
type (
	groupedFirstKey  struct{}
	groupedSecondKey struct{}
)

func groupedFirstValue(ctx ctxpkg.Context) any  { return ctx.Value(groupedFirstKey{}) }
func groupedSecondValue(ctx ctxpkg.Context) any { return ctx.Value(groupedSecondKey{}) }

// want +1 "keep context accessor writeGroupedFirst.*after context key type groupedFirstKey"
func writeGroupedFirst(ctx ctxpkg.Context) ctxpkg.Context {
	return ctxpkg.WithValue(ctx, groupedFirstKey{}, true)
}

func writeGroupedSecond(ctx ctxpkg.Context) ctxpkg.Context {
	return ctxpkg.WithValue(ctx, groupedSecondKey{}, true)
}

type unrelatedContextKey struct{}

func writeUnpairedContext(ctx ctxpkg.Context) ctxpkg.Context { // want "function writeUnpairedContext is used only by UnpairedContext; declare it after UnpairedContext in the same file"
	return ctxpkg.WithValue(ctx, unrelatedContextKey{}, true)
}

type readOnlyContextKey struct{}

func readOtherValue(ctx ctxpkg.Context) any { // want "function readOtherValue is used only by UnpairedContext; declare it after UnpairedContext in the same file"
	return ctx.Value(readOnlyContextKey{})
}

type nonContext struct{}

func (nonContext) Value(any) any { return nil }

func readUnrelatedValue(ctx nonContext) any { // want "function readUnrelatedValue is used only by UnpairedContext; declare it after UnpairedContext in the same file"
	return ctx.Value(unrelatedContextKey{})
}

func UnpairedContext(ctx ctxpkg.Context) (ctxpkg.Context, any, any) {
	return writeUnpairedContext(ctx), readOtherValue(ctx), readUnrelatedValue(nonContext{})
}
