package missingimport

import "testlintdata/iferrinline/widget"

func Capacity() (int, error) {
	time := 1
	value, err := widget.Duration() // want "if err can be inlined by hoisting value"
	if err != nil {
		return 0, err
	}
	count, err := convert(int(value)) // want "if err can be inlined by hoisting count"
	if err != nil {
		return 0, err
	}
	return count + time, nil
}

func convert(value int) (int, error) { return value, nil }

func Other() (int, error) {
	value, err := widget.Duration() // want "if err can be inlined by hoisting value"
	if err != nil {
		return 0, err
	}
	return int(value), nil
}
