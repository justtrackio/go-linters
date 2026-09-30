package noanonstruct

import "testing"

var fixture struct{ Value string }

func TestAllowed(t *testing.T) {
	cases := []struct{ Name string }{{Name: "allowed"}}
	for _, tc := range cases {
		t.Log(tc.Name)
	}
}
