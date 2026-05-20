package main

import (
	"log"

	"golang.org/x/tools/go/analysis/singlechecker"

	linters "github.com/justtrackio/go-linters"
)

func main() {
	plugin, err := linters.New(nil)
	if err != nil {
		log.Fatalf("init plugin: %v", err)
	}
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil {
		log.Fatalf("build analyzers: %v", err)
	}
	singlechecker.Main(analyzers[0])
}
