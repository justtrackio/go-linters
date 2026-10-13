package methodfilecheck

func (Across) First()   {} // want "method Across.First: must be declared in order.go with receiver type Across"
func helperAcross()     {}
func (*Across) Second() {}

type Interloper int

func (Interloper) First() {}
func (Across) Third()     {} // want "method Across.Third: keep Across methods contiguous" "method Across.Third: must be declared in order.go"

// The type in a generated file is not considered an owner.
func (GeneratedOwner) NormalMethod() {}

// A guarded type does not impose cross-file placement.
func (GuardedOwner) NormalMethod() {}
