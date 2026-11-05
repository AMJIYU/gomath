package svg_test

import (
	"bytes"
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

const bboxHalf = `<defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath></defs>`

func TestSVGBBoxClipUsesLocalGeometryBeforeAffine(t *testing.T) {
	for _, matrix := range []string{"rotate(31 20 30)", "matrix(1 .2 .7 1 3 4)", "matrix(-2 .1 .2 1.5 90 2)", "scale(2 .5)"} {
		t.Run(matrix, func(t *testing.T) {
			// Compare with independently explicit user-space clip coordinates. Neither
			// outline is measured after the target transform, so world AABB cannot pass.
			body := `<rect id="body" x="10" y="20" width="40" height="30" transform="` + matrix + `" clip-path="url(#c)"/>`
			a := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+body+`</svg>`)
			b := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c"><rect x="10" y="20" width="20" height="30"/></clipPath></defs>`+body+`</svg>`)
			pa, pb := cssObject(t, a, "body").Clip.Path(), cssObject(t, b, "body").Clip.Path()
			compareBBoxPaths(t, pa, pb)
		})
	}
	// A thick stroke must never enlarge the coordinate system. A cubic's local
	// maximum is 75, despite its control points reaching 100.
	root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`
 <rect id="stroke" x="10" y="20" width="40" height="30" stroke="red" stroke-width="40" clip-path="url(#c)"/>
 <path id="curve" d="M0 0 C0 100 100 100 100 0Z" clip-path="url(#c)"/></svg>`)
	clipBox(t, cssObject(t, root, "stroke"), [4]float64{10, 20, 20, 30})
	clipBox(t, cssObject(t, root, "curve"), [4]float64{0, 0, 50, 75})
}

func TestSVGBBoxClipCubicExtremaAreScaleIndependent(t *testing.T) {
	for _, size := range []float64{1e-100, 1e-15, 1, 1e15, 1e100} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			outline := fmt.Sprintf("M0 0 C0 %.17g %.17g %.17g %.17g 0Z", size, size, size, size)
			factor := 10 / size
			for _, body := range []string{
				fmt.Sprintf(`<path id="r" d="%s" transform="scale(%.17g)" clip-path="url(#c)"/>`, outline, factor),
				fmt.Sprintf(`<defs><path id="s" d="%s"/></defs><use id="r" href="#s" transform="scale(%.17g)" clip-path="url(#c)"/>`, outline, factor),
			} {
				root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+body+`</svg>`)
				clipBox(t, cssObject(t, root, "r"), [4]float64{0, 0, 5, 7.5})
			}
			// The same numerical issue can occur in a clipping graphic that itself
			// references a bbox clip, before its clipPath transform magnifies it.
			source := fmt.Sprintf(`<svg viewBox="0 0 100 100">%s<defs><clipPath id="outer" transform="scale(%.17g)"><path d="%s" clip-path="url(#c)"/></clipPath></defs><rect id="r" width="10" height="10" clip-path="url(#outer)"/></svg>`, bboxHalf, factor, outline)
			root := readInstances(t, source)
			// Intersection wrapper holds the mapped half rectangle, not a degenerate
			// empty effect; the underlying path silhouette remains independently owned.
			var half *gomath.Object
			root.Walk(func(o *gomath.Object) {
				if o.Clip != nil && len(o.Clip.Path().Contours[0].Curves) == 4 {
					half = o
				}
			})
			if half == nil {
				t.Fatal("nested bbox extrema effect disappeared")
			}
			clipBox(t, half, [4]float64{0, 0, 5, 7.5})
		})
	}
}

func compareBBoxPaths(t *testing.T, a, b path.Path) {
	t.Helper()
	if len(a.Contours) != len(b.Contours) {
		t.Fatal("contour count", a, b)
	}
	for i, c := range a.Contours {
		d := b.Contours[i]
		if len(c.Curves) != len(d.Curves) {
			t.Fatal("curve count")
		}
		for j, v := range c.Curves {
			w := d.Curves[j]
			for k, p := range []geom.Vec3{v.P0, v.P1, v.P2, v.P3} {
				q := []geom.Vec3{w.P0, w.P1, w.P2, w.P3}[k]
				if p.Sub(q).Norm() > 1e-9 {
					t.Fatalf("clip coordinate differs %v != %v", p, q)
				}
			}
		}
	}
}

