package intest

import "testing"

func TestHelpers(t *testing.T) {
	_ = missingImageReferences([]string{"two:v1"}, nil)
	testChain()
	recursiveA()
	_ = generic(1)
	_ = callback
	shared()
	fromInitializer()
	fromInit()
	fromMethod()
	worker{}.testOnlyMethod()
	generatedCallerHelper()
	generatedOnly()
	closureHelper()
	productionCallback()
	fromBlankInitializer()
	shadowed := func() {}
	shadowed()
	testFileHelper()
}

func testFileHelper() {}

var testInitializer = missingImageReferences(nil, nil)
