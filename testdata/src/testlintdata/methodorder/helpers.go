package methodorder

type contextHandlerGo struct{}

func (c *contextHandlerGo) buildVSCodeGoLaunchConfig() {} // want "method buildVSCodeGoLaunchConfig is used only by writeVSCodeLaunchConfigs; declare it after writeVSCodeLaunchConfigs in the same file"
func (c contextHandlerGo) writeVSCodeLaunchConfigs() {
	c.buildVSCodeGoLaunchConfig()
	c.buildVSCodeGoLaunchConfig()
}

type Helpers[T any] struct{}

func (h Helpers[T]) genericHelper()  {} // want "method genericHelper is used only by GenericCaller; declare it after GenericCaller in the same file"
func (h Helpers[T]) GenericCaller()  { h.genericHelper() }
func (h Helpers[T]) OrderedCaller()  { h.orderedHelper() }
func (h Helpers[T]) orderedHelper()  {}
func (h Helpers[T]) sharedHelper()   {}
func (h Helpers[T]) FirstCaller()    { h.sharedHelper() }
func (h Helpers[T]) SecondCaller()   { h.sharedHelper() }
func (h Helpers[T]) ExportedHelper() {}
func (h Helpers[T]) ExportedCaller() { h.ExportedHelper() }
func (h Helpers[T]) unusedHelper()   {}
func (h Helpers[T]) recursiveHelper(n int) { // want "method recursiveHelper is used only by RecursiveCaller; declare it after RecursiveCaller in the same file"
	if n > 0 {
		h.recursiveHelper(n - 1)
	}
}
func (h Helpers[T]) RecursiveCaller()      { h.recursiveHelper(1) }
func (h Helpers[T]) cycleFirst()           { h.cycleSecond() }
func (h Helpers[T]) cycleSecond()          { h.cycleFirst() }
func (h Helpers[T]) crossFileHelper()      {}
func (h Helpers[T]) localCrossFileCaller() { h.crossFileHelper() }
func (h Helpers[T]) globalHelper()         {}

var globalMethod = Helpers[int].globalHelper

func (h Helpers[T]) GlobalCaller()     { h.globalHelper() }
func (h Helpers[T]) expressionHelper() {} // want "method expressionHelper is used only by ExpressionCaller; declare it after ExpressionCaller in the same file"
func (h Helpers[T]) ExpressionCaller() { Helpers[T].expressionHelper(h) }
func (h Helpers[T]) valueHelper()      {} // want "method valueHelper is used only by ValueCaller; declare it after ValueCaller in the same file"
func (h Helpers[T]) ValueCaller() {
	callback := h.valueHelper
	callback()
}

type OtherHelpers struct{}

func (h OtherHelpers) sharedHelper() {} // want "method sharedHelper is used only by Caller; declare it after Caller in the same file"
func (h OtherHelpers) Caller()       { h.sharedHelper() }

func (h Helpers[T]) generatedSharedHelper() {}
func (h Helpers[T]) GeneratedSharedCaller() { h.generatedSharedHelper() }

// Unit tests must not hide a helper's sole production caller.
type K3dService struct{}

func (s *K3dService) availableImageReferences() {} // want "method availableImageReferences is used only by PullImages; declare it after PullImages in the same file"

func (s *K3dService) PullImages() { s.availableImageReferences() }

func (s *K3dService) testOnlyHelper() {}
