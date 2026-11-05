package export_test

import (
	"bytes"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCameraAndStrokeReachRasterAndSVG(t *testing.T) {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 128, 128, 128, gomath.Black
	for _, cap := range []gomath.StrokeCap{gomath.CapButt, gomath.CapRound, gomath.CapSquare} {
		for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
			// World-space path translated from pixel positions (32,80),(64,80),(64,40).
			o := gomath.Polyline(geom.V(-32, -16), geom.V(0, -16), geom.V(0, 24)).SetStroke(gomath.White, 24)
			o.Style.StrokeCap, o.Style.StrokeJoin, o.Style.MiterLimit = cap, join, 6
			// Apply 2x camera zoom: author geometry/styles at half size first.
			o.ApplyMatrix(geom.Scaling(.5, .5, 1)).Shift(geom.V(2, 3))
			o.Style.StrokeWidth = 12
			cam := gomath.Camera2D{Center: geom.V(2, 3), FrameWidth: 64}
			frame := &gomath.Frame{Objects: []*gomath.Object{o}, Camera: &cam}
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			pixels, err := r.Render(frame)
			if err != nil {
				t.Fatal(err)
			}
			want := uint8(255)
			if cap == gomath.CapButt {
				want = 0
			}
			if got := pixels.RGBAAt(25, 80).R; got != want {
				t.Fatalf("cap %v via raster: %d want %d", cap, got, want)
			}
			// This far corner also verifies the ROI includes the miter extent.
			want = 0
			if join == gomath.JoinMiter {
				want = 255
			}
			if got := pixels.RGBAAt(74, 90).R; got != want {
				t.Fatalf("join %v via raster ROI: %d want %d", join, got, want)
			}
			file := filepath.Join(t.TempDir(), "camera.svg")
			if err = export.SVG(file, frame, c); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			attrs, err := export.StrokeAttributes(o.Style)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), attrs) || !strings.Contains(string(data), `scale(2 -2)`) {
				t.Fatal("camera or stroke absent from SVG", string(data))
			}
			// Reuse the same rasterizer without a camera: its config must be restored.
			plain := &gomath.Frame{Objects: []*gomath.Object{o}}
			got, err := r.Render(plain)
			if err != nil {
				t.Fatal(err)
			}
			fresh, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := fresh.Render(plain)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, expected.Pix) {
				t.Fatal("camera config leaked into next render")
			}
		}
	}
}
