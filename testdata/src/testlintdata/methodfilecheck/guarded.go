//go:build !methodfilecheck_disabled

package methodfilecheck

type GuardedOwner struct{}

func (GuardedOwner) First()   {}
func (Across) GuardedMethod() {}

type GuardedLocal struct{}

var guardedSeparator = 1

func (GuardedLocal) First()          {}                        // want "method GuardedLocal.First: keep the receiver type"
func MakeGuardedLocal() GuardedLocal { return GuardedLocal{} } // want "constructor MakeGuardedLocal for GuardedLocal"
