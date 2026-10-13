package inlineguard

func pair() (int, error) { return 1, nil }
func lookup() (int, bool) { return 1, true }

func unsafeErrorHoist() {
	{
		value, err := pair()
		if err != nil {
			return
		}
		_ = value
	}
	err := error(nil)
	_ = err
}

func unsafeBooleanHoist() {
	{
		value, ok := lookup()
		if !ok {
			return
		}
		_ = value
	}
	ok := true
	_ = ok
}

func competingVarOwners() {
	value, err := pair()
	if err != nil {
		return
	}
	_ = value
	other, ok := lookup()
	if !ok {
		return
	}
	_ = other
}

func safeDisjoint() {
	err := error(nil)
	if err != nil {
		return
	}
}
