package declfilecheck

import "context"

func writeUnpairedContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, unpairedContextKey{}, true)
}
