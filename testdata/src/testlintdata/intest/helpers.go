package intest

func missingImageReferences(images []string, existing map[string]struct{}) []string { // want "function missingImageReferences is used only by tests"
	var missing []string
	for _, image := range images {
		if _, ok := existing[image]; !ok {
			missing = append(missing, image)
		}
	}

	return missing
}

func testChain() { // want "function testChain is used only by tests"
	testLeaf()
}

func testLeaf() {} // want "function testLeaf is used only by tests"

func recursiveA() { // want "function recursiveA is used only by tests"
	recursiveB()
}

func recursiveB() { // want "function recursiveB is used only by tests"
	recursiveA()
}

func generic[T any](value T) T { // want "function generic is used only by tests"
	return value
}

func callback() {} // want "function callback is used only by tests"

func unused() {}

func unusedCycleA() { unusedCycleB() }

func unusedCycleB() { unusedCycleA() }

func production() { shared() }

func shared() {}

func fromInitializer() int { return 1 }

var initialized = fromInitializer()

func fromInit() {}

func init() { fromInit() }

func Exported() { production() }

func PublicAPI() {}

func fromMethod() {}

type worker struct{}

func (worker) run() { fromMethod() }

func (worker) testOnlyMethod() {}

func generatedCallerHelper() {}

func closureHelper() {}

func Closure() {
	_ = func() { closureHelper() }
}

func productionCallback() {}

var registeredCallback = productionCallback

func shadowed() {}

func fromBlankInitializer() int { return 0 }

var _ = fromBlankInitializer()
