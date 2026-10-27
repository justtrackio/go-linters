//go:build !declfilecheck_disabled

package declfilecheck

type guardedDeclaration struct{}

func Guarded() { _ = guardedUser{} }
