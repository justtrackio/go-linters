package noanonstruct

type Named struct{ Value string }
type Alias = struct{ Value string }
type Parenthesized (struct{ Value string })
type Empty struct{}

var named = Named{Value: "ok"}
var empty = struct{}{}
var set map[string]struct{}
var signal chan struct{}
var literal = struct { // want "non-empty anonymous struct"
	Tables []Named `json:"tables"`
}{nil}
var pointer = &struct{ Value int }{1}      // want "non-empty anonymous struct"
var declaration struct{ Value int }        // want "non-empty anonymous struct"
var slice []struct{ Value int }            // want "non-empty anonymous struct"
var mapping map[string]struct{ Value int } // want "non-empty anonymous struct"
type List []struct{ Value int }            // want "non-empty anonymous struct"
type Container struct {
	Nested struct{ Value int } // want "non-empty anonymous struct"
	Empty  struct{}
	Named
}

var embedded struct{ Named } // want "non-empty anonymous struct"

func signature(input struct{ Value int }) struct{ Value int } { // want "non-empty anonymous struct" "non-empty anonymous struct"
	return input
}

func local() {
	type Local struct{ Value int }
	var _ = Local{}
	var _ = struct{ Value int }{} // want "non-empty anonymous struct"
}
