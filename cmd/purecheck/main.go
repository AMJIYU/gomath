// purecheck is a development audit tool. It invokes the Go toolchain to inspect
// actual build inputs; the gomath library and renderer never launch programs.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type pkg struct {
	ImportPath                                                                              string
	Standard                                                                                bool
	Imports, CgoFiles, CFiles, CXXFiles, MFiles, SwigFiles, SwigCXXFiles, SFiles, SysoFiles []string
}

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	cmd := exec.Command("go", "list", "-deps", "-json", "-tags", "noasm", "./...")
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "CGO_ENABLED=") {
			env = append(env, v)
		}
	}
	cmd.Env = append(env, "CGO_ENABLED=0")
	cmd.Stderr = os.Stderr
	pipe, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	if e = cmd.Start(); e != nil {
		return e
	}
	dec := json.NewDecoder(pipe)
	count := 0
	var auditErr error
	for {
		var p pkg
		if e := dec.Decode(&p); e == io.EOF {
			break
		} else if e != nil {
			auditErr = e
			break
		}
		if p.Standard {
			continue
		}
		count++
		if len(p.CgoFiles)+len(p.CFiles)+len(p.CXXFiles)+len(p.MFiles)+len(p.SwigFiles)+len(p.SwigCXXFiles)+len(p.SFiles)+len(p.SysoFiles) > 0 {
			auditErr = fmt.Errorf("non-Go build input in %s", p.ImportPath)
		}
		if strings.Contains(p.ImportPath, "/purego") || strings.Contains(p.ImportPath, "/go-gl/") {
			auditErr = fmt.Errorf("native binding dependency: %s", p.ImportPath)
		}
		development := strings.HasPrefix(p.ImportPath, "gomath/cmd/") || strings.HasPrefix(p.ImportPath, "gomath/tests/")
		if strings.HasPrefix(p.ImportPath, "gomath/") && !development || p.ImportPath == "gomath" {
			for _, i := range p.Imports {
				if strings.HasPrefix(i, "gomath/tests/") {
					auditErr = fmt.Errorf("test tooling imported by runtime package %s", p.ImportPath)
				}
				if i == "os/exec" || i == "plugin" || i == "C" {
					auditErr = fmt.Errorf("forbidden runtime import %s in %s", i, p.ImportPath)
				}
			}
		}
	}
	if e := cmd.Wait(); e != nil {
		return e
	}
	if auditErr != nil {
		return auditErr
	}
	fmt.Printf("PASS: %d non-standard packages; CGO_ENABLED=0; noasm; no native inputs or runtime process launch imports\n", count)
	return nil
}
