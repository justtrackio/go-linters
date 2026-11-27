package methodorder

import "context"

func withEarlyRequest(ctx context.Context, request string) context.Context { // want "keep context accessor withEarlyRequest with the other accessors directly after context key type earlyRequestKey in contextkeys_order.go"
	return context.WithValue(ctx, earlyRequestKey{}, request)
}

type earlyRequestKey struct{}

func earlyRequestFromContext(ctx context.Context) any {
	return ctx.Value(earlyRequestKey{})
}

func CaptureEarlyRequest(ctx context.Context) func(string) context.Context {
	return func(request string) context.Context { return withEarlyRequest(ctx, request) }
}

type tenantContextKey struct{}

const DefaultTenant = "default"

func withTenant(ctx context.Context, tenant string) context.Context { // want "keep context accessor withTenant with the other accessors directly after context key type tenantContextKey in contextkeys_order.go"
	return context.WithValue(ctx, tenantContextKey{}, tenant)
}

func tenantFromContext(ctx context.Context) any { // want "keep context accessor tenantFromContext with the other accessors directly after context key type tenantContextKey in contextkeys_order.go"
	return ctx.Value(tenantContextKey{})
}

type requestIDContextKey struct{}

func requestIDFromContext(ctx context.Context) any {
	return ctx.Value(requestIDContextKey{})
}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, id)
}

func (requestIDContextKey) String() string { return "request ID" }

func withEmptyRequestID(ctx context.Context) context.Context { // want "keep context accessor withEmptyRequestID with the other accessors directly after context key type requestIDContextKey in contextkeys_order.go"
	return context.WithValue(ctx, requestIDContextKey{}, "")
}
