package render_test

import (
	"gomath"
	"gomath/examples/scenes"
	"gomath/render"
	"image/png"
	"math"
	"os"
	"testing"
)

// This is a bounded visual regression, not an assertion of pixel identity or
// animation compatibility. The independent golden's provenance is in testdata.
func TestPinnedManimRasterReference(t *testing.T) {
	file, err := os.Open("../tests/data/render/manim-reference-0.22.1.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reference, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	s, err := scenes.Reference()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := s.Sample(.5)
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.Background = 640, 360, 1, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := r.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Bounds() != reference.Bounds() {
		t.Fatal("reference dimensions differ")
	}
	var absolute float64
	maxError, mismatches := 0, 0
	for y := range c.Height {
		for x := range c.Width {
			rr, rg, rb, _ := reference.At(x, y).RGBA()
			ar, ag, ab, _ := actual.At(x, y).RGBA()
			expected, got := [3]uint32{rr, rg, rb}, [3]uint32{ar, ag, ab}
			mismatch := false
			for i := range 3 {
				diff := int(math.Abs(float64(int(expected[i]>>8) - int(got[i]>>8))))
				absolute += float64(diff)
				maxError = max(maxError, diff)
				mismatch = mismatch || diff > 1
			}
			if mismatch {
				mismatches++
			}
		}
	}
	mean := absolute / float64(c.Width*c.Height*3)
	ratio := float64(mismatches) / float64(c.Width*c.Height)
	if mean > .25 || maxError > 16 || ratio > .06 {
		t.Fatalf("Manim visual regression: MAE %.6f, max %d, mismatched pixels %.6f", mean, maxError, ratio)
	}
}
