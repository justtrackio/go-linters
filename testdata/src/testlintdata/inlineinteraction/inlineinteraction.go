package inlineinteraction

func read() (int, error) { return 1, nil }
func lookup(values map[string]int) (int, bool) { return values["key"], true }
func use(int) {}

func process(values map[string]int, flag bool) error {
	{
		value, err := read() // want "if err can be inlined by declaring value, err before the assignment and changing := to ="
		if err != nil {
			return err
		}
		use(value)
	}
	{
		value, ok := lookup(values) // want "if ok can be inlined by declaring value, ok before the assignment and changing := to ="
		if !ok {
			return nil
		}
		sum := value +
			values["suffix"]
		if flag { // want "missing blank line before control block"
			use(sum)
		}
	}
	return nil
}

func nestedCrossRuleCandidate(flag bool) error {
	var err error
	value, err := func() (int, error) { ok := flag; if !ok { return 0, nil }; return 1, nil }() // want "if err can be inlined by declaring value, err before the assignment and changing := to =" "if ok can be inlined into the assignment"
	if err != nil {
		_ = value
		return err
	}
	_ = err
	return nil
}
