package sparse

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// All fuzz failure corpora belong under tests/data/sparse, even with overlays.
func TestMain(m *testing.M) {
	start, err := os.Getwd()
	if err == nil {
		for dir := start; ; dir = filepath.Dir(dir) {
			if _, e := os.Stat(filepath.Join(dir, "go.mod")); e == nil {
				err = os.Chdir(filepath.Join(dir, "tests", "data", "sparse"))
				break
			}
			if filepath.Dir(dir) == dir {
				err = fmt.Errorf("cannot locate sparse test data from %s", start)
				break
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
