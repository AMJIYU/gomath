package code

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeFrozenCandidateAndReferenceHashes(t *testing.T) {
	var manifest struct {
		Schema int
		Manim  struct{ Commit, Path, SHA256 string }
		Files  map[string]string
	}
	if err := json.Unmarshal(fixture(t, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != 1 || len(manifest.Files) < 40 || manifest.Manim.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || manifest.Manim.SHA256 != "d88f13c596745ac55e303a9ad26e6618ae1cae61697084a9d8f1a0e564a28088" {
		t.Fatal("incomplete source/reference freeze")
	}
	root, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for {
		if _, e := os.Stat(filepath.Join(root, "go.mod")); e == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("missing repository root")
		}
		root = parent
	}
	for logical, want := range manifest.Files {
		actual := filepath.Join(root, filepath.FromSlash(logical))
		if _, err := os.Stat(actual); os.IsNotExist(err) {
			if strings.HasPrefix(logical, "code/") {
				actual = filepath.Join(root, "code/_staging", strings.TrimPrefix(logical, "code/"))
			}
			if strings.HasPrefix(logical, "tests/_cases/code/") {
				actual = filepath.Join(root, "tests/_cases/code/_staging", strings.TrimPrefix(logical, "tests/_cases/code/"))
			}
		}
		data, e := os.ReadFile(actual)
		if e != nil {
			t.Fatal(logical, e)
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != want {
			t.Errorf("frozen hash changed: %s", logical)
		}
	}
}
