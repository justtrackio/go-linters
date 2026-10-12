package inlineprivate

func localPrivateStruct() (struct{ privateField int }, error) {
	return struct{ privateField int }{privateField: 1}, nil
}

func use(int) {}

func SamePackagePrivateStruct() error {
	value, err := localPrivateStruct() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
	if err != nil {
		return err
	}
	use(value.privateField)
	return nil
}
