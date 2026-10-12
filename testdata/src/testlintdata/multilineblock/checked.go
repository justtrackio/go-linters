package multilineblock

import "errors"

type status struct{ Exists *bool }

func inspect() (status, int, int, error) { return status{}, 0, 0, nil }

func checked(flag bool) {
	cat := errors.New("access denied")
	service := cat.Error()
	result, _, _, err := inspect() // want "missing blank line before assignment checked by if"
	if err == nil || result.Exists != nil {
		_ = service
	}

	service = query()
	// Keep this comment with the assignment.
	result, _, _, err = inspect() // want "missing blank line before assignment checked by if"
	// Keep this comment with the check.
	if err == nil || result.Exists != nil {
		_ = service
	}

	service = query()

	result, _, _, err = inspect()
	if err != nil {
		_ = service
	}

	result, _, _, err = inspect()
	if err != nil {
		_ = service
	}

	service = query()
	result, _, _, err = inspect()
	if flag { // want "missing blank line before control block"
		_ = result
	}

	service = query()
	result, _, _, err = // want "missing blank line before assignment checked by if"
		inspect()
	if err == nil || result.Exists != nil {
		_ = service
	}

	service = query()
	result, _, _, err = inspect()
	if err := errors.New("other"); err != nil { // want "missing blank line before control block"
		_ = err
	}

	// A field name is not a reference to an assigned variable of that name.
	service = query()
	Exists := true
	if result.Exists != nil { // want "missing blank line before control block"
		_ = Exists
	}

	if flag {
		_ = service
	}
	result, _, _, err = inspect() // want "missing blank line after block"
	if err != nil {
		_ = service
	}
}
