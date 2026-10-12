package inlineunsafe

import . "testlintdata/iferrinline/widget"

func dotImportedTypeShadow() error {
	type Service string
	value, err := New() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = value
	return nil
}
