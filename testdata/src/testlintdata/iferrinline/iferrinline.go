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
	e := boom() // want "if e can be inlined into the assignment"
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
	x, err := bar() // want "if err can be inlined by declaring x, err before the assignment and changing := to ="
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
	s, ok, err := three() // want "if err can be inlined by declaring s, ok, err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	use2(s, ok)
	return nil
}

func MultiErrReused() error {
	x, err := bar() // want "if err can be inlined by declaring x, err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	err = wrap(err)
	_ = err
	use(x)
	return nil
}

func ConsecutiveErrSites() error {
	a, err := bar() // want "if err can be inlined by declaring a, err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	b, err := bar() // want "if err can be inlined by declaring b before the assignment and changing := to ="
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

	a, err := bar() // want "if err can be inlined by declaring a before the assignment and changing := to ="
	if err != nil {
		return err
	}
	use(a)
	_ = existing
	return nil
}

func useService(*widget.Service) {}

func ParamShadow(svc *widget.Service) error {
	useService(svc)
	svc, err := widget.With(svc) // want "if err can be inlined by declaring err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	useService(svc)
	return nil
}

func ExistingVarBlockReorder() error {
	var x int
	var err error

	a, err := bar() // want "if err can be inlined by declaring a before the assignment and changing := to ="
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
		a, err := bar() // want "if err can be inlined by declaring a, err before the assignment and changing := to ="
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
		a, err := bar() // want "if err can be inlined by declaring a, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(a)
		use(i)
	}
	return err
}

func AssignErrUsedAfter() error {
	var err error
	err = boom() // want "if err can be inlined into the assignment"
	if err != nil {
		return err
	}
	return err
}

func four() (int, bool, string, error) { return 0, false, "", nil }

func AssignMultiBlankUsedAfter() error {
	var status int
	var err error
	status, _, _, err = four() // want "if err can be inlined into the assignment"
	if err != nil {
		return wrap(err)
	}
	use(status)
	return err
}

func AssignNamedReturn() (status int, err error) {
	status, err = bar() // want "if err can be inlined into the assignment"
	if err != nil {
		return
	}
	return
}

type clientHolder struct{ client widget.Querier }

func PrivateClient() (*clientHolder, error) {
	client, err := widget.ProvideClient() // want "if err can be inlined by declaring client, err before the assignment and changing := to ="
	if err != nil {
		return nil, err
	}
	defer client.Close()
	if err := client.Exec(); err != nil {
		return nil, err
	}
	return &clientHolder{client: client}, nil
}

func PreserveInitializedBinding() error {
	var value int = 42
	value, err := bar() // want "if err can be inlined by declaring err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	use(value)
	return nil
}

func PreserveCapturedErrorBinding() error {
	var err error
	observe := func() error { return err }
	value, err := bar() // want "if err can be inlined by declaring value before the assignment and changing := to ="
	if err != nil {
		return err
	}
	use(value)
	return observe()
}

func NestedErrorBeforeOuterDeclaration() error {
	{
		value, err := bar() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(value)
	}

	var err error
	err = boom()
	return err
}

func IfInitBindingShadow() error {
	if _, err := bar(); err != nil {
		return err
	} else {
		value, err := bar() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(value)
	}
	return nil
}

func SiblingBlocks() error {
	{
		value, err := bar() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(value)
	}
	{
		value, err := bar() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(value)
	}
	return nil
}

func LoopShadowingOuterValue(items []int) error {
	value := 42
	for _, item := range items {
		value, err := bar() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(value)
		use(item)
	}
	use(value)
	return nil
}

func LoopClosures(items []int) []func() int {
	callbacks := make([]func() int, 0, len(items))
	for _, item := range items {
		value, err := bar() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			continue
		}
		callbacks = append(callbacks, func() int { return value + item })
	}
	return callbacks
}

func RhsLocalShadow() error {
	value, err := func(value int) (int, error) { return value, nil }(42) // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	use(value)
	return nil
}

