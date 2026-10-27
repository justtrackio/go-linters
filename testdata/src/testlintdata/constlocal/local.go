package constlocal

func LocalQuery() string {
	const query = /* want "local constant query is used once; inline its value" */ `SELECT
    name FROM views
`
	const order = " ORDER BY name" // want "local constant order is used once; inline its value"
	result := query
	result += order
	return result
}

func LocalRepeated() string {
	const repeated = "repeated"
	return repeated + repeated
}

func LocalClosure() func() string {
	const captured = "captured" // want "local constant captured is used once; inline its value"
	return func() string {
		const nested = "nested" // want "local constant nested is used once; inline its value"
		return captured + nested
	}
}

func LocalBoundaries() string {
	const unused = "unused"
	const typed string = "typed"
	const (
		zero = iota
		one
	)
	const (
		explicit = "explicit"
		implicit
	)
	const (
		first, second = "first", "second" // want "local constant first is used once; inline its value"
	)
	const Upper = "upper" // want "local constant Upper is used once; inline its value"
	const numeric = 1     // want "local constant numeric is used once; inline its value"
	_ = [zero + one + numeric]int{}
	return typed + explicit + implicit + first + second + second + Upper
}

var localFactory = func() string {
	const value = "value" // want "local constant value is used once; inline its value"
	return value
}
