package svg_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func TestSVGGradientIndependentCairoReferences(t *testing.T) {
	for _, name := range []string{"linear", "bbox-affine", "radial", "alpha", "use-clip-bitmap"} {
		t.Run(name, func(t *testing.T) {
			prefix := "../tests/data/svg/gradient-import-" + name
			source, err := os.ReadFile(prefix + ".svg")
			if err != nil {
				t.Fatal(err)
			}
			root, err := svg.Read(strings.NewReader(string(source)), svg.Options{Width: 8})
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(prefix + ".png")
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			config := render.DefaultConfig()
			config.Width, config.Height, config.Workers, config.FrameWidth, config.Background = 128, 96, 1, 8, gomath.Transparent
			raster, err := render.NewRasterizer(config)
			if err != nil {
				t.Fatal(err)
			}
			got, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != want.Bounds() {
				t.Fatal("reference dimensions differ")
			}
			total, maximum, changed := 0, 0, 0
			for y := range 96 {
				for x := range 128 {
					a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
					pixelChanged := false
					for _, pair := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
						d := int(math.Abs(float64(int(pair[0]) - int(pair[1]))))
						total += d
						maximum = max(maximum, d)
						pixelChanged = pixelChanged || d > 8
					}
					if pixelChanged {
						changed++
					}
				}
			}
			mean, ratio := float64(total)/(128*96*4), float64(changed)/(128*96)
			t.Logf("CairoSVG gradient %s 128x96 premultiplied RGBA: MAE=%.9g max=%d fraction>8=%.9g", name, mean, maximum, ratio)
			if mean > .45 || maximum > 50 || ratio > .025 {
				t.Fatal("gradient reference tolerance exceeded")
			}
			// Transform the imported geometry, gradients and clips, then independently invert
			// that rigid transform via camera projection. Source remains immutable.
			original := root.Copy()
			root.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(math.Pi / 6)))
			camera := gomath.Camera2D{Center: geom.V(10, 20), Rotation: math.Pi / 6, FrameWidth: 8}
			moved, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}, Camera: &camera})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, moved.Pix) {
				maximum := 0
				for i, v := range got.Pix {
					maximum = max(maximum, int(math.Abs(float64(int(v)-int(moved.Pix[i])))))
				}
				t.Logf("camera max delta %d", maximum)
				if maximum > 1 {
					t.Fatal("camera transform changed imported gradient/clipping pixels")
				}
			}
			again, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{original}})
			if err != nil || !bytes.Equal(got.Pix, again.Pix) {
				t.Fatal("mutating an imported copy changed its sibling snapshot", err)
			}
		})
	}
}

func TestSVGGradientTextOutlinePixelsUseOneUserSpaceFrame(t *testing.T) {
	// The independently rendered solid outline supplies only coverage. Expected
	// gradient RGB comes from the authored viewport ramp at each pixel center,
	// never from Gradient.ColorAt or imported gradient matrices.
	const outline = `<g opacity=".7"><text x="8" y="60" font-size="40" opacity=".6" fill-opacity=".8">Go<tspan fill-opacity=".4">A</tspan></text></g>`
	source := func(paint string) string {
		return `<svg viewBox="0 0 128 96"><defs><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" x2="128"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs><g fill="` + paint + `">` + outline + `</g></svg>`
	}
	config := render.DefaultConfig()
	config.Width, config.Height, config.Workers, config.FrameWidth, config.Background = 128, 96, 1, 8, gomath.Transparent
	raster, err := render.NewRasterizer(config)
	if err != nil {
		t.Fatal(err)
	}
	roots := make([]*gomath.Object, 2)
	for i, paint := range []string{"white", "url(#g)"} {
		roots[i], err = svg.Read(strings.NewReader(source(paint)), svg.Options{Width: 8})
		if err != nil {
			t.Fatal(err)
		}
	}
	mask, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{roots[0]}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{roots[1]}})
	if err != nil {
		t.Fatal(err)
	}
	painted := 0
	maximum, alphaMaximum := 0, 0
	for y := range 96 {
		for x := range 128 {
			a, b := mask.RGBAAt(x, y), got.RGBAAt(x, y)
			alphaDelta := int(math.Abs(float64(int(a.A) - int(b.A))))
			alphaMaximum = max(alphaMaximum, alphaDelta)
			// Native solid and gradient paths quantize opacity at different
			// stages, so coverage/alpha may differ by one 8-bit unit.
			if alphaDelta > 1 {
				t.Fatalf("gradient changed text coverage at %d,%d: %v vs %v", x, y, a, b)
			}
			if a.A != 0 {
				painted++
			}
			u := (float64(x) + .5) / 128
			for _, p := range [][2]float64{{float64(b.R), float64(b.A) * (1 - u)}, {float64(b.G), 0}, {float64(b.B), float64(b.A) * u}} {
				d := int(math.Abs(p[0] - math.Round(p[1])))
				maximum = max(maximum, d)
				if d > 1 {
					t.Fatalf("text gradient not in shared SVG user space at %d,%d: %v coverage %d", x, y, b, a.A)
				}
			}
		}
	}
	if painted < 300 {
		t.Fatal("text outlines disappeared", painted)
	}
	t.Logf("real glyph outline gradient: %d covered pixels, analytical RGB max quantization delta %d, solid-mask alpha max delta %d", painted, maximum, alphaMaximum)
}
