// run is development-only test tooling. Tests and fixtures live under tests/.
package main

import (
	"fmt"
	"gomath/tests/internal/harness"
	"os"
)

func main() {
	cwd, err := os.Getwd()
	if err == nil {
		var root string
		root, err = harness.Root(cwd)
		if err == nil {
			err = harness.Run(root, os.Args[1:], os.Stdout, os.Stderr)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
