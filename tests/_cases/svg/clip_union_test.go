package svg_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"gomath/svg"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const unionRect = `<rect x="10" y="10" width="40" height="40"/>`

func unionDoc(children string) string {
	return `<svg viewBox="0 0 100 100"><defs><clipPath id="c">` + children + `</clipPath></defs><rect id="body" width="100" height="100" fill="#20a0e0" clip-path="url(#c)"/></svg>`
}
func unionArea(p path.Path) float64 {
	var area float64
	for _, c := range p.Contours {
		prev := c.Start
		for _, v := range c.Curves {
			area += prev.X*v.P3.Y - prev.Y*v.P3.X
			prev = v.P3
		}
		area += prev.X*c.Start.Y - prev.Y*c.Start.X
	}
	return math.Abs(area) / 2
}
func TestSVGClipUnionRegionsAndIndependentRules(t *testing.T) {
	for _, tc := range []struct {
		name, children string
		area           float64
		probes         [][3]int
	}{
		{"opposite-overlap", unionRect + `<path d="M30 30V70H70V30Z"/>`, 2800, [][3]int{{40, 40, 1}, {20, 60, 0}}},
		{"identical-opposite", unionRect + `<path d="M10 10V50H50V10Z"/>`, 1600, [][3]int{{30, 30, 1}}},
		{"evenodd-duplicates-across-children", strings.ReplaceAll(unionRect+unionRect, "/>", ` clip-rule="evenodd"/>`), 1600, [][3]int{{30, 30, 1}}},
		{"evenodd-duplicates-within-child", `<path clip-rule="evenodd" d="M10 10H50V50H10Z M10 10H50V50H10Z"/><rect x="60" y="10" width="20" height="20"/>`, 400, [][3]int{{30, 30, 0}, {70, 20, 1}}},
		{"mixed-holes", `<path clip-rule="evenodd" d="M10 10H90V90H10Z M30 30H70V70H30Z"/><rect x="50" y="30" width="40" height="40"/>`, 5600, [][3]int{{40, 50, 0}, {60, 50, 1}, {20, 50, 1}}},
		{"nonzero-hole", `<path d="M10 10H90V90H10Z M30 30V70H70V30Z"/><rect x="50" y="30" width="40" height="40"/>`, 5600, [][3]int{{40, 50, 0}, {60, 50, 1}}},
		{"winding-cancel-within", `<path d="M10 10H50V50H10Z M10 10V50H50V10Z"/><rect x="60" y="10" width="20" height="20"/>`, 400, [][3]int{{30, 30, 0}, {70, 20, 1}}},
		{"edge-and-point-contacts", `<rect x="10" y="10" width="20" height="20"/><rect x="30" y="10" width="20" height="20"/><rect x="50" y="30" width="20" height="20"/>`, 1200, [][3]int{{30, 20, 1}, {60, 40, 1}, {40, 40, 0}}},
		{"partial-collinear", `<rect x="10" y="10" width="40" height="40"/><rect x="30" y="10" width="40" height="20"/>`, 2000, [][3]int{{60, 20, 1}, {60, 40, 0}}},
		{"scientific-relative", `<path d="m1e1 1e1h40v40h-40z"/><polygon points="30,30 30,70 70,70 70,30"/>`, 2800, [][3]int{{40, 40, 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := unionDoc(tc.children)
			root := readInstances(t, source)
			o := cssObject(t, root, "body")
			if o.Clip == nil || o.Clip.FillRule() != path.NonZero || math.Abs(unionArea(o.Clip.Path())-tc.area) > 1e-8 {
				t.Fatalf("wrong canonical union area/rule: %v", o.Clip)
			}
			count := 0
			root.Walk(func(o *gomath.Object) {
				if o.Clip != nil {
					count++
				}
			})
			if count != 1 {
				t.Fatal("union must materialize one Clip", count)
			}
			im := renderInstances(t, source, 100, 100)
			for _, q := range tc.probes {
				if (im.RGBAAt(q[0], q[1]).A != 0) != (q[2] != 0) {
					t.Fatal("wrong union membership", q, im.RGBAAt(q[0], q[1]))
				}
			}
		})
	}
}
func TestSVGClipUnionCSSUseTransformsAndOwnership(t *testing.T) {
	const source = `<svg viewBox="0 0 100 100" xmlns:xlink="http://www.w3.org/1999/xlink"><style>
 .defs > clipPath {clip-rule:evenodd} clipPath > .solid{clip-rule:nonzero} #s{clip-rule:inherit} .host path{clip-rule:nonzero}
 </style><defs class="defs" transform="translate(900 900)"><path id="s" d="M0 0H20V20H0Z M5 5H15V15H5Z"/>
 <clipPath id="c" transform="translate(2 3)"><use xlink:href="#s" x="10" y="20" transform="matrix(1 .25 .5 1 0 0)"/><use href="#s" x="50" y="20" class="solid"/></clipPath>
 <rect id="tile" width="90" height="80" clip-path="url(#c)"/></defs><g class="host"><use href="#tile"/><use href="#tile" x="1"/></g></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 2 {
		t.Fatal(len(shapes))
	}
	// First source's determinant is .875, area=(400-100)*.875; second is solid.
	if math.Abs(unionArea(shapes[0].Clip.Path())-662.5) > 1e-8 {
		t.Fatal("use rule/source ancestry leaked", unionArea(shapes[0].Clip.Path()))
	}
	before := shapes[1].Clip.Path()
	saved := root.Copy()
	returned := shapes[0].Clip.Path()
	returned.Contours[0].Curves[0].P0 = geom.V(999, 999)
	shapes[0].Shift(geom.V(7, 9))
	if !reflect.DeepEqual(before, shapes[1].Clip.Path()) || !reflect.DeepEqual(before, instanceShapes(saved)[1].Clip.Path()) {
		t.Fatal("union clips alias")
	}
	ids := map[uint64]bool{}
	root.Walk(func(o *gomath.Object) {
		if ids[o.ID] {
			t.Fatal("instance IDs reused")
		}
		ids[o.ID] = true
	})
}
func TestSVGClipUnionVisibilityAndPaintIndependence(t *testing.T) {
	source := unionDoc(`<rect x="10" y="10" width="20" height="20" fill="none" stroke="red" stroke-width="90" opacity="0"/><rect x="40" y="10" width="20" height="20" display="none"/><rect x="70" y="10" width="20" height="20" visibility="hidden"/>`)
	im := renderInstances(t, source, 100, 100)
	for _, q := range [][3]int{{20, 20, 1}, {50, 20, 0}, {80, 20, 0}, {5, 20, 0}} {
		if (im.RGBAAt(q[0], q[1]).A != 0) != (q[2] != 0) {
			t.Fatal(q)
		}
	}
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><rect id="s" width="20" height="20" visibility="visible"/><clipPath id="c"><use href="#s" visibility="hidden"/><rect x="30" width="10" height="10"/></clipPath></defs><rect id="body" width="100" height="100" clip-path="url(#c)"/></svg>`)
	if unionArea(cssObject(t, root, "body").Clip.Path()) != 500 {
		t.Fatal("target visibility must override inherited use visibility")
	}
}
func TestSVGClipUnionBBoxAndAffine(t *testing.T) {
	for _, matrix := range []string{"rotate(31 20 30)", "matrix(1 .2 .7 1 3 4)", "matrix(-2 .1 .2 1.5 90 2)", "scale(2 .5)"} {
		a := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c" clipPathUnits="objectBoundingBox" transform="translate(.1 .2)"><rect width=".5" height=".5"/><rect x=".25" y=".25" width=".5" height=".5"/></clipPath></defs><rect id="r" x="10" y="20" width="40" height="30" transform="`+matrix+`" clip-path="url(#c)"/></svg>`)
		b := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c" transform="translate(14 26) scale(40 30)"><rect width=".5" height=".5"/><rect x=".25" y=".25" width=".5" height=".5"/></clipPath></defs><rect id="r" x="10" y="20" width="40" height="30" transform="`+matrix+`" clip-path="url(#c)"/></svg>`)
		compareUnionVertices(t, cssObject(t, a, "r").Clip.Path(), cssObject(t, b, "r").Clip.Path())
	}
	for _, target := range []string{`<line x1="10" x2="80" y1="20" y2="20" stroke="red" stroke-width="10"/>`, `<rect width="0" height="20"/>`} {
		def := `<defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/><rect x=".5" width=".5" height="1"/></clipPath></defs>`
		got := renderInstances(t, `<svg viewBox="0 0 100 100">`+def+strings.Replace(target, "/>", ` clip-path="url(#c)"/>`, 1)+`</svg>`, 100, 100)
		want := renderInstances(t, `<svg viewBox="0 0 100 100">`+target+`</svg>`, 100, 100)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatal("degenerate bbox must ignore union effect")
		}
	}
}
func TestSVGClipUnionRejectUnsupportedAtomically(t *testing.T) {
	for _, child := range []string{
		`<circle r="10"/>`, `<ellipse rx="10" ry="5"/>`, `<rect width="10" height="10" rx="1"/>`,
		`<path d="M0 0C1 0 2 0 3 0L4 4Z"/>`, `<path d="M0 0Q1 1 3 0L4 4Z"/>`, `<path d="M0 0A2 2 0 0 0 3 0L4 4Z"/>`,
		`<path d="M0 0L10 0L10 10"/>`, `<path d="M0 0Z"/>`, `<path/>`, `<path d="M0 0L0 0L1 1Z"/>`, `<path d="M0 0H1H2Z"/>`,
		`<polyline points="0,0 10,0 10,10"/>`, `<line x2="10" y2="10"/>`, `<polygon points="0,0 10,0"/>`, `<rect width="0" height="10"/>`,
		`<g><rect width="10" height="10"/></g>`, `<text>Go</text>`, `<image width="10" height="10" href="https://example.invalid/x.png"/>`,
		`<rect width="10" height="10" clip-path="url(#c)"/>`, `<use href="#absent"/>`, `<use href="https://example.invalid/a#s"/>`,
		`<rect width="10" height="10" bad="1"/>`, `<rect width="10" height="10" transform="scale(0)"/>`,
		`<circle r="10" display="none"/>`, `<rect width="10%" height="10"/>`,
	} {
		source := unionDoc(unionRect + child)
		o, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
		if err == nil || o != nil {
			t.Fatalf("unsupported yielded output: %s %v", child, err)
		}
	}
	for _, defs := range []string{
		`<rect id="r" width="10" height="10"/><use id="u" href="#r"/><clipPath id="c"><use href="#u"/>` + unionRect + `</clipPath>`,
		`<g id="r"><rect width="10" height="10"/></g><clipPath id="c"><use href="#r"/>` + unionRect + `</clipPath>`,
		`<rect id="r" width="10" height="10" clip-path="url(#other)"/><clipPath id="other"/><clipPath id="c"><use href="#r"/>` + unionRect + `</clipPath>`,
		`<clipPath id="c" clip-path="url(#other)">` + unionRect + unionRect + `</clipPath><clipPath id="other"/>`,
	} {
		o, err := svg.Read(strings.NewReader(`<svg><defs>`+defs+`</defs></svg>`), svg.DefaultOptions())
		if err == nil || o != nil {
			t.Fatal("unsupported use/nesting accepted", err)
		}
	}
	// Cached and uncached union plans must obey the same nesting boundary.
	union := `<clipPath id="u">` + unionRect + unionRect + `</clipPath>`
	single := `<clipPath id="s"><rect width="100" height="100" clip-path="url(#u)"/></clipPath>`
	for _, defs := range []string{union + single, single + union} {
		o, err := svg.Read(strings.NewReader(`<svg><defs>`+defs+`</defs></svg>`), svg.DefaultOptions())
		if o != nil || err == nil || !strings.Contains(err.Error(), "nested") {
			t.Fatal("cached nested union bypass", err)
		}
	}
	// Unitless-only bbox validation must run through direct use too.
	for _, units := range []string{"50%", "1px"} {
		source := `<svg><defs><rect id="r" width="` + units + `" height="1"/><clipPath id="c" clipPathUnits="objectBoundingBox"><use href="#r"/><rect width="1" height="1"/></clipPath></defs></svg>`
		o, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
		if o != nil || err == nil {
			t.Fatal("bbox use length accepted", units)
		}
	}
}
func TestSVGClipUnionLimits(t *testing.T) {
	for _, children := range []string{strings.Repeat(unionRect, 17), `<polygon points="` + strings.Repeat("1 2 ", 17000) + `"/>` + unionRect, `<path d="` + strings.Repeat("M0 0H1V1H0Z ", 129) + `"/>` + unionRect} {
		o, err := svg.Read(strings.NewReader(unionDoc(children)), svg.DefaultOptions())
		if o != nil || err == nil {
			t.Fatal("unbounded input accepted")
		}
	}
	// 11 definitions of two identical children reserve 33 calls, even unused.
	var defs strings.Builder
	for i := range 11 {
		fmt.Fprintf(&defs, `<clipPath id="c%d">%s%s</clipPath>`, i, unionRect, unionRect)
	}
	o, err := svg.Read(strings.NewReader(`<svg><defs>`+defs.String()+`</defs></svg>`), svg.DefaultOptions())
	if o != nil || err == nil || !strings.Contains(err.Error(), "reservation") {
		t.Fatal("per-definition Boolean budget reset", err)
	}
}
func TestSVGClipUnionConcurrentImports(t *testing.T) {
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			root := readInstances(t, unionDoc(unionRect+`<path d="M30 30V70H70V30Z"/>`))
			if unionArea(cssObject(t, root, "body").Clip.Path()) != 2800 {
				t.Fatal("concurrent plan corrupted")
			}
		})
	}
}

