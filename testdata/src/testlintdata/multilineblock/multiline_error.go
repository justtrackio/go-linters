package multilineblock

func updateMetadata(updates []string, location string) (string, error) { return location, nil }

func multilineErrorCheck() {
	meta, err := updateMetadata([]string{
		"snapshot",
		"reference",
	}, "")
	if err != nil {
		panic(err)
	}

	_ = meta

	meta, err = updateMetadata([]string{
		"snapshot",
	}, "")
	// Keep the error check with the multiline call.
	if err != nil {
		panic(err)
	}

	_ = meta
	// Keep this comment with the assignment.
	meta, err = updateMetadata([]string{ // want "missing blank line before assignment checked by if"
		"reference",
	}, "")
	// Keep this comment with the check.
	if err != nil {
		panic(err)
	}

	_ = meta
}
