package subtitle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The overlay puts tests in the production package for compilation, but Go's
// fuzz coordinator saves failures relative to its process working directory.
// Move both ordinary tests and fuzz workers to the central fixture directory
// before m.Run, so testdata/fuzz can never appear under subtitle/.
func TestMain(m *testing.M) {
	if err := useCentralTestDirectory(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func useCentralTestDirectory() error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}
	// Workers may inherit the already-centralized directory. Walking ancestors
	// handles both that case and the original production-package test directory.
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return os.Chdir(filepath.Join(dir, "tests", "data", "subtitle"))
		}
		if filepath.Dir(dir) == dir {
			return fmt.Errorf("subtitle tests: cannot locate module from %s", start)
		}
	}
}

func TestCentralFuzzWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	wantSuffix := filepath.Join("tests", "data", "subtitle")
	if filepath.Base(wd) != "subtitle" || filepath.Base(filepath.Dir(wd)) != "data" || filepath.Base(filepath.Dir(filepath.Dir(wd))) != "tests" {
		t.Fatalf("fuzz failure corpus would escape %s: cwd=%s", wantSuffix, wd)
	}
	// TestMain must also work when re-entered from a fuzz worker's inherited cwd.
	if err := useCentralTestDirectory(); err != nil {
		t.Fatal(err)
	}
	got, err := os.Getwd()
	if err != nil || got != wd {
		t.Fatal("central test-directory setup is not idempotent", got, err)
	}
}
