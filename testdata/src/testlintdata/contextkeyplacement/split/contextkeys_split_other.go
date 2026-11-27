package split

import "context"

// want +1 "keep context accessor writePairedContext.*after context key type pairedContextKey in contextkeys_split.go"
func writePairedContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, pairedContextKey{}, "value")
}

// want +1 "keep context accessor readPairedContext.*after context key type pairedContextKey in contextkeys_split.go"
func readPairedContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(pairedContextKey{}).(string)

	return value, ok
}
