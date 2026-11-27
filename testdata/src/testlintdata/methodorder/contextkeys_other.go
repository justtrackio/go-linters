package methodorder

import "context"

// want +1 "keep context accessor writeCrossFileContext.*after context key type crossFileContextKey in contextkeys.go"
func writeCrossFileContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, crossFileContextKey{}, true)
}
