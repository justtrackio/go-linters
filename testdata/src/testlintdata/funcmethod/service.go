package funcmethod

type SetupSchemaService struct{}
type OtherService struct{}

func (s *SetupSchemaService) Apply() {
	buildSchemaUpdate()
	chainStart()
	shared()
	throughShared()
	viaPackage()
	escaped()
	_ = escaped
	packageValue()
	Exported()
	generic(1)
	generic[int](1)
	multiGeneric[int, string](1, "")
	main()
	cycleA()
	_ = func() { inClosure() }
	go asynchronous()
	defer deferred()
	(recursive)(2)
	shadowed := func() {}
	shadowed()
	collision()
	testCompanion()
	orphanShared()
}

func (s SetupSchemaService) Inspect() { buildSchemaUpdate() }
func (s *OtherService) Apply()        { shared(); throughShared() }

func buildSchemaUpdate() {} // want "function buildSchemaUpdate is used only by SetupSchemaService; make it a method of SetupSchemaService"

// Mirrors the example: a package helper used only by tests also calls buildSchemaUpdate.
func planSchemaUpdate() { buildSchemaUpdate(); testCompanion() }

func chainStart() { chainEnd() } // want "function chainStart is used only by SetupSchemaService"
func chainEnd()   {}             // want "function chainEnd is used only by SetupSchemaService"

func shared()        {}
func throughShared() { sharedLeaf() }
func sharedLeaf()    {}

func viaPackage()   { packageLeaf() }
func packageLeaf()  {}
func packageEntry() { viaPackage() }

func PackageEntry() { packageEntry() }

func escaped()      { escapedLeaf() }
func escapedLeaf()  {}
func packageValue() {}

var callback = packageValue

func Exported()     { exportedLeaf() }
func exportedLeaf() {}

// Go methods cannot declare their own type parameters.
func generic[T any](value T) { genericLeaf() }
func genericLeaf()           {} // want "function genericLeaf is used only by SetupSchemaService"

func multiGeneric[T, U any](first T, second U) { multiGenericLeaf() }
func multiGenericLeaf()                        {} // want "function multiGenericLeaf is used only by SetupSchemaService"

// A function named main is an entry point only in package main.
func main() {} // want "function main is used only by SetupSchemaService"

func cycleA() { cycleB() } // want "function cycleA is used only by SetupSchemaService"
func cycleB() { cycleA() } // want "function cycleB is used only by SetupSchemaService"
func recursive(n int) { // want "function recursive is used only by SetupSchemaService"
	if n > 0 {
		recursive(n - 1)
	}
}
func inClosure()    {} // want "function inClosure is used only by SetupSchemaService"
func asynchronous() {} // want "function asynchronous is used only by SetupSchemaService"
func deferred()     {} // want "function deferred is used only by SetupSchemaService"
func shadowed()     {}
func unused()       {}
func testsOnly()    {}

func (s *SetupSchemaService) collision() {}
func collision()                         {}

func testCompanion() {} // want "function testCompanion is used only by SetupSchemaService"

// An unowned recursive group must not be attributed to SetupSchemaService.
func orphanA()      { orphanB(); orphanShared() }
func orphanB()      { orphanA() }
func orphanShared() {}

func OrphanEntry() { orphanA() }

type GenericService[T any] struct{}

func (s *GenericService[T]) Apply()  { genericOwner() }
func (s GenericService[T]) Inspect() { genericOwner() }
func genericOwner()                  {} // want "function genericOwner is used only by GenericService"

type Alias = SetupSchemaService

func (s *Alias) AliasMethod() { aliasHelper() }
func aliasHelper()            {} // want "function aliasHelper is used only by SetupSchemaService"

type FieldService struct{ fieldCollision int }

func (s FieldService) Apply() { fieldCollision() }
func fieldCollision()         {}

func init()       { initHelper() }
func initHelper() {}

func initialized() int { return 1 }

var value = initialized()
