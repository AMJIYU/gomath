package harness

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, root, name, source string) {
	t.Helper()
	name = filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestOuterSuiteCacheTracksChildSourcesAndFixtures(t *testing.T) {
	t.Setenv("GOWORK", "off")
	root := filepath.Join(t.TempDir(), "cached suite")
	source, err := os.ReadFile("harness.go")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "go.mod", "module sample\n\ngo 1.26.0\n")
	write(t, root, "sample.go", "package sample\nfunc private() int { return 42 }\n")
	write(t, root, "tests/internal/harness/harness.go", string(source))
	write(t, root, "tests/suite_test.go", `package tests
import ("os"; "testing"; "sample/tests/internal/harness")
func TestSuite(t *testing.T) {
 cwd,err:=os.Getwd();if err!=nil { t.Fatal(err) }
 root,err:=harness.Root(cwd);if err!=nil { t.Fatal(err) }
 if err=harness.Run(root,nil,os.Stdout,os.Stderr);err!=nil { t.Fatal(err) }
}
`)
	write(t, root, "tests/_cases/root/private_test.go", `package sample
import ("os"; "testing")
func TestPrivate(t *testing.T) {
 if private()!=42 { t.Fatal("private dependency changed") }
 data,err:=os.ReadFile("tests/data/value.txt")
 if err!=nil || string(data)!="good" { t.Fatal("fixture dependency changed",err) }
}
`)
	write(t, root, "tests/data/value.txt", "good")
	// Go refuses caching for files modified in the past two seconds. Age these
	// temporary inputs explicitly so the check does not need sleeps or timing luck.
	stamp := time.Now().Add(-time.Hour)
	err = filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(name, stamp, stamp)
	})
	if err != nil {
		t.Fatal(err)
	}
	run := func() ([]byte, error) {
		cmd := exec.Command("go", "test", "-tags", "noasm", "./tests")
		cmd.Dir = root
		return cmd.CombinedOutput()
	}
	if out, err := run(); err != nil {
		t.Fatalf("initial suite: %v\n%s", err, out)
	}
	if out, err := run(); err != nil || !bytes.Contains(out, []byte("(cached)")) {
		t.Fatalf("suite did not establish reusable cache: %v\n%s", err, out)
	}
	write(t, root, "sample.go", "package sample\nfunc private() int { return 41 }\n")
	if out, err := run(); err == nil || !bytes.Contains(out, []byte("private dependency changed")) {
		t.Fatalf("outer cache concealed source failure: %v\n%s", err, out)
	}
	write(t, root, "sample.go", "package sample\nfunc private() int { return 42 }\n")
	write(t, root, "tests/data/value.txt", "broken")
	if out, err := run(); err == nil || !bytes.Contains(out, []byte("fixture dependency changed")) {
		t.Fatalf("outer cache concealed fixture failure: %v\n%s", err, out)
	}
}

func TestOverlayPrivateExternalAndFailure(t *testing.T) {
	t.Setenv("GOWORK", "off")
	root := filepath.Join(t.TempDir(), "project with spaces")
	write(t, root, "go.mod", "module sample\n\ngo 1.26.0\n")
	write(t, root, "sample.go", "package sample\nfunc private() int { return 42 }\nfunc Public() int { return private() }\n")
	write(t, root, "tests/_cases/root/private_test.go", `package sample
import "testing"
func TestPrivate(t *testing.T) { if private() != 42 { t.Fatal("private") } }
`)
	write(t, root, "tests/_cases/root/external_test.go", `package sample_test
import ("testing"; "sample"; "os")
func TestExternal(t *testing.T) {
 if sample.Public() != 42 { t.Fatal("public") }
 if _, err := os.ReadFile("tests/data/value.txt"); err != nil { t.Fatal(err) }
}
`)
	write(t, root, "tests/data/value.txt", "fixture")
	got, err := Root(filepath.Join(root, "tests/_cases/root"))
	if err != nil || got != root {
		t.Fatalf("Root = %q, %v", got, err)
	}
	var output bytes.Buffer
	if err = Run(root, []string{"-pkg=root", "-count=1"}, &output, &output); err != nil {
		t.Fatalf("overlay tests: %v\n%s", err, &output)
	}
	for _, name := range []string{"private_test.go", "external_test.go"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("test leaked into production directory: %s (%v)", name, err)
		}
	}
	if err = Run(root, []string{"-pkg=missing"}, &output, &output); err == nil {
		t.Fatal("unknown package selection accepted")
	}
	write(t, root, "tests/_cases/root/private_test.go", `package sample
import "testing"
func TestPrivate(t *testing.T) { t.Fatal("expected-child-failure") }
`)
	output.Reset()
	if err = Run(root, []string{"-pkg=root", "-count=1"}, &output, &output); err == nil || !strings.Contains(output.String(), "expected-child-failure") {
		t.Fatalf("child failure not propagated: %v\n%s", err, &output)
	}
}

func TestCentralLayoutGuard(t *testing.T) {
	for _, name := range []string{"sample_test.go", "pkg/sample_test.go", "pkg/testdata/input.txt"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, name, "")
			if err := InspectInputs(root); err == nil || !strings.Contains(err.Error(), "under tests/") {
				t.Fatalf("scattered test accepted: %v", err)
			}
		})
	}
	root := t.TempDir()
	write(t, root, "tests/_cases/root/sample_test.go", "")
	write(t, root, "tests/data/value.txt", "")
	if err := InspectInputs(root); err != nil {
		t.Fatal(err)
	}
}
