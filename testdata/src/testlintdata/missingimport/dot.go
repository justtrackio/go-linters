package missingimport

import (
	"testlintdata/iferrinline/widget"
	. "time"
)

func Dot() (int, error) {
	value, err := widget.Duration() // want "if err can be inlined by hoisting value"
	if err != nil {
		return 0, err
	}
	return int(value), nil
}

var _ Duration
