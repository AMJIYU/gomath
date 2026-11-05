package mathtex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/path"
	"os"
	"path/filepath"
	"testing"
)

type legacyLayoutFixture struct {
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

// Exclude fresh identities and include every exact path control point, style,
// drawable order and baseline metric. These pin this library's prior layout,
// not independent TeX/Manim output.
func legacyLayoutDigest(t *testing.T, source string) string {
	t.Helper()
	b := mustLayout(t, source)
	type drawable struct {
		Path   path.Path
		Style  gomath.Style
		ZIndex int
	}
	value := struct {
		Width, Height, Depth float64
		Drawables            []drawable
	}{Width: b.width, Height: b.height, Depth: b.depth}
	b.object.Walk(func(o *gomath.Object) {
		if len(o.Path.Contours) > 0 {
			value.Drawables = append(value.Drawables, drawable{o.Path, o.Style, o.ZIndex})
		}
	})
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func TestLegacyLayoutControlPointFixtures(t *testing.T) {
	filename := filepath.Join("..", "tests", "data", "mathtex", "legacy-layout.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []legacyLayoutFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 8 {
		t.Fatal("incomplete legacy fixtures")
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Source, func(t *testing.T) {
			if got := legacyLayoutDigest(t, fixture.Source); got != fixture.SHA256 {
				t.Fatal("pre-extension layout/control points changed", got, fixture.SHA256)
			}
		})
	}
}
