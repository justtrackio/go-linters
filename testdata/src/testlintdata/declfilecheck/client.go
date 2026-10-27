package declfilecheck

func Client() {
	_ = alias{}
	_ = generic[int]{}
	_ = first + second
	_ = derived
	_ = shared{}
	_ = collectionHelper{}
	_ = collectionConstant
	_ = collectionVariable
	_ = privateConstant
	_ = methodOwner{}.Value()
	_ = oneUser{}
	_ = pairA
	_ = sharedFirst
	_ = taskPrefix
	_ = privateIndependent
	_ = explicitIotaFirst
	_ = implicitFirst
	_ = constPairA
	_ = dependent
	_ = generatedDeclaration{}
	_ = guardedDeclaration{}
	// A local declaration with the same name must not count as a package use.
	type local struct{}
	_ = local{}
}
