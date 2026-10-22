package methodfilecheck

type Good struct{}

func BuildGood() (*Good, error) { return nil, nil }
func (Good) First()             {}
func helper()                   {}

const between = 1

func (*Good) Second() {}

type Other struct{}

func (Other) First() {}
func (Good) Third()  {} // want "method Good.Third: keep Good methods contiguous"
func (Good) Fourth() {}

func (Before) First() {} // want "method Before.First: keep the receiver type"
type Before int

type Separated struct{}

var separator = 1

func (Separated) First() {} // want "method Separated.First: keep the receiver type"

type Late struct{}

func (Late) First()                  {}
func BuildLate() Late                { return Late{} }   // want "constructor BuildLate for Late: must be declared before Late methods"
func BuildLateAgain() (*Late, error) { return nil, nil } // want "constructor BuildLateAgain for Late"

// Generics and grouped declarations are supported.
type (
	Single[T any]  struct{}
	Pair[T, U any] struct{}
)

func NewSingle[T any]() *Single[T] { return nil }
func (*Single[T]) First()          {}
func (Single[T]) Second()          {}
func (*Pair[T, U]) First()         {} // want "method Pair.First: keep the receiver type"

type Alias = Good

func (Alias) Fifth() {}

type Across struct{}

type GeneratedType struct{}
