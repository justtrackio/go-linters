package main

import (
	"log"

	linters "github.com/justtrackio/go-linters"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	plugin, err := linters.NewFuncMethod(nil)
	if err != nil {
		log.Fatalf("init plugin: %v", err)
	}
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil {
		log.Fatalf("build analyzers: %v", err)
	}
	singlechecker.Main(analyzers[0])
}
