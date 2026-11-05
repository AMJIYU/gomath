package network_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	start, e := os.Getwd()
	if e == nil {
		for dir := start; ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				e = os.Chdir(filepath.Join(dir, "tests/data/network"))
				break
			}
			if filepath.Dir(dir) == dir {
				e = fmt.Errorf("repository not found from %s", start)
				break
			}
		}
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
