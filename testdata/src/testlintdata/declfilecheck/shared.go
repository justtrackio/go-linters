package declfilecheck

type shared struct{}
type sharedDependency struct{}

// The shared types make this file a package collection, including this helper.
type collectionHelper struct{}

const collectionConstant = 1

var collectionVariable = 1
