package declfilecheck

type diagnostic struct { // want "type diagnostic is used only by client.go"
	value detail
}
type detail struct { // want "type detail is used only by client.go"
	parent *diagnostic
}
type alias = diagnostic               // want "type alias is used only by client.go"
type generic[T any] struct{ value T } // want "type generic is used only by client.go"

const (
	first  = iota // want "const first is used only by client.go"
	second        // want "const second is used only by client.go"
)

var pattern = "pattern" // want "var pattern is used only by client.go"
var derived = pattern   // want "var derived is used only by client.go"

type unused struct{}
type local struct{}

func Local() local { return local{} }

type Exported struct{ value privateDependency }
type privateDependency struct{}

const (
	PublicConstant = iota
	privateConstant
)

type generatedUser struct{}
type guardedUser struct{}
type platformUser struct{}
type testOnly struct{}

type oneUser struct{ value sharedDependency }   // want "type oneUser is used only by client.go"
type otherUser struct{ value sharedDependency } // want "type otherUser is used only by other.go"
type unusedCycleA struct{ value *unusedCycleB }
type unusedCycleB struct{ value *unusedCycleA }

var pairA, pairB = 1, 2

const (
	sharedFirst = iota
	sharedSecond
)

const (
	taskPrefix      = "image-prefetch:" // want "const taskPrefix is used only by client.go"
	pullConcurrency = 2                 // want "const pullConcurrency is used only by other.go"
)

const (
	PublicIndependent  = 1
	privateIndependent = 2 // want "const privateIndependent is used only by client.go"
)

const (
	explicitIotaFirst  = iota
	explicitIotaSecond = iota + 1
)

const (
	implicitFirst = 3
	implicitSecond
)

const constPairA, constPairB = 1, 2

const (
	dependency = 1              // want "const dependency is used only by client.go"
	dependent  = dependency + 1 // want "const dependent is used only by client.go"
)

type methodOwner struct{}

func (methodOwner) Value() int { return 1 }
