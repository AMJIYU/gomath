package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The overlay preserves package-private access; relocate Go's failure corpus
// as well as fixture reads before both the fuzz coordinator and workers run.
func TestMain(m *testing.M) {
	start, err := os.Getwd()
	if err == nil {
		for dir := start; ; dir = filepath.Dir(dir) {
			if info, e := os.Stat(filepath.Join(dir, "go.mod")); e == nil && !info.IsDir() {
				err = os.Chdir(filepath.Join(dir, "tests", "data", "audio"))
				break
			}
			if filepath.Dir(dir) == dir {
				err = fmt.Errorf("audio tests: cannot locate module from %s", start)
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

func TestCentralAudioFuzzDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil || filepath.Base(wd) != "audio" || filepath.Base(filepath.Dir(wd)) != "data" || filepath.Base(filepath.Dir(filepath.Dir(wd))) != "tests" {
		t.Fatal("fuzz corpus must remain under tests/data/audio", wd, err)
	}
}