func TestSVGClipUnionIndependentCairo(t *testing.T) {
	entries, err := os.ReadDir("../tests/data/svg/clip-unions")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".svg") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".svg")
		count++
		for _, scale := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/ss%d", name, scale), func(t *testing.T) {
				prefix := "../tests/data/svg/clip-unions/" + name
				data, err := os.ReadFile(prefix + ".svg")
				if err != nil {
					t.Fatal(err)
				}
				root, err := svg.Read(bytes.NewReader(data), svg.Options{Width: 8})
				if err != nil {
					t.Fatal(err)
				}
				f, err := os.Open(fmt.Sprintf("%s-cairo-ss%d.png", prefix, scale))
				if err != nil {
					t.Fatal(err)
				}
				want, err := png.Decode(f)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
				got := renderBBoxFrame(t, root, scale, nil)
				var sum, squares float64
				maximum := 0
				for y := range 96 {
					for x := range 128 {
						a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
						for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
							sum += math.Abs(float64(d))
							squares += float64(d * d)
							maximum = max(maximum, int(math.Abs(float64(d))))
						}
					}
				}
				mae, rms := sum/(128*96*4), math.Sqrt(squares/(128*96*4))
				t.Logf("direct Cairo %s SS%d MAE=%.9g RMS=%.9g max=%d", name, scale, mae, rms, maximum)
				if mae > .4 || rms > 2 || maximum > 40 {
					t.Fatal("independent clip union reference mismatch")
				}
				saved := root.Copy()
				root.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(.3)))
				cam := gomath.Camera2D{Center: geom.V(10, 20), Rotation: .3, FrameWidth: 8}
				moved := renderBBoxFrame(t, root, scale, &cam)
				for i, v := range got.Pix {
					d := int(v) - int(moved.Pix[i])
					if d < -1 || d > 1 {
						t.Fatalf("camera lost union at %d: %d", i, d)
					}
				}
				c := render.DefaultConfig()
				c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 128, 96, 1, 8, gomath.Transparent
				target := filepath.Join(t.TempDir(), "union.svg")
				if err := export.SVG(target, &gomath.Frame{Objects: []*gomath.Object{saved}}, c); err != nil {
					t.Fatal(err)
				}
				encoded, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				var document struct {
					Defs []struct {
						Clips []struct {
							Units string `xml:"clipPathUnits,attr"`
							Paths []struct {
								D    string `xml:"d,attr"`
								Rule string `xml:"clip-rule,attr"`
							} `xml:"path"`
						} `xml:"clipPath"`
					} `xml:"defs"`
				}
				if err := xml.Unmarshal(encoded, &document); err != nil {
					t.Fatal(err)
				}
				var clips []*gomath.Clip
				saved.Walk(func(o *gomath.Object) {
					if o.Clip != nil {
						clips = append(clips, o.Clip)
					}
				})
				index := 0
				for _, defs := range document.Defs {
					for _, clip := range defs.Clips {
						if index >= len(clips) || clip.Units != "userSpaceOnUse" || len(clip.Paths) != 1 || clip.Paths[0].Rule != "nonzero" {
							t.Fatal("bad materialized export")
						}
						outline, err := svg.ParsePath(clip.Paths[0].D)
						if err != nil {
							t.Fatal(err)
						}
						if math.Abs(unionArea(outline)-unionArea(clips[index].Path())) > 1e-7 {
							t.Fatal("export lost union geometry")
						}
						index++
					}
				}
				if index != len(clips) {
					t.Fatal("export omitted clips")
				}
			})
		}
	}
	if count < 8 {
		t.Fatal("missing independent cases", count)
	}
}
func TestSVGClipUnionOracleGeometry(t *testing.T) {
	data, err := os.ReadFile("../tests/data/svg/clip-unions/geometry.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name, Source string
		Area         float64
		Samples      [][3]float64
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			root := readInstances(t, tc.Source)
			o := cssObject(t, root, "body")
			if math.Abs(unionArea(o.Clip.Path())-tc.Area) > 1e-7 {
				t.Fatal("Skia geometry area mismatch", unionArea(o.Clip.Path()), tc.Area)
			}
			im := renderInstances(t, tc.Source, 100, 100)
			for _, p := range tc.Samples {
				x, y := int(p[0]), int(p[1])
				if (im.RGBAAt(x, y).A > 127) != (p[2] != 0) {
					t.Fatal("Skia membership mismatch", p)
				}
			}
		})
	}
}
func TestSVGClipUnionIntermediateMorph(t *testing.T) {
	from := cssObject(t, readInstances(t, unionDoc(unionRect+`<rect x="30" y="30" width="40" height="40"/>`)), "body")
	to := from.Copy()
	to.ApplyMatrix(geom.Translation(geom.V(12, -8)).Mul(geom.Scaling(1.25, .75, 1)))
	original := from.Copy()
	scene := gomath.NewScene()
	scene.Add(from)
	if err := scene.Play(1, gomath.Transform(from, to).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{0, .35, .7, 1} {
		frame, err := scene.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		o := frame.Find(from.ID)
		if o == nil || o.ID != from.ID || o.Clip.FillRule() != path.NonZero {
			t.Fatal("morph lost union identity")
		}
		expected := original.Copy()
		expected.ApplyMatrix(geom.Translation(geom.V(12*time, -8*time)).Mul(geom.Scaling(1+.25*time, 1-.25*time, 1)))
		compareUnionVertices(t, o.Clip.Path(), expected.Clip.Path())
		a, err := renderUnionAnimation(o)
		if err != nil {
			t.Fatal(err)
		}
		b, err := renderUnionAnimation(expected)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range a.Pix {
			if delta := int(v) - int(b.Pix[i]); delta < -1 || delta > 1 {
				t.Fatal("intermediate union pixels changed", time, i, delta)
			}
		}
	}
}
func renderUnionAnimation(o *gomath.Object) (*image.RGBA, error) {
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth = 128, 96, 1, 100
	r, err := render.NewRasterizer(c)
	if err != nil {
		return nil, err
	}
	im, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if err != nil {
		return nil, err
	}
	return im, nil
}
func BenchmarkSVGClipUnionImportRender(b *testing.B) {
	data, err := os.ReadFile("../tests/data/svg/clip-unions/mixed-holes.svg")
	if err != nil {
		b.Fatal(err)
	}
	for _, scale := range []int{1, 4} {
		b.Run(fmt.Sprintf("2_children_128x96_SS%d", scale), func(b *testing.B) {
			c := render.DefaultConfig()
			c.Width, c.Height, c.Workers, c.FrameWidth, c.Supersampling = 128, 96, 1, 8, scale
			r, err := render.NewRasterizer(c)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for range b.N {
				root, err := svg.Read(bytes.NewReader(data), svg.Options{Width: 8})
				if err != nil {
					b.Fatal(err)
				}
				if _, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{root}}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Canonical Boolean output may choose a different cycle start after anisotropic
// transforms. Compare endpoint geometry without imposing a private start order.
func compareUnionVertices(t *testing.T, a, b path.Path) {
	t.Helper()
	var aa, bb []geom.Vec3
	for _, c := range a.Contours {
		for _, v := range c.Curves {
			aa = append(aa, v.P0)
		}
	}
	for _, c := range b.Contours {
		for _, v := range c.Curves {
			bb = append(bb, v.P0)
		}
	}
	if len(aa) != len(bb) || math.Abs(unionArea(a)-unionArea(b)) > 1e-7 {
		t.Fatal("union geometry differs")
	}
	used := make([]bool, len(bb))
	for _, v := range aa {
		found := false
		for j, w := range bb {
			if !used[j] && v.Sub(w).Norm() < 1e-8 {
				used[j] = true
				found = true
				break
			}
		}
		if !found {
			t.Fatal("missing union endpoint", v, bb)
		}
	}
}
func TestSVGClipUnionMaximumChildrenAndNumericFailures(t *testing.T) {
	root := readInstances(t, unionDoc(strings.Repeat(unionRect, 16)))
	if unionArea(cssObject(t, root, "body").Clip.Path()) != 1600 {
		t.Fatal("maximum supported child count")
	}
	for _, child := range []string{`<path d="M0 0H1e-320V1H0Z"/>`, `<rect width="10" height="10" transform="scale(1e308)"/>`} {
		o, err := svg.Read(strings.NewReader(unionDoc(unionRect+child)), svg.DefaultOptions())
		if o != nil || err == nil {
			t.Fatal("numerically unsupported input yielded output", err)
		}
	}
}

func TestSVGClipUnionIndependentIntermediateAnimation(t *testing.T) {
	source, err := os.ReadFile("../tests/data/svg/clip-unions/opposite-overlap.svg")
	if err != nil {
		t.Fatal(err)
	}
	from, err := svg.Read(bytes.NewReader(source), svg.Options{Width: 8})
	if err != nil {
		t.Fatal(err)
	}
	to := from.Copy()
	to.ApplyMatrix(geom.Translation(geom.V(.5, -.3125)).Mul(geom.Scaling(.8, .9, 1)))
	scene := gomath.NewScene()
	scene.Add(from)
	if err := scene.Play(1, gomath.Transform(from, to).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		time  float64
		label string
	}{{.35, "035"}, {.7, "070"}} {
		frame, err := scene.Sample(tc.time)
		if err != nil {
			t.Fatal(err)
		}
		c := render.DefaultConfig()
		c.Width, c.Height, c.Workers, c.FrameWidth, c.Supersampling, c.Background = 128, 96, 1, 8, 4, gomath.Transparent
		raster, err := render.NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		got, err := raster.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open("../tests/data/svg/clip-unions/morph-" + tc.label + "-cairo-ss4.png")
		if err != nil {
			t.Fatal(err)
		}
		want, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		var sum, squares float64
		maximum := 0
		for y := range 96 {
			for x := range 128 {
				a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
				for _, delta := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
					sum += math.Abs(float64(delta))
					squares += float64(delta * delta)
					maximum = max(maximum, int(math.Abs(float64(delta))))
				}
			}
		}
		mae, rms := sum/(128*96*4), math.Sqrt(squares/(128*96*4))
		t.Logf("Cairo intermediate t=%g MAE=%g RMS=%g max=%d", tc.time, mae, rms, maximum)
		if mae > .15 || rms > 1 || maximum > 24 {
			t.Fatal("independent intermediate clip union mismatch")
		}
	}
}

func TestSVGClipUnionSourceCacheModesAndRules(t *testing.T) {
	source := `<path id="s" d="M0 0H20V20H0Z M5 5H15V15H5Z" clip-rule="inherit"/>`
	single := `<clipPath id="one"><use href="#s" clip-rule="nonzero"/></clipPath>`
	multi := `<clipPath id="many"><use href="#s" clip-rule="evenodd"/><rect x="30" width="10" height="10"/></clipPath>`
	bbox := `<clipPath id="box" clipPathUnits="objectBoundingBox"><rect width=".5" height=".5"/><rect x=".25" y=".25" width=".5" height=".5"/></clipPath>`
	body := `<rect id="oneBody" width="100" height="100" clip-path="url(#one)"/><rect id="manyBody" width="100" height="100" clip-path="url(#many)"/><rect id="boxBody" width="40" height="20" clip-path="url(#box)"/>`
	for _, defs := range []string{source + single + multi + bbox, source + bbox + multi + single} {
		root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>`+defs+`</defs>`+body+`</svg>`)
		for _, tc := range []struct {
			name string
			area float64
		}{{"oneBody", 500}, {"manyBody", 400}, {"boxBody", 350}} {
			o := cssObject(t, root, tc.name)
			// Old single has same-oriented outer/inner contours; shoelace sums both,
			// while its nonzero silhouette still fills the entire outer rectangle.
			if unionArea(o.Clip.Path()) != tc.area {
				t.Fatal("source plan/rule/bbox cache leak", tc, unionArea(o.Clip.Path()))
			}
		}
	}
}
