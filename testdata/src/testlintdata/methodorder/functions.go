package methodorder

func imageReferenceValue(value any) {} // want "function imageReferenceValue is used only by extractImageReferencesFromValue; declare it after extractImageReferencesFromValue in the same file"

func extractImageReferencesFromValue(value any) {
	imageReferenceValue(value)
	if value != nil {
		extractImageReferencesFromValue(nil)
	}
}

func functionShared() {}
func functionFirst()  { functionShared() }
func functionSecond() { functionShared() }

func functionOrderedCaller() { functionOrderedHelper() }
func functionOrderedHelper() {}

func ExportedFunctionHelper() {}
func functionExportedCaller() { ExportedFunctionHelper() }

func functionCycleFirst()  { functionCycleSecond() }
func functionCycleSecond() { functionCycleFirst() }

func functionMethodHelper()            {} // want "function functionMethodHelper is used only by FunctionCaller; declare it after FunctionCaller in the same file"
func (h OtherHelpers) FunctionCaller() { functionMethodHelper() }

func (h OtherHelpers) functionCalledMethod() {} // want "method functionCalledMethod is used only by methodFunctionCaller; declare it after methodFunctionCaller in the same file"
func methodFunctionCaller()                  { OtherHelpers{}.functionCalledMethod() }

func functionGlobalHelper() {}

var functionGlobal = functionGlobalHelper

func functionGlobalCaller() { functionGlobalHelper() }
