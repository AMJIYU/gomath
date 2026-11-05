package svg_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"gomath/geom"
	"gomath/svg"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func imageURI(t testing.TB, name, mime string) string {
	t.Helper()
	data, err := os.ReadFile("../tests/data/svg/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return "data:image/" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}
func imageDoc(uri, attrs string) string {
	return `<svg viewBox="0 0 100 100" xmlns:xlink="http://www.w3.org/1999/xlink"><image href="` + uri + `" ` + attrs + `/></svg>`
}
func TestEmbeddedImageFormats(t *testing.T) {
	for _, tc := range []struct{ name, mime string }{{"embedded.png", "png"}, {"embedded.jpg", "jpeg"}, {"embedded.gif", "gif"}} {
		t.Run(tc.mime, func(t *testing.T) {
			uri := imageURI(t, tc.name, tc.mime)
			root := readInstances(t, imageDoc(uri, `x="10" y="20" width="40" height="20"`))
			shapes := instanceShapes(root)
			if len(shapes) != 1 || shapes[0].Bitmap == nil {
				t.Fatalf("missing bitmap: %v", shapes)
			}
			assertInstanceBox(t, shapes[0], [4]float64{10, 20, 40, 20})
			data, err := os.ReadFile("../tests/data/svg/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			decoded, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			referenceData, err := os.ReadFile("../tests/data/svg/" + tc.name + "-decoded.png")
			if err != nil {
				t.Fatal(err)
			}
			independent, err := png.Decode(bytes.NewReader(referenceData))
			if err != nil {
				t.Fatal(err)
			}
			bitmap := shapes[0].Bitmap
			maxDelta := 0
			for y := range independent.Bounds().Dy() {
				for x := range independent.Bounds().Dx() {
					a, b := bitmap.RGBAAt(x, y), color.RGBAModel.Convert(independent.At(x, y)).(color.RGBA)
					for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
						maxDelta = max(maxDelta, int(math.Abs(float64(d))))
					}
				}
			}
			tolerance := 0
			if tc.mime == "jpeg" {
				tolerance = 3
			}
			t.Logf("Pillow complete %s frame-zero decode max RGBA delta=%d", tc.mime, maxDelta)
			if maxDelta > tolerance {
				t.Fatalf("independent decoder mismatch %d > %d", maxDelta, tolerance)
			}
			for y := range decoded.Bounds().Dy() {
				for x := range decoded.Bounds().Dx() {
					if got, want := bitmap.RGBAAt(x, y), color.RGBAModel.Convert(decoded.At(x, y)).(color.RGBA); got != want {
						t.Fatalf("pixel %d,%d: %v != %v", x, y, got, want)
					}
				}
			}
			if tc.mime == "gif" && bitmap.RGBAAt(0, 0) != (color.RGBA{R: 255, A: 255}) {
				t.Fatal("GIF did not use first frame")
			}
		})
	}
}
func TestEmbeddedImagePlacement(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	for _, tc := range []struct {
		attrs string
		box   [4]float64
	}{
		{`x="10" y="20" width="40" height="40"`, [4]float64{10, 30, 40, 20}},
		{`x="10" y="20" width="40" height="40" preserveAspectRatio="xMaxYMax meet"`, [4]float64{10, 40, 40, 20}},
		{`x="10" y="20" width="40" height="40" preserveAspectRatio="none"`, [4]float64{10, 20, 40, 40}},
		{`x="10%" y="20%" width="40%" height="40%" preserveAspectRatio="xMinYMin"`, [4]float64{10, 20, 40, 20}},
		{`width="32"`, [4]float64{0, 0, 32, 16}},
		{`height="12"`, [4]float64{0, 0, 24, 12}},
		{``, [4]float64{0, 0, 16, 8}},
		{`width="0" height="10" preserveAspectRatio="none"`, [4]float64{0, 0, 0, 10}},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			assertInstanceBox(t, instanceShapes(readInstances(t, imageDoc(uri, tc.attrs)))[0], tc.box)
		})
	}
	// Keep pixel rows upright through SVG Y-down to world Y-up conversion.
	img := renderInstances(t, imageDoc(uri, `x="10" y="20" width="40" height="20"`), 100, 100)
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{
		{15, 24, color.RGBA{240, 24, 16, 255}}, {45, 24, color.RGBA{12, 157, 36, 192}},
		{15, 36, color.RGBA{12, 24, 112, 128}}, {45, 36, color.RGBA{240, 200, 16, 255}},
	} {
		if got := img.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Fatalf("orientation pixel %d,%d: %v want %v", tc.x, tc.y, got, tc.want)
		}
	}
}
func TestEmbeddedImageAffineAndOwnership(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	source := `<svg viewBox="0 0 100 100"><defs><image id="a" href="` + uri + `" width="16" height="8"/></defs><use href="#a" x="10" y="20" transform="matrix(1 .5 .25 2 3 4)"/><use href="#a" x="60" y="30" opacity=".5"/><image href="` + uri + `" width="16" height="8" transform="translate(70 70) rotate(90) scale(-1 1)"/></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 3 {
		t.Fatalf("images %d", len(shapes))
	}
	a, b, c := shapes[0], shapes[1], shapes[2]
	if a.ID == b.ID || a.Bitmap != b.Bitmap || b.Bitmap != c.Bitmap {
		t.Fatal("instance identity or immutable asset sharing failed")
	}
	tl, tr, bl, err := a.ImageCorners()
	if err != nil {
		t.Fatal(err)
	}
	for i, got := range []geom.Vec3{tl, tr, bl} {
		want := []geom.Vec3{geom.V(18-50, 50-49), geom.V(34-50, 50-57), geom.V(20-50, 50-65)}[i]
		if got.Sub(want).Norm() > 1e-8 {
			t.Fatalf("affine corner %d %v want %v", i, got, want)
		}
	}
	before := b.Path.Bounds()
	a.Shift(geom.V(9, 9)).SetOpacity(.1)
	if b.Path.Bounds() != before || b.Style.Opacity != 1 {
		t.Fatal("use instances share mutable geometry/style")
	}
	copy := b.Copy()
	if copy.Bitmap != b.Bitmap || copy.ID == b.ID {
		t.Fatal("bitmap snapshot ownership")
	}
	for i := range 4 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			other := readInstances(t, source)
			if instanceShapes(other)[0].Bitmap == a.Bitmap {
				t.Fatal("cache leaked between imports")
			}
		})
	}
}
func TestEmbeddedImageCSSOpacityAndHref(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	source := `<svg viewBox="0 0 100 100" xmlns:xlink="http://www.w3.org/1999/xlink"><style>image{opacity:.5;fill:red;stroke:blue;stroke-width:4;fill-opacity:0}</style><defs><image id="a" xlink:href="` + uri + `" width="40" height="20"/></defs><g opacity=".5"><use href="#a" x="10" y="20" opacity=".5"/></g><image href="` + uri + `" xlink:href="external.png" x="60" y="20" width="40" height="20" style="opacity:1"/></svg>`
	img := renderInstances(t, source, 100, 100)
	if got := img.RGBAAt(15, 24); got != (color.RGBA{30, 3, 2, 32}) {
		t.Fatalf("accumulated opacity: %v", got)
	}
	if got := img.RGBAAt(65, 24); got != (color.RGBA{240, 24, 16, 255}) {
		t.Fatalf("href precedence or inline opacity: %v", got)
	}
	for _, attrs := range []string{`style="display:none"`, `visibility="hidden"`, `opacity="0"`} {
		hidden := renderInstances(t, imageDoc(uri, attrs+` width="40" height="20"`), 100, 100)
		if hidden.RGBAAt(5, 4).A != 0 {
			t.Fatal("hidden image rendered")
		}
	}
}
func TestEmbeddedImageFailures(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	for _, tc := range []struct{ name, source, want string }{
		{"external", imageDoc("https://example.test/a.png", ``), "data URI"},
		{"local path", imageDoc("file:///tmp/a.png", ``), "data URI"},
		{"fragment", imageDoc("#other", ``), "data URI"},
		{"missing", imageDoc("", ``), "data URI"},
		{"percent", imageDoc("data:image/png,%89PNG", ``), "data URI"},
		{"bad64", imageDoc("data:image/png;base64,!!!!", ``), "base64"},
		{"folded", imageDoc("data:image/png;base64,AAAA AA=", ``), "base64"},
		{"mime", imageDoc(strings.Replace(uri, "image/png", "image/jpeg", 1), ``), "MIME mismatch"},
		{"truncated", imageDoc(uri[:len(uri)-8], ``), "decode"},
		{"slice", imageDoc(uri, `preserveAspectRatio="xMidYMid slice"`), "slice/clipping"},
		{"bad aspect", imageDoc(uri, `preserveAspectRatio="defer xMidYMid meet"`), "meet/none"},
		{"negative", imageDoc(uri, `width="-1"`), "width"},
		{"nan", imageDoc(uri, `height="NaN"`), "height"},
		{"clip", imageDoc(uri, `clip-path="url(#a)"`), "clip-path"},
		{"css effect", imageDoc(uri, `style="image-rendering:pixelated"`), "CSS property"},
		{"attribute effect", imageDoc(uri, `image-rendering="pixelated"`), "image attribute"},
		{"child", `<svg><image href="` + uri + `"><rect width="2" height="2"/></image></svg>`, "unsupported child"},
		{"unused external", `<svg><defs><image href="no.png"/></defs></svg>`, "data URI"},
		{"symbol clip", `<svg><defs><symbol id="s" viewBox="0 0 10 10" overflow="hidden"><image href="` + uri + `" width="20" height="20" preserveAspectRatio="none"/></symbol></defs><use href="#s" width="10" height="10"/></svg>`, "viewport clipping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svg.Read(strings.NewReader(tc.source), svg.DefaultOptions())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v; want %q", err, tc.want)
			}
		})
	}
	// Partial first GIF frame cannot be substituted for the logical canvas.
	frame := image.NewPaletted(image.Rect(1, 0, 3, 2), color.Palette{color.Black, color.White})
	var data bytes.Buffer
	if err := gif.EncodeAll(&data, &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{0}, Config: image.Config{ColorModel: frame.Palette, Width: 4, Height: 2}}); err != nil {
		t.Fatal(err)
	}
	partial := "data:image/gif;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
	if _, err := svg.Read(strings.NewReader(imageDoc(partial, ``)), svg.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "full logical canvas") {
		t.Fatalf("partial GIF: %v", err)
	}
}
func TestEmbeddedImageRasterReference(t *testing.T) {
	source, err := os.ReadFile("../tests/data/svg/embedded-images.svg")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../tests/data/svg/embedded-images.png")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	actual := renderInstances(t, string(source), 160, 120)
	total, maxDiff, pixels := 0.0, 0, 0
	for y := range 120 {
		for x := range 160 {
			a, b := actual.RGBAAt(x, y), color.RGBAModel.Convert(ref.At(x, y)).(color.RGBA)
			mismatch := false
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				delta := int(math.Abs(float64(d)))
				total += float64(delta)
				maxDiff = max(maxDiff, delta)
				mismatch = mismatch || delta > 8
			}
			if mismatch {
				pixels++
			}
		}
	}
	mean, ratio := total/(160*120*4), float64(pixels)/(160*120)
	t.Logf("CairoSVG embedded images 160x120 RGBA: MAE=%g max=%d fraction_over_8=%g", mean, maxDiff, ratio)
	if mean > 3 || maxDiff > 170 || ratio > .1 {
		t.Fatalf("image reference differs: MAE=%g max=%d fraction=%g", mean, maxDiff, ratio)
	}
}
func BenchmarkReadEmbeddedImageInstances(b *testing.B) {
	uri := imageURI(b, "embedded.png", "png")
	var source strings.Builder
	source.WriteString(`<svg viewBox="0 0 256 256"><defs><image id="tile" href="` + uri + `" width="16" height="8"/></defs>`)
	for i := range 256 {
		fmt.Fprintf(&source, `<use href="#tile" x="%d" y="%d"/>`, (i%16)*16, (i/16)*16)
	}
	source.WriteString(`</svg>`)
	doc := source.String()
	b.ReportAllocs()
	b.SetBytes(int64(len(doc)))
	b.ResetTimer()
	for range b.N {
		if _, err := svg.Read(strings.NewReader(doc), svg.DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEmbeddedImageSymbolAndAspectAlignments(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	// An image inside a clipped symbol fits exactly. The symbol establishes the
	// percentage viewport; use sizes change the symbol viewport, not image attrs.
	source := `<svg viewBox="0 0 100 100"><defs><symbol id="s" viewBox="0 0 20 10" overflow="hidden"><defs><image href="` + uri + `" width="90" height="90"/></defs><image href="` + uri + `" x="10%" y="10%" width="80%" height="80%" preserveAspectRatio="none"/></symbol></defs><use href="#s" x="10" y="20" width="40" height="20"/></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 1 {
		t.Fatalf("nested defs painted %d images", len(shapes))
	}
	assertInstanceBox(t, shapes[0], [4]float64{14, 22, 32, 16})
	direct := readInstances(t, `<svg viewBox="0 0 100 100"><defs><image id="a" href="`+uri+`" width="16" height="8"/></defs><use href="#a" width="90" height="90"/></svg>`)
	assertInstanceBox(t, instanceShapes(direct)[0], [4]float64{0, 0, 16, 8})
	for xi, x := range []string{"Min", "Mid", "Max"} {
		for yi, y := range []string{"Min", "Mid", "Max"} {
			aspect := "x" + x + "Y" + y + " meet"
			tall := readInstances(t, imageDoc(uri, `width="40" height="40" preserveAspectRatio="`+aspect+`"`))
			assertInstanceBox(t, instanceShapes(tall)[0], [4]float64{0, float64(yi) * 10, 40, 20})
			wide := readInstances(t, imageDoc(uri, `width="80" height="20" preserveAspectRatio="`+aspect+`"`))
			assertInstanceBox(t, instanceShapes(wide)[0], [4]float64{float64(xi) * 20, 0, 40, 20})
		}
	}
	// Sample reflected+rotated pixels, independently deriving this integer map:
	// SVG mapping (u,v)->(70-v,70-u), so TL red lands near (68,68).
	transformed := renderInstances(t, imageDoc(uri, `width="16" height="8" transform="translate(70 70) rotate(90) scale(-1 1)"`), 100, 100)
	if got := transformed.RGBAAt(68, 68); got != (color.RGBA{240, 24, 16, 255}) {
		t.Fatalf("reflected rotation pixel %v", got)
	}
}
