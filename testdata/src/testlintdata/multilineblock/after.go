package multilineblock

import "os"

func after(flag bool, values []int, value any) {
	if flag {
		query()
	}
	query() // want "missing blank line after block"

	for flag {
		break
	}
	query() // want "missing blank line after block"

	for range values {
		query()
	}
	query() // want "missing blank line after block"

	switch value {
	default:
	}
	query() // want "missing blank line after block"

	switch value.(type) {
	default:
	}
	query() // want "missing blank line after block"

	select {
	default:
	}
	query() // want "missing blank line after block"

	{
		query()
	}
	// This comment belongs to the next statement.
	query() // want "missing blank line after block"

	if flag {
		query()
	} else {
		query()
	}
	if !flag { // want "missing blank line after block"
		query()
	}

	if flag {
		query()
	}

	query()
}

func cleanup() error {
	f, err := os.Open("file")
	if err != nil {
		return err
	}
	defer f.Close()

	if f, err := os.Open("file"); nil != err {
		return err
	} else {
		defer f.Close()
	}
	query() // want "missing blank line after block"

	if err != nil {
		return err
	}
	defer query() // want "missing blank line after block"

	other, err := os.Open("other")
	if err != nil {
		return err
	}
	defer f.Close() // want "missing blank line after block"

	defer other.Close()

	return nil
}

func cleanupWithInit() {
	if f, err := os.Open("file"); err != nil {
		defer f.Close()
	}
	// f is out of scope, so this cannot be resource cleanup.
	defer query() // want "missing blank line after block"
}

func cleanupWithAssignmentInit() error {
	var f *os.File
	var err error
	if f, err = os.Open("file"); err != nil { // want "missing blank line before control block"
		return err
	}
	defer f.Close()

	return nil
}

func labeled(flag bool) {
loop:
	for flag {
		break loop
	}
	query() // want "missing blank line after block"
}
