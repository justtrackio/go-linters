package recvaftertype

func (TestType) Before() {} // want "method Before must be declared after receiver type TestType in the same file"

type TestType struct{}

func (TestType) After() {}
