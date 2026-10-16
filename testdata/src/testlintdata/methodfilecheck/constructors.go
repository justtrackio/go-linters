package methodfilecheck

type Validator interface {
	Validate()
}

type validator struct{}

// Constructors may return the receiver type or an interface it implements.
func NewValidator() (Validator, error) { return nil, nil }
func BuildValidator() *validator       { return &validator{} }
func (validator) Validate()            {}

type Foreign interface {
	Other()
}

type foreign struct{}

// The interface is not implemented, so the factory is not a constructor.
func NewForeign() (Foreign, error) { return nil, nil }
func (foreign) First()             {} // want "method foreign.First: keep the receiver type, its constructors, and its first method together"

type Shaped interface {
	Shape()
}

type shaped struct{}

// An interface may only be returned alone or alongside a final error.
func NewShaped() (Shaped, bool) { return nil, true }
func (shaped) First()           {} // want "method shaped.First: keep the receiver type, its constructors, and its first method together"
func (shaped) Shape()           {}

type Helper struct{}

// Non-constructors do not belong in the gap.
func notAConstructor() {}
func (Helper) First()  {} // want "method Helper.First: keep the receiver type, its constructors, and its first method together"

type RawShaped struct{}

// The final-error shape rule applies to the receiver type as well.
func NewRawShaped() (RawShaped, bool) { return RawShaped{}, true }
func (RawShaped) First()              {} // want "method RawShaped.First: keep the receiver type, its constructors, and its first method together"

type Failing struct{}

// A function returning error is not a constructor: every type implements it.
func failSomething() error { return nil }
func (Failing) First()     {} // want "method Failing.First: keep the receiver type, its constructors, and its first method together"

type Created interface {
	Create()
}

type created struct{}

// The Create and Provide prefixes are constructor names as well.
func CreateCreated() (Created, error) { return nil, nil }
func ProvideCreated() *created        { return &created{} }
func (created) Create()               {}

type Renamed struct{}

// A factory without a constructor name is not a constructor.
func makeRenamed() *Renamed { return &Renamed{} }
func (Renamed) First()      {} // want "method Renamed.First: keep the receiver type, its constructors, and its first method together"

type Line struct{}

// A prefix without a word boundary is not a constructor name.
func Newline() *Line { return &Line{} }
func (Line) First()  {} // want "method Line.First: keep the receiver type, its constructors, and its first method together"
