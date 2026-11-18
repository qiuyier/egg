package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/qiuyier/egg/internal/scaffold"
)

func main() {
	out := flag.String("out", "", "path of the new project (must not exist)")
	module := flag.String("module", "", "Go module path of the new project")
	name := flag.String("name", "", "project name (defaults to the output directory name)")
	example := flag.String("example", "", "include a runnable example (inventory)")
	flag.Parse()
	if *out == "" || *module == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	root, err := os.Getwd()
	if err == nil {
		var path string
		path, err = scaffold.Generate(os.DirFS(root), root, scaffold.Options{
			Out: *out, Module: *module, Name: *name, Example: *example,
		})
		if err == nil {
			fmt.Printf("Created %s (%s)\n", path, *module)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "new-project:", err)
	os.Exit(1)
}
