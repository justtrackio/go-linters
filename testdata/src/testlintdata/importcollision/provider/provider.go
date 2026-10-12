package provider

import (
	"testlintdata/importcollision/errorvalue"
	"testlintdata/importcollision/okvalue"
)

func ErrorValue() (errorvalue.Value, error) { return errorvalue.Value{}, nil }
func OkValue() (okvalue.Value, bool) { return okvalue.Value{}, true }
