package funcmethod

func (s *SetupSchemaService) CrossFile() { crossFile() }
func crossFile()                         {} // want "function crossFile is used only by SetupSchemaService"

type CallbackService struct{}

func (s CallbackService) Apply() { acceptCallback(callbackHelper) }
func acceptCallback(fn func())   { fn() } // want "function acceptCallback is used only by CallbackService"
func callbackHelper()            {}

type DualService struct{}

func (s DualService) CrossFile()              { crossFileShared() }
func (s SetupSchemaService) CrossFileShared() { crossFileShared() }
func crossFileShared()                        {}
