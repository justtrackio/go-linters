package funcmethod

func generatedCallerHelper()                  {} // want "function generatedCallerHelper is used only by SetupSchemaService"
func generatedShared()                        {}
func (s SetupSchemaService) GeneratedShared() { generatedShared() }
