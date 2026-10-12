package missingimport

import (
	"fmt" // Keep this comment.

	"testlintdata/iferrinline/widget"
)

func Grouped() (string, error) {
	value, err := widget.Duration() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
	if err != nil {
		return "", err
	}
	return fmt.Sprint(value), nil
}
