package iferrinline

import (
	"errors"

	"testlintdata/iferrinline/widget"
)

func boom() error { return errors.New("x") }

func AlreadyInline() error {
	if err := boom(); err != nil {
		return err
	}
	return nil
}

func ShouldInline() error {
	err := boom() // want "if err can be inlined into the assignment"
	if err != nil {
		return err
	}
	return nil
}

func ErrUsedAfter() error {
	err := boom()
	if err != nil {
		return err
	}
	_ = err
	return nil
}

func NotErrName() error {
	e := boom()
	if e != nil {
		return e
	}
	return nil
}

func IfHasInitAlready() error {
	if err := boom(); err != nil {
		return err
	}
	return nil
}

func bar() (int, error) { return 0, nil }

func use(int) {}

func wrap(e error) error { return e }

func MultiBlank() error {
	_, err := bar() // want "if err can be inlined into the assignment"
	if err != nil {
		return err
	}
	return nil
}

func MultiCompanionUnusedAfter() error {
	x, err := bar() // want "if err can be inlined into the assignment"
	if err != nil {
		_ = x
		return err
	}
	return nil
}

func MultiCompanionUsedAfter() error {
	x, err := bar() // want "if err can be inlined by hoisting x to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	use(x)
	return nil
}

func withCallback(fn func() int) (int, error) { return fn(), nil }

func MultiLineCallNotFlagged() error {
	x, err := withCallback(func() int {
		return 42
	})
	if err != nil {
		return err
	}
	use(x)
	return nil
}

func three() (string, bool, error) { return "", false, nil }

func use2(string, bool) {}

func ThreeReturns() error {
	s, ok, err := three() // want "if err can be inlined by hoisting s, ok to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	use2(s, ok)
	return nil
}

func MultiErrReused() error {
	x, err := bar() // want "if err can be inlined by hoisting x to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	err = wrap(err)
	_ = err
	use(x)
	return nil
}

func ConsecutiveErrSites() error {
	a, err := bar() // want "if err can be inlined by hoisting a to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	b, err := bar() // want "if err can be inlined by hoisting b to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	use(a)
	use(b)
	return nil
}

func ExistingVarBlock() error {
	var err error
	var existing int

	a, err := bar() // want "if err can be inlined by hoisting a to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	use(a)
	_ = existing
	return nil
}

type holder struct {
	widget *widget.Service
}

func PackageShadow() (*holder, error) {
	widget, err := widget.New() // want "if err can be inlined by hoisting widget \\(renamed to service to avoid shadowing\\) to var declarations at the top of the function and changing := to ="
	if err != nil {
		return nil, err
	}
	return &holder{widget: widget}, nil
}

func useService(*widget.Service) {}

func ParamShadow(svc *widget.Service) error {
	useService(svc)
	svc, err := widget.With(svc) // want "if err can be inlined by hoisting svc to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	useService(svc)
	return nil
}

func ExistingVarBlockReorder() error {
	var x int
	var err error

	a, err := bar() // want "if err can be inlined by hoisting a to var declarations at the top of the function and changing := to ="
	if err != nil {
		return err
	}
	use(a)
	use(x)
	_ = err
	return nil
}

func NamedReturnErrNestedBlock(items []int) (err error) {
	for _, item := range items {
		a, err := bar() // want "if err can be inlined by hoisting a to var declarations at the top of the function and changing := to ="
		if err != nil {
			return err
		}
		use(a)
		use(item)
	}
	return
}

func ParamErrNestedBlock(err error) error {
	for i := 0; i < 3; i++ {
		a, err := bar() // want "if err can be inlined by hoisting a to var declarations at the top of the function and changing := to ="
		if err != nil {
			return err
		}
		use(a)
		use(i)
	}
	return err
}
