package constlocal

func MultilineCall() {
	const query = `SELECT
name FROM views`
	consumeQuery(query, "argument")
}

func MultilineConcatenatedCall() {
	const query = `SELECT
name FROM views WHERE ` + "`DATABASE`" + ` = ?`
	consumeQuery((query), "argument")
}

func MultilineArgumentExpression() {
	const query = `SELECT
name FROM views`
	consumeQuery(query + " ORDER BY name")
}

func SingleLineCall() {
	const query = "SELECT name FROM views" // want "local constant query is used once; inline its value"
	consumeQuery(query)
}

func EscapedNewlineCall() {
	const query = "SELECT\nname FROM views" // want "local constant query is used once; inline its value"
	consumeQuery(query)
}

func MultilineReturnedFromClosure() {
	const query = /* want "local constant query is used once; inline its value" */ `SELECT
name FROM views`
	consumeQuery(func() string { return query }())
}

const packageQuery = /* want "constant packageQuery is used only by function PackageMultilineCall; move it into that function$" */ `SELECT
name FROM views`

func PackageMultilineCall() {
	consumeQuery(packageQuery)
}

func MixedGroupCall() {
	const (
		query, suffix = /* want "local constant suffix is used once; inline its value" */ `SELECT
name FROM views`, " ORDER BY name"
	)
	consumeQuery(query, suffix)
}

func consumeQuery(...string) {}
