package contextkeyplacement

import "context"

type ContextAdapter struct{ context.Context }

func (ContextAdapter) Apply() {}

type requestKey struct{}

func readRequest(ctx context.Context) any { return ctx.Value(requestKey{}) }

func writeRequest(ctx context.Context, value any) context.Context {
	return context.WithValue(ctx, requestKey{}, requestValue(value))
}

func BuildRequestContext(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, requestKey{}, value)
}

func (requestKey) String() string { return "request" }

func requestValue(value any) any { return value }

type Middleware struct{}

func (Middleware) Capture(ctx context.Context, value any) context.Context {
	return writeRequest(ctx, value)
}

func RequestValue(ctx context.Context) any { return readRequest(ctx) }
