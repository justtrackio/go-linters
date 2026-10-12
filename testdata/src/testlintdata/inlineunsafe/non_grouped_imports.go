package inlineunsafe

import "testlintdata/importcollision/provider"

func nonGroupedImportEditPlansDiffer() error {
	var err error
	errorValue, err := provider.ErrorValue() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	var ok bool
	okValue, ok := provider.OkValue()
	if !ok {
		return nil
	}
	_ = errorValue
	_ = okValue
	_ = err
	_ = ok
	return nil
}
