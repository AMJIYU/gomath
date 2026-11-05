// Package harness hosts development-only centralized Go test tooling.
// It never participates in the gomath library or rendering executable.
package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Root finds the repository from an arbitrary directory inside it.
func Root(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, e := os.Stat(filepath.Join(dir, "go.mod")); e == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cannot locate gomath go.mod from %s", start)
		}
		dir = parent
	}
}

// InspectInputs enforces the centralized test layout and reads repository inputs.
// When called from a Go test, the reads and directory visits are recorded by the
// test cache. A change to a child package, fixture, or embedded asset therefore
// invalidates the outer suite's cached result as well as the child's result.
func InspectInputs(root string) error {
	return filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel == ".git" || rel == "artifacts" || rel == "bin" {
				return filepath.SkipDir
			}
			if entry.Name() == "testdata" && !strings.HasPrefix(filepath.ToSlash(rel), "tests/") {
				return fmt.Errorf("test fixtures must live under tests/: %s", rel)
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), "_test.go") && !strings.HasPrefix(filepath.ToSlash(rel), "tests/") {
			return fmt.Errorf("test sources must live under tests/: %s", rel)
		}
		if entry.Type().IsRegular() {
			if _, err := os.ReadFile(name); err != nil {
				return fmt.Errorf("read test input %s: %w", rel, err)
			}
		}
		return nil
	})
}

// Run compiles central tests in their original package using the Go toolchain's
// virtual source overlay. No test files are copied into production directories.
// Arguments are go test flags, plus optional -pkg=<repository-relative-package>.
// Without -pkg every package with central test sources is selected.
func Run(root string, args []string, stdout, stderr io.Writer) error {
	if err := InspectInputs(root); err != nil {
		return err
	}
	caseRoot := filepath.Join(root, "tests", "_cases")
	replacements := map[string]string{}
	packages := map[string]bool{}
	selectors := map[string]bool{}
	var forwarded []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-pkg=") {
			selector := strings.TrimPrefix(arg, "-pkg=")
			selector = strings.TrimPrefix(selector, "gomath/")
			selector = strings.TrimPrefix(selector, "./")
			if selector == "" || selector == "gomath" || selector == "root" {
				selector = "."
			}
			selectors[filepath.ToSlash(selector)] = true
		} else {
			forwarded = append(forwarded, arg)
		}
	}
	err := filepath.WalkDir(caseRoot, func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(caseRoot, source)
		if err != nil {
			return err
		}
		pkgDir := filepath.Dir(rel)
		if pkgDir == "root" {
			pkgDir = "."
		}
		key := filepath.ToSlash(pkgDir)
		if len(selectors) > 0 && !selectors[key] {
			return nil
		}
		if _, err := os.Stat(filepath.Join(root, pkgDir)); err != nil {
			return fmt.Errorf("test package %s: %w", key, err)
		}
		virtual := filepath.Join(root, pkgDir, entry.Name())
		if _, err := os.Lstat(virtual); err == nil {
			return fmt.Errorf("production directory contains test file %s", virtual)
		} else if !os.IsNotExist(err) {
			return err
		}
		if _, exists := replacements[virtual]; exists {
			return fmt.Errorf("duplicate test destination %s", virtual)
		}
		replacements[virtual] = source
		packages[key] = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return fmt.Errorf("no centralized test packages match selection")
	}
	for selector := range selectors {
		if !packages[selector] {
			return fmt.Errorf("unknown test package %q", selector)
		}
	}
	var paths []string
	for p := range packages {
		if p != "." {
			p = "./" + p
		}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	data, err := json.Marshal(struct{ Replace map[string]string }{replacements})
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "gomath-test-overlay-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	overlay := filepath.Join(dir, "overlay.json")
	if err = os.WriteFile(overlay, data, 0600); err != nil {
		return err
	}
	command := []string{"test", "-overlay", overlay, "-tags", "noasm"}
	command = append(command, forwarded...)
	command = append(command, paths...)
	cmd := exec.Command("go", command...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, stdout, stderr
	return cmd.Run()
}
