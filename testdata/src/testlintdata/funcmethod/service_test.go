package funcmethod

import "testing"

type TestService struct{}

func (TestService) Apply() { buildSchemaUpdate() }

func TestHelpers(t *testing.T) {
	planSchemaUpdate()
	buildSchemaUpdate()
	testsOnly()
	_ = chainEnd
}

func testHelper() {}