func TestSVGBBoxClipBasicShapesAndDecimalRange(t *testing.T) {
	for _, shape := range []string{
		`<rect x="10" y="20" width="40" height="60"/>`,
		`<circle cx="30" cy="40" r="20"/>`, `<ellipse cx="30" cy="40" rx="20" ry="30"/>`,
		`<line x1="10" y1="20" x2="50" y2="80"/>`,
		`<polygon points="10,20 50,20 50,80 10,80"/>`, `<polyline points="10,20 50,20 50,80"/>`,
		`<path d="M10 20H50V80H10Z"/>`,
	} {
		root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect x="-.25" y=".1" width="1.5" height=".8"/></clipPath></defs>`+strings.Replace(shape, "/>", ` id="r" clip-path="url(#c)"/>`, 1)+`</svg>`)
		o := cssObject(t, root, "r")
		box := o.Path.Bounds()
		b := o.Clip.Bounds()
		if math.Abs(b.Min.X-(box.Min.X-.25*box.Size().X)) > 1e-8 || math.Abs(b.Size().X-1.5*box.Size().X) > 1e-8 || math.Abs(b.Size().Y-.8*box.Size().Y) > 1e-8 {
			t.Fatal("wrong decimal geometry mapping", shape, box, b)
		}
	}
}

func TestSVGBBoxClipDefinitionTransformChildUseAndCascade(t *testing.T) {
	const outline = `M0 0H1V1H0Z M.25 .25H.75V.75H.25Z`
	source := `<svg viewBox="0 0 100 100"><style>.defs > clipPath > use{clip-rule:evenodd} #s{clip-rule:inherit} .tile{clip-path:url(#c)} .host path{clip-rule:nonzero}</style>
 <defs class="defs" transform="translate(900 900)"><path id="s" d="` + outline + `"/>
 <clipPath id="c" clipPathUnits="objectBoundingBox" transform="translate(.1 .2)"><use href="#s" x=".05" y=".1" transform="scale(.5)"/></clipPath>
 <clipPath id="user"><use href="#s" style="clip-rule:initial"/></clipPath><rect id="tile" class="tile" x="10" y="20" width="40" height="60"/>
 </defs><g class="host"><use href="#tile" x="5"/><use href="#tile" x="30"/></g>
 <rect id="direct" width="80" height="30" clip-path="url(#c)"/>
 <rect id="normal" width="100" height="100" clip-path="url(#user)"/></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 4 {
		t.Fatal("definitions painted", len(shapes))
	}
	clipBox(t, shapes[0], [4]float64{20, 35, 20, 30})
	clipBox(t, shapes[1], [4]float64{45, 35, 20, 30})
	clipBox(t, shapes[2], [4]float64{10, 7.5, 40, 15})
	for _, o := range shapes[:3] {
		if o.Clip.FillRule() != path.EvenOdd {
			t.Fatal("CSS/use clip rule leaked")
		}
	}
	if shapes[3].Clip.FillRule() != path.NonZero {
		t.Fatal("cache leaked bbox rule to user space")
	}
	before := shapes[1].Clip.Path()
	copyRoot := root.Copy()
	shapes[0].Shift(geom.V(7, 8))
	modified := shapes[0].Clip.Path()
	modified.Contours[0].Curves[0].P0 = geom.V(999, 999)
	if !reflect.DeepEqual(before, shapes[1].Clip.Path()) || !reflect.DeepEqual(before, instanceShapes(copyRoot)[1].Clip.Path()) {
		t.Fatal("bbox clips alias sibling/snapshot")
	}
}

func TestSVGBBoxClipUseHostMeasuresUnclippedSourceInHostSpace(t *testing.T) {
	source := `<svg viewBox="0 0 100 100">` + bboxHalf + `<defs>
 <clipPath id="tiny"><rect width="1" height="1"/></clipPath>
 <path id="curve" d="M0 0 C0 100 100 100 100 0Z" transform="matrix(1 0 .5 1 0 0)" clip-path="url(#tiny)"/>
 </defs><use id="host" href="#curve" x="4" y="5" transform="rotate(10)" clip-path="url(#c)"/></svg>`
	root := readInstances(t, source)
	// Measure the transformed actual cubic through an independent native path,
	// not the source control hull or an AABB of its untransformed bounds.
	// x(t)=4+150t+150t²-200t³ has its maximum at
	// t=(1+sqrt(5))/4; y(t)=5+300t(1-t) peaks at 80.
	tMax := (1 + math.Sqrt(5)) / 4
	xMax := 4 + 150*tMax + 150*tMax*tMax - 200*tMax*tMax*tMax
	box := geom.Bounds{Min: geom.V(4, 5), Max: geom.V(xMax, 80), Valid: true}
	explicit := fmt.Sprintf(`<svg viewBox="0 0 100 100"><defs><clipPath id="c"><rect x="%.17g" y="%.17g" width="%.17g" height="%.17g"/></clipPath></defs><rect id="r" width="100" height="100" transform="rotate(10)" clip-path="url(#c)"/></svg>`, box.Min.X, box.Min.Y, box.Size().X/2, box.Size().Y)
	compareBBoxPaths(t, cssObject(t, root, "host").Clip.Path(), cssObject(t, readInstances(t, explicit), "r").Clip.Path())
}

func TestSVGBBoxClipExplicitInheritReevaluatesUseAndTarget(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<defs><rect id="r" x="10" y="20" width="40" height="60" transform="translate(2 3)" clip-path="inherit"/></defs><use id="host" href="#r" x="5" y="7" clip-path="url(#c)" transform="scale(.8)"/></svg>`)
	clipBox(t, cssObject(t, root, "host"), [4]float64{13.6, 24, 16, 48})
	clipBox(t, cssObject(t, root, "r"), [4]float64{13.6, 24, 16, 48})
}

func TestSVGBBoxClipImageViewportBeforeMeetAndUse(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	for _, attrs := range []string{`x="10" y="20" width="60" height="40"`, `x="10%" y="20%" width="60%" height="40%"`} {
		source := `<svg viewBox="0 0 100 100">` + bboxHalf + `<defs><image id="source" href="` + uri + `" ` + attrs + ` preserveAspectRatio="xMaxYMid meet"/></defs>
 <image id="direct" href="` + uri + `" ` + attrs + ` preserveAspectRatio="xMaxYMid meet" clip-path="url(#c)"/>
 <use id="host" href="#source" x="5" y="7" clip-path="url(#c)"/></svg>`
		root := readInstances(t, source)
		clipBox(t, cssObject(t, root, "direct"), [4]float64{10, 20, 30, 40})
		clipBox(t, cssObject(t, root, "host"), [4]float64{15, 27, 30, 40})
	}
	// The missing/auto dimension uses the immutable asset's intrinsic aspect.
	root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<image id="auto" href="`+uri+`" width="60" clip-path="url(#c)"/></svg>`)
	o := cssObject(t, root, "auto")
	if math.Abs(o.Clip.Bounds().Size().X-30) > 1e-8 || math.Abs(o.Clip.Bounds().Size().Y-o.Path.Bounds().Size().Y) > 1e-8 {
		t.Fatal("auto dimensions not used")
	}
}

