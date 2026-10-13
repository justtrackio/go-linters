package iferrinline

func NamedErrorUsedAfter() error {
	convErr := boom()
	if convErr != nil {
		return convErr
	}
	return convErr
}

func CheckDifferentError(otherErr error) error {
	convErr := boom()
	if otherErr != nil {
		return otherErr
	}
	return convErr
}

func NonErrorNamedErr() {
	err := new(int)
	if err != nil {
		use(*err)
	}
}

func NonErrorNamedConvErr() {
	convErr := new(int)
	if convErr != nil {
		use(*convErr)
	}
}

func ShadowedNil(nil error) error {
	convErr := boom()
	if convErr != nil {
		return convErr
	}
	return nil
}

func NamedErrorAlreadyInline() error {
	if convErr := boom(); convErr != nil {
		return convErr
	}
	return nil
}

func NamedErrorAssignment() error {
	var convErr error
	convErr = boom() // want "if convErr can be inlined into the assignment"
	if convErr != nil {
		return convErr
	}
	return convErr
}

func NamedErrorWithCompanionInLoop(items []int) ([]int, error) {
	var err error
	applications := make([]int, 0, len(items))
	for _, item := range items {
		view, convErr := bar() // want "if convErr can be inlined by hoisting view to var declarations at the top of the function and changing := to ="
		if convErr != nil {
			return nil, wrap(convErr)
		}
		applications = append(applications, view+item)
	}
	return applications, err
}

type conversionError struct{}

func concreteError() *conversionError { return nil }

func (*conversionError) Error() string { return "conversion failed" }

func ConcreteErrorVariable() error {
	convErr := concreteError() // want "if convErr can be inlined into the assignment"
	if convErr != nil {
		return convErr
	}
	return nil
}
