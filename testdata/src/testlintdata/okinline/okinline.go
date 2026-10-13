package okinline

import "testlintdata/iferrinline/widget"

func normalize() (string, bool) { return "event", true }
func use(string)                {}
func check() bool               { return true }

func EventUsedAfter() string {
	event, ok := normalize() // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
	if !ok {
		return "not configured"
	}
	return event
}

func EventOnlyInIf() {
	event, ok := normalize() // want "if ok can be inlined into the assignment"
	if !ok {
		use(event)
	}
}

func PositiveCheck() {
	event, ok := normalize() // want "if ok can be inlined into the assignment"
	if ok {
		use(event)
	} else {
		use(event)
	}
}

func BlankCompanion() {
	_, ok := normalize() // want "if ok can be inlined into the assignment"
	if !ok {
		return
	}
}

func BooleanOnly() {
	ok := check() // want "if ok can be inlined into the assignment"
	if !ok {
		return
	}
}

func AssignmentUsedAfter() bool {
	var event string
	var ok bool
	event, ok = normalize() // want "if ok can be inlined into the assignment"
	if !ok {
		return ok
	}
	use(event)
	return ok
}

func BothUsedAfter() (string, bool) {
	event, ok := normalize() // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
	if !ok {
		return "", ok
	}
	return event, ok
}

func ExistingVar() string {
	var ok bool
	event, ok := normalize() // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
	if !ok {
		return ""
	}
	return event
}

func Parameter(event string) string {
	event, ok := normalize() // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
	if !ok {
		return ""
	}
	return event
}

func Consecutive() {
	first, ok := normalize() // want "if ok can be inlined by hoisting first to var declarations at the top of the function and changing := to ="
	if !ok {
		return
	}
	second, ok := normalize() // want "if ok can be inlined by hoisting second to var declarations at the top of the function and changing := to ="
	if !ok {
		return
	}
	use(first)
	use(second)
}

func MapLookup(events map[string]string) string {
	event, ok := events["event"] // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
	if !ok {
		return ""
	}
	return event
}

func TypeAssertion(value any) {
	event, ok := value.(string) // want "if ok can be inlined into the assignment"
	if !ok {
		use(event)
	}
}

func ChannelReceive(events <-chan string) string {
	event, ok := <-events // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
	if !ok {
		return ""
	}
	return event
}

func Parenthesized() {
	ok := check() // want "if ok can be inlined into the assignment"
	if !(ok) {
		return
	}
}

func Nested() {
	for i := 0; i < 2; i++ {
		event, ok := normalize() // want "if ok can be inlined by hoisting event to var declarations at the top of the function and changing := to ="
		if !ok {
			return
		}
		use(event)
	}
}

func widgetOK() (*widget.Service, bool) { return nil, true }

func PackageShadow() *widget.Service {
	_ = widget.Service{}
	widget, ok := widgetOK() // want "if ok can be inlined by hoisting widget \\(renamed to service to avoid shadowing\\) to var declarations at the top of the function and changing := to ="
	if !ok {
		return nil
	}
	return widget
}

func AlreadyInline() {
	if event, ok := normalize(); !ok {
		use(event)
	}
}

func OkOnlyUsedAfter() bool {
	ok := check()
	if !ok {
		return false
	}
	return ok
}

func DifferentName() {
	event, found := normalize()
	if !found {
		use(event)
	}
}

func DifferentCondition() {
	event, ok := normalize()
	if event == "" {
		use(event)
	}
	_ = ok
}

func CompoundCondition() {
	event, ok := normalize()
	if !ok || event == "" {
		return
	}
}

func InterveningStatement() {
	event, ok := normalize()
	use(event)
	if !ok {
		return
	}
}

func ExistingIfInit() {
	event, ok := normalize()
	if other := check(); !ok {
		use(event)
		_ = other
	}
}

func MultiLine() {
	event, ok := func() (string, bool) {
		return normalize()
	}()
	if !ok {
		use(event)
	}
}
