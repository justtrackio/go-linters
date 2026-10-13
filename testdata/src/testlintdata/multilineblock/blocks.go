package multilineblock

func query() string { return "query" }

func blocks(flag bool, values []int, value any) {
	// A multiline assignment is still one statement.
	q := query() +
		" suffix"
	if flag { // want "missing blank line before control block"
		_ = q
	}

	// A single line remains allowed, including its leading comment.
	q = query()
	if flag {
		_ = q
	}

	q = query() +
		" suffix"

	if flag {
		_ = q
	}

	q = query()
	q += " suffix"
	for flag { // want "missing blank line before control block"
		break
	}

	q = query() +
		" suffix" // trailing comment
	// Keep this comment attached to the loop.
	for range values { // want "missing blank line before control block"
		_ = q
	}

	q = query() +
		" suffix"
	switch q { // want "missing blank line before control block"
	case "query":
		q = query() +
			" suffix"
		if flag { // want "missing blank line before control block"
			_ = q
		}
	}

	q = query() +
		" suffix"
	switch value.(type) { // want "missing blank line before control block"
	case int:
	}

	q = query() +
		" suffix"
	select { // want "missing blank line before control block"
	default:
		q = query() +
			" suffix"
		if flag { // want "missing blank line before control block"
			_ = q
		}
	}

	if flag {
		q = query() +
			" suffix"
		if flag { // want "missing blank line before control block"
			_ = q
		}
	}

	q = query()

	q += " suffix"
	if flag {
		_ = q
	}
}
