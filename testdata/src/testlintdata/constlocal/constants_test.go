package constlocal

import "testing"

const testOnly = "test" // want "constant testOnly is used only by function TestConstant"

func TestConstant(t *testing.T) {
	t.Log(testOnly)
}
