package tests

import (
	"gomath/tests/internal/harness"
	"os"
	"testing"
)

func TestCentralSuite(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := harness.Root(cwd)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-vet=all"}
	if raceEnabled {
		args = append(args, "-race")
	}
	if testing.Short() {
		args = append(args, "-short")
	}
	if testing.Verbose() {
		args = append(args, "-v")
	}
	if err = harness.Run(root, args, os.Stdout, os.Stderr); err != nil {
		t.Fatal(err)
	}
}
