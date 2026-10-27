package methodorder

func (TestType) Before() {} // want "method Before must be declared after receiver type TestType in the same file"

type TestType struct{}

func (TestType) After() {}

func testImageReferences() {
	s := &K3dService{}
	s.availableImageReferences()
	s.testOnlyHelper()
}

func (t TestType) testHelper() {} // want "method testHelper is used only by TestCaller; declare it after TestCaller in the same file"
func (t TestType) TestCaller() { t.testHelper() }

func testImageReferenceValue() { imageReferenceValue(nil) }
