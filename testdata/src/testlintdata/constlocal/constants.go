package constlocal

const query = `SELECT name FROM views`                                 // want "constant query is used only by function List"
const order = " ORDER BY name"                                         // want "constant order is used only by function List"
const tasks = `SELECT * FROM task_runs WHERE ` + "`DATABASE`" + ` = ?` // want "constant tasks is used only by function Tasks"
const repeated = "repeated"                                            // want "constant repeated is used only by function Repeat"
const crossFile = "other file"                                         // want "constant crossFile is used only by function CrossFile"
const closure = "closure"                                              // want "constant closure is used only by function Closure"
const numeric = 100                                                    // want "constant numeric is used only by function Limit"

const (
	first, second = "first", "second" // want "constant first is used only by function Pair" "constant second is used only by function Pair"
)

func List() string {
	return query + order
}

type Handler struct{}

func (Handler) Tasks() string {
	return tasks
}

func Repeat() string {
	return repeated + repeated
}

func Closure() func() string {
	return func() string { return closure }
}

func Limit() int {
	return numeric
}

func Pair() string {
	return first + second
}

const shared = "shared"
const initializer = "initializer"
const dependency = "dependency"
const derived = dependency + "!" // want "constant derived is used only by function Boundaries"
const signature = 2
const Exported = "public"
const typed string = "typed"
const unused = "unused"
const fromGenerated = "generated user"

var initialized = initializer

const (
	zero = iota
	one
)

const (
	explicit = "explicit"
	implicit
)

func Boundaries(_ [signature]int) string {
	const local = "local" // want "local constant local is used once; inline its value"
	return shared + initializer + derived + Exported + typed + local + fromGenerated + explicit + implicit
}

func Shared() string {
	return shared
}

func Numbers() int {
	return zero + one
}

const shadowed = "package" // want "constant shadowed is used only by function PackageValue"

func Shadowed() string {
	const shadowed = "local" // want "local constant shadowed is used once; inline its value"
	return shadowed
}

func PackageValue() string {
	return shadowed
}

const literalInitializer = "initializer closure"

var factory = func() string { return literalInitializer }
