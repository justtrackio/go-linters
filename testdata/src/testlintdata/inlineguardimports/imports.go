package inlineguardimports

import "testlintdata/inlineguardimports/provider"

func safeOwner() {
	value, err := provider.Pair()
	if err != nil { return }
	_ = value
}

func unsafeConsumer() {
	{
		value, err := provider.Pair()
		if err != nil { return }
		_ = value
	}
	err := error(nil)
	_ = err
}
