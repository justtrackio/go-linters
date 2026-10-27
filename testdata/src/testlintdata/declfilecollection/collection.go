package declfilecollection

type Settings struct{ value ProjectSettings }
type ProjectSettings struct{}

type privateSettings struct{}
type privateDetail struct{ settings privateSettings }

const privateConstant = 1

var privateVariable = 1

func ReadSettings() Settings { return Settings{} }
