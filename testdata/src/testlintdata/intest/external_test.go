package intest_test

import (
	"testing"

	"testlintdata/intest"
)

func TestAPI(t *testing.T) { intest.PublicAPI() }