func TestSVGBBoxClipNestedReferencesAndDegenerateBBox(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><defs>
 <clipPath id="half" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath>
 <clipPath id="c" clipPathUnits="objectBoundingBox" transform="translate(.1 .1)"><rect x=".1" y=".2" width=".8" height=".6" transform="scale(.8)" clip-path="url(#half)"/></clipPath>
 </defs><rect width="100" height="100" fill="blue" clip-path="url(#c)"/></svg>`
	got := renderInstances(t, source, 100, 100)
	for _, p := range []struct {
		x, y    int
		visible bool
	}{{20, 30, true}, {45, 40, true}, {53, 40, false}, {20, 20, false}, {20, 80, false}} {
		if (got.RGBAAt(p.x, p.y).A != 0) != p.visible {
			t.Fatal("nested bbox intersection", p, got.RGBAAt(p.x, p.y))
		}
	}
	for _, geometry := range []string{`<line x1="10" y1="30" x2="90" y2="30" stroke="red" stroke-width="8"/>`, `<line x1="30" y1="10" x2="30" y2="90" stroke="red" stroke-width="8"/>`, `<path d="M10 10"/>`, `<path/>`, `<rect width="0" height="10"/>`} {
		source := `<svg viewBox="0 0 100 100"><defs><clipPath id="c" clipPathUnits="objectBoundingBox"/></defs>` + strings.Replace(geometry, "/>", ` id="r" clip-path="url(#c)"/>`, 1) + `</svg>`
		if cssObject(t, readInstances(t, source), "r").Clip != nil {
			t.Fatal("degenerate bbox must ignore the effect", geometry)
		}
		plain := renderInstances(t, `<svg viewBox="0 0 100 100">`+geometry+`</svg>`, 100, 100)
		if !bytes.Equal(plain.Pix, renderInstances(t, source, 100, 100).Pix) {
			t.Fatal("degenerate bbox changed pixels", geometry)
		}
	}
}

func TestSVGBBoxClipStrictAtomicBoundaries(t *testing.T) {
	for _, body := range []string{
		`<defs><symbol id="s" overflow="visible"><rect width="10" height="10"/></symbol></defs><use href="#s" clip-path="url(#c)"/>`,
		`<clipPath id="outer" clip-path="url(#c)"><rect width="10" height="10"/></clipPath>`,
		`<defs><rect id="r" width="10" height="10"/><clipPath id="outer"><use href="#r" clip-path="url(#c)"/></clipPath></defs>`,
	} {
		source := `<svg>` + bboxHalf + `<rect width="100" height="100"/>` + body + `</svg>`
		o, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
		if o != nil || err == nil || !strings.Contains(err.Error(), "objectBoundingBox") {
			t.Fatal("unsupported bbox semantics accepted or partial output", body, err)
		}
	}
	// Source-cache mode must not bypass validation when the same shape has
	// already been compiled under userSpaceOnUse.
	for _, v := range []string{"50%", "1px", "1in", "auto", "NaN", "Inf"} {
		source := `<svg><defs><rect id="r" width="` + v + `" height="1"/><clipPath id="u"><use href="#r"/></clipPath><clipPath id="c" clipPathUnits="objectBoundingBox"><use href="#r"/></clipPath></defs></svg>`
		o, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
		if o != nil || err == nil {
			t.Fatal("unsupported normalized length accepted", v, err)
		}
	}
	for _, source := range []string{
		`<svg><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width="1" height="1" clip-path="url(#c)"/></clipPath></svg>`,
		`<svg><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width="1" height="1"/></clipPath><path clip-path="url(#missing)"/></svg>`,
		`<svg><clipPath id="c" clipPathUnits="objectBoundingBox" transform="scale(1e308)"><rect width="1" height="1"/></clipPath><rect width="1e308" height="1" clip-path="url(#c)"/></svg>`,
	} {
		o, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
		if o != nil || err == nil {
			t.Fatal("cycle/reference/overflow yielded partial output", err)
		}
	}
}

func renderBBoxFrame(t *testing.T, root *gomath.Object, scale int, camera *gomath.Camera2D) *image.RGBA {
	t.Helper()
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background, c.Supersampling = 128, 96, 1, 8, gomath.Transparent, scale
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	im, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{root}, Camera: camera})
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func TestSVGBBoxClipIndependentReferencesCameraMorphExport(t *testing.T) {
	for _, name := range []string{"affine", "use", "nested", "gradient", "bitmap-meet", "evenodd", "degenerate"} {
		for _, scale := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/ss%d", name, scale), func(t *testing.T) {
				prefix := "../tests/data/svg/bbox-clips/" + name
				source, err := os.ReadFile(prefix + ".svg")
				if err != nil {
					t.Fatal(err)
				}
				root, err := svg.Read(bytes.NewReader(source), svg.Options{Width: 8})
				if err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(fmt.Sprintf("%s-cairo-ss%d.png", prefix, scale))
				if err != nil {
					t.Fatal(err)
				}
				want, err := png.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatal(err)
				}
				got := renderBBoxFrame(t, root, scale, nil)
				var total, maximum, over int
				for y := range 96 {
					for x := range 128 {
						a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
						pixelMax := 0
						for _, v := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
							d := int(math.Abs(float64(v)))
							total += d
							maximum = max(maximum, d)
							pixelMax = max(pixelMax, d)
						}
						if pixelMax > 8 {
							over++
						}
					}
				}
				mean, ratio := float64(total)/(128*96*4), float64(over)/(128*96)
				t.Logf("direct Cairo bbox %s SS%d MAE=%.9g max=%d pixels>8=%.9g", name, scale, mean, maximum, ratio)
				if mean > .35 || maximum > 45 || ratio > .02 {
					t.Fatal("independent bbox clipping reference mismatch")
				}
				saved := root.Copy()
				root.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(.3)))
				cam := gomath.Camera2D{Center: geom.V(10, 20), Rotation: .3, FrameWidth: 8}
				moved := renderBBoxFrame(t, root, scale, &cam)
				for i, v := range got.Pix {
					if d := int(v) - int(moved.Pix[i]); d < -1 || d > 1 {
						t.Fatalf("camera lost instantiated bbox clips at %d, delta %d", i, d)
					}
				}
				// Export uses already materialized native user-space clips. Check the
				// serialized outline and rule without expanding importer scope.
				c := render.DefaultConfig()
				c.Width, c.Height, c.Workers, c.FrameWidth = 128, 96, 1, 8
				c.Background = gomath.Transparent
				target := filepath.Join(t.TempDir(), "bbox.svg")
				if err := export.SVG(target, &gomath.Frame{Objects: []*gomath.Object{saved}}, c); err != nil {
					t.Fatal(err)
				}
				exported, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				// Verify actual exported definitions rather than assuming this
				// bounded importer accepts every exporter decoration/background.
				var document struct {
					Defs []struct {
						Clips []struct {
							Units string `xml:"clipPathUnits,attr"`
							Path  struct {
								D    string `xml:"d,attr"`
								Rule string `xml:"clip-rule,attr"`
							} `xml:"path"`
						} `xml:"clipPath"`
					} `xml:"defs"`
				}
				if err := xml.Unmarshal(exported, &document); err != nil {
					t.Fatal(err)
				}
				var original []*gomath.Clip
				saved.Walk(func(o *gomath.Object) {
					if o.Clip != nil {
						original = append(original, o.Clip)
					}
				})
				index := 0
				for _, defs := range document.Defs {
					for _, c := range defs.Clips {
						if index >= len(original) || c.Units != "userSpaceOnUse" {
							t.Fatal("wrong exported materialized clip")
						}
						wantRule := "nonzero"
						if original[index].FillRule() == path.EvenOdd {
							wantRule = "evenodd"
						}
						if c.Path.Rule != wantRule {
							t.Fatal("export lost clip rule")
						}
						outline, err := svg.ParsePath(c.Path.D)
						if err != nil {
							t.Fatal(err)
						}
						a, b := outline.Bounds(), original[index].Bounds()
						if a.Valid != b.Valid || a.Min.Sub(b.Min).Norm() > 1e-7 || a.Max.Sub(b.Max).Norm() > 1e-7 {
							t.Fatal("export changed materialized bbox outline", a, b)
						}
						index++
					}
				}
				if index != len(original) {
					t.Fatal("export dropped clip definitions")
				}

			})
		}
	}
	// Immutable native clip interpolation uses its ordinary rule-preserving
	// authoring pipeline; bbox is materialized at import, never recomputed from
	// progressively clipped bounds.
	a := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<rect id="r" width="20" height="40" clip-path="url(#c)"/></svg>`)
	b := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<rect id="r" x="20" width="60" height="40" clip-path="url(#c)"/></svg>`)
	from, to := cssObject(t, a, "r"), cssObject(t, b, "r")
	scene := gomath.NewScene()
	scene.Add(from)
	if err := scene.Play(1, gomath.Transform(from, to)); err != nil {
		t.Fatal(err)
	}
	frame, err := scene.Sample(.5)
	if err != nil {
		t.Fatal(err)
	}
	clipBox(t, frame.Find(from.ID), [4]float64{10, 0, 20, 40})
}

func TestSVGBBoxClipConcurrentImports(t *testing.T) {
	const source = `<svg viewBox="0 0 100 100">` + bboxHalf + `<rect width="30" height="50" clip-path="url(#c)"/></svg>`
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			o := readInstances(t, source)
			clipBox(t, instanceShapes(o)[0], [4]float64{0, 0, 15, 50})
		})
	}
}

func BenchmarkSVGBBoxClipImportRender(b *testing.B) {
	var source strings.Builder
	source.WriteString(`<svg viewBox="0 0 128 96">` + bboxHalf + `<defs><rect id="r" width="12" height="10" fill="#3090e0"/></defs>`)
	for i := range 64 {
		fmt.Fprintf(&source, `<use href="#r" x="%d" y="%d" clip-path="url(#c)"/>`, i%8*16, i/8*12)
	}
	source.WriteString(`</svg>`)
	input := source.String()
	for _, scale := range []int{1, 4} {
		b.Run(fmt.Sprintf("64_instances_128x96_SS%d", scale), func(b *testing.B) {
			c := render.DefaultConfig()
			c.Width, c.Height, c.Workers, c.FrameWidth, c.Supersampling = 128, 96, 1, 8, scale
			raster, err := render.NewRasterizer(c)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(input)))
			b.ResetTimer()
			for range b.N {
				root, err := svg.Read(strings.NewReader(input), svg.Options{Width: 8})
				if err != nil {
					b.Fatal(err)
				}
				if _, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
