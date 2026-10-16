package funcmethod

import "time"

type Thing struct{}

func newThing() *Thing { return &Thing{} }

func (Thing) Use() {}

type thingOwner struct{}

func (o *thingOwner) Make() *Thing {
	return newThing()
}

type Box[T any] struct{ value T }

func createBox[T any](value T) *Box[T] { return &Box[T]{value: value} }

func (Box[T]) Use() {}

type boxOwner struct{}

func (o *boxOwner) Pack() *Box[int] {
	return createBox(1)
}

type Multi struct{}

func newMulti() (*Multi, error) { return &Multi{}, nil }

func (Multi) Use() {}

type multiOwner struct{}

func (o *multiOwner) Build() (*Multi, error) {
	return newMulti()
}

type Value struct{}

func provideValue() Value { return Value{} }

func (Value) Use() {}

type valueOwner struct{}

func (o *valueOwner) Produce() Value {
	return provideValue()
}

type predeclaredOwner struct{}

func (o *predeclaredOwner) Describe() string {
	return describeThing()
}

// Predeclared results do not construct a type, so the helper stays reportable.
func describeThing() string { return "thing" } // want "function describeThing is used only by predeclaredOwner"

type timedOwner struct{}

func (o *timedOwner) Stamp() time.Time {
	return stampTime()
}

// Foreign types are not constructed by this package, so the helper stays reportable.
func stampTime() time.Time { return time.Time{} } // want "function stampTime is used only by timedOwner"

type Flagged struct{}

func (Flagged) Use() {}

type flaggedOwner struct{}

func (o *flaggedOwner) Check() (Flagged, bool) {
	return checkFlag()
}

// Only a single result or a final error may accompany the constructed type.
func checkFlag() (Flagged, bool) { return Flagged{}, true } // want "function checkFlag is used only by flaggedOwner"

type Pair struct{ first, second int }

func (Pair) Use() {}

type pairOwner struct{}

func (o *pairOwner) Split() (Pair, Pair) {
	return splitPair()
}

func splitPair() (Pair, Pair) { return Pair{}, Pair{} } // want "function splitPair is used only by pairOwner"

type Triple struct{}

func (Triple) Use() {}

type tripleOwner struct{}

func (o *tripleOwner) Assemble() (Triple, error, int) {
	return makeTriple()
}

func makeTriple() (Triple, error, int) { return Triple{}, nil, 0 } // want "function makeTriple is used only by tripleOwner"

type Session struct{}

func makeSession() Session { return Session{} } // want "function makeSession is used only by sessionOwner; make it a method of sessionOwner"

func (Session) Use() {}

type sessionOwner struct{}

func (o *sessionOwner) Make() Session {
	return makeSession()
}

type Report struct{}

func makeReport() (*Report, error) { return &Report{}, nil } // want "function makeReport is used only by reportOwner; make it a method of reportOwner"

func (Report) Use() {}

type reportOwner struct{}

func (o *reportOwner) Make() (*Report, error) {
	return makeReport()
}

type namedOwner struct{}

func (o *namedOwner) Label() string {
	return newLabel()
}

// A constructor-like name alone does not make a constructor.
func newLabel() string { return "label" } // want "function newLabel is used only by namedOwner"

type Newline struct{}

func (Newline) Use() {}

type newlineOwner struct{}

func (o *newlineOwner) Next() Newline {
	return newline()
}

// A prefix without a word boundary is not a constructor name.
func newline() Newline { return Newline{} } // want "function newline is used only by newlineOwner"
