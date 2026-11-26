// Command httpauthz checks production HTTP registrations, including generated files.
package main

import (
	linters "github.com/justtrackio/go-linters"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(linters.HTTPAuthzAnalyzer())
}
