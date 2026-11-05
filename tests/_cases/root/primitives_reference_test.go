package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"image/png"
	"math"
	"os"
	"testing"
)

func TestPinnedManimPrimitiveRasterReference(t *testing.T) {
	const base = "tests/data/root/manim-primitives-0.22.1"
	data, err := os.ReadFile(base + ".json")
	must(t, err)
	var provenance struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		PNG string `json:"png_sha256"`
	}
	must(t, json.Unmarshal(data, &provenance))
	generator, err := os.ReadFile("tests/reference/primitives.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if provenance.Source.Version != "0.22.1" || provenance.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || provenance.Source.Hash != hex.EncodeToString(hash[:]) {
		t.Fatal("primitive reference provenance differs")
	}
	data, err = os.ReadFile(base + ".png")
	must(t, err)
	hash = sha256.Sum256(data)
	if provenance.PNG != hex.EncodeToString(hash[:]) {
		t.Fatal("primitive reference image hash differs")
	}
	file, err := os.Open(base + ".png")
	must(t, err)
	defer file.Close()
	reference, err := png.Decode(file)
	must(t, err)
	ellipse, err := gomath.Ellipse(2.5, 1.2)
	must(t, err)
	annulus, err := gomath.Annulus(.4, .9)
	must(t, err)
	wedge, err := gomath.AnnularSector(.35, 1, .3, -4.2)
	must(t, err)
	rounded, err := gomath.RoundedRectangle(3, 1, .25)
	must(t, err)
	sector, err := gomath.Sector(.9, .2, 2.3)
	must(t, err)
	objects := []*gomath.Object{ellipse.Shift(geom.V(-3, 1)), annulus.Shift(geom.V(0, 1)), wedge.Shift(geom.V(3, 1)), rounded.Shift(geom.V(-2, -1.1)), sector.Shift(geom.V(2, -1.2))}
	s := gomath.NewScene()
	for _, o := range objects {
		o.SetFill(gomath.White).SetStroke(gomath.Transparent, 0)
		must(t, s.Add(o))
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 640, 360, 10, gomath.Black
	r, err := render.NewRasterizer(c)
	must(t, err)
	f, err := s.Sample(0)
	must(t, err)
	actual, err := r.Render(f)
	must(t, err)
	if actual.Bounds() != reference.Bounds() {
		t.Fatal("primitive reference dimensions differ")
	}
	var total float64
	maximum, different := 0, 0
	for y := range c.Height {
		for x := range c.Width {
			rr, _, _, _ := reference.At(x, y).RGBA()
			ar := actual.RGBAAt(x, y).R
			diff := int(math.Abs(float64(int(rr>>8) - int(ar))))
			total += float64(diff)
			maximum = max(maximum, diff)
			if diff > 1 {
				different++
			}
		}
	}
	mean, ratio := total/float64(c.Width*c.Height), float64(different)/float64(c.Width*c.Height)
	t.Logf("five primitive raster: MAE %.6f/255, max %d, differing-pixel ratio %.6f", mean, maximum, ratio)
	if mean > .5 || maximum > 64 || ratio > .025 {
		t.Fatal("primitive reference visual regression exceeds bounded thresholds")
	}
}
