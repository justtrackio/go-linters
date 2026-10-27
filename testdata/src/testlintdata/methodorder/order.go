package methodorder

func (Value) Before() {} // want "method Before must be declared after receiver type Value in the same file"

func (*Value) PointerBefore() {} // want "method PointerBefore must be declared after receiver type Value in the same file"

type Value struct{}

func (Value) After() {}

func (*Value) PointerAfter() {}

func (Single[T]) Before() {} // want "method Before must be declared after receiver type Single in the same file"

type Single[T any] struct{}

func (*Single[T]) After() {}

// PurgeJobs mirrors the generic pointer receiver in queue_service.go.
func (s *QueueService[T, F]) PurgeJobs() {} // want "method PurgeJobs must be declared after receiver type QueueService in the same file"

type QueueService[T any, F any] struct{}

func (s *QueueService[T, F]) After() {}

func (Pair[T, F]) Before() {} // want "method Before must be declared after receiver type Pair in the same file"

type Pair[T, F any] struct{}

func (Grouped) Before() {} // want "method Before must be declared after receiver type Grouped in the same file"

type (
	Unrelated int
	Grouped   struct{}
)

func (Grouped) After() {}

func (Alias) AliasBefore() {} // want "method AliasBefore must be declared after receiver type Alias in the same file"

type Alias = Value

func (Alias) AliasAfter() {}

// A receiver declared in another file has no ordering requirement here.
func (Elsewhere) OtherFile() {}

func (*Elsewhere) OtherFilePointer() {}

// Functions and local types do not participate in the rule.
func BeforeFunctionType() {}

type FunctionType struct{}

func LocalType() {
	type Elsewhere struct{}
	_ = Elsewhere{}
}
