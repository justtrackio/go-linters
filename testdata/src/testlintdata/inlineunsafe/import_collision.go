package inlineunsafe

import (
	"testlintdata/importcollision/p1"
	"testlintdata/importcollision/p2"
)

func importAliasErrorCollision() error {
	var err error
	value, err := p1.New() // want "if err cannot be safely inlined automatically"
	if err != nil {
		return err
	}
	_ = value
	_ = err
	return nil
}

func importAliasOkCollision() bool {
	var ok bool
	value, ok := p2.New()
	if !ok {
		return false
	}
	_ = value
	_ = ok
	return true
}
