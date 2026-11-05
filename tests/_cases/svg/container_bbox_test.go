package svg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"gomath/text"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

const containerBBoxData = "../tests/data/svg/container-bbox/"

func containerBBoxRead(t testing.TB, source []byte, width float64, kern bool) *gomath.Object {
	t.Helper()
	opt := svg.Options{Width: width}
	if kern {
		data, err := os.ReadFile("../tests/data/svg/text-kern.ttf")
		if err != nil {
			t.Fatal(err)
		}
		font, err := text.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		opt.FontResolver = func(string, text.FontStyle) (*text.Font, error) { return font, nil }
	}
	o, err := svg.Read(bytes.NewReader(source), opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	return o
}

func containerBBoxFile(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(containerBBoxData + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSVGContainerBBoxExactIndependentOracle(t *testing.T) {
	var oracle struct {
		Cases []struct {
			Name string
			Font string
			BBox [4]float64
		}
	}
	if err := json.Unmarshal(containerBBoxFile(t, "bbox-oracle.json"), &oracle); err != nil {
		t.Fatal(err)
	}
	for _, tc := range oracle.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			root := containerBBoxRead(t, containerBBoxFile(t, "oracle-"+tc.Name+".svg"), 128, tc.Font == "kern")
			object := cssObject(t, root, "target")
			if object.Clip == nil {
				t.Fatal("missing materialized full-bbox clip")
			}
			b := object.Clip.Bounds()
			got := [4]float64{b.Min.X + 64, 48 - b.Max.Y, b.Size().X, b.Size().Y}
			for i, v := range got {
				if math.Abs(v-tc.BBox[i]) > 1e-8 {
					t.Fatalf("bbox %v, independent oracle %v", got, tc.BBox)
				}
			}
		})
	}
}

func TestSVGContainerBBoxTransformOriginalCurvesBeforeExtrema(t *testing.T) {
	for _, size := range []float64{1e-100, 1e-15, 1, 1e15, 1e100} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			factor := 10 / size
			geometry := fmt.Sprintf(`<path d="M0 0C0 %.17g %.17g %.17g %.17g 0Z" transform="matrix(%.17g 0 %.17g %.17g 4 5)"/>`, size, size, size, size, factor, factor/2, factor)
			root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<g id="g" clip-path="url(#c)">`+geometry+`</g></svg>`)
			r := (1 + math.Sqrt(5)) / 4
			xMax := 15*r + 15*r*r - 20*r*r*r
			clipBox(t, cssObject(t, root, "g"), [4]float64{4, 5, xMax / 2, 7.5})
		})
	}
	// The target's own affine transform must not affect local measurement.
	for _, affine := range []string{"rotate(31 20 30)", "matrix(1 .2 .7 1 3 4)", "matrix(-2 .1 .2 1.5 90 2)", "scale(0 1)"} {
		body := `<g id="g" transform="` + affine + `" clip-path="url(#c)"><g transform="translate(3 4)"><rect x="10" y="20" width="40" height="30"/></g></g>`
		a := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+body+`</svg>`)
		b := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c"><rect x="13" y="24" width="20" height="30"/></clipPath></defs>`+body+`</svg>`)
		compareBBoxPaths(t, cssObject(t, a, "g").Clip.Path(), cssObject(t, b, "g").Clip.Path())
	}
}

func TestSVGContainerBBoxIgnoresPaintStrokeClippingAndDefinitions(t *testing.T) {
	source := `<svg viewBox="0 0 100 100">` + bboxHalf + `<defs><clipPath id="tiny"><rect width="1" height="1"/></clipPath></defs>
<g id="g" clip-path="url(#c)"><defs><rect x="-900" y="-900" width="2000" height="2000"/></defs><symbol><rect width="900" height="900"/></symbol>
<g clip-path="url(#tiny)"><rect x="10" y="20" width="40" height="30" fill="none" stroke="red" stroke-width="40"/></g>
<rect x="50" y="30" width="20" height="40" opacity="0"/><rect x="5" y="40" width="2" height="10" visibility="hidden"/>
<g display="none"><rect display="inline" x="-500" y="-500" width="1000" height="1000"/></g></g></svg>`
	root := readInstances(t, source)
	clipBox(t, cssObject(t, root, "g"), [4]float64{5, 20, 32.5, 50})
	// Hiding a subtree excludes it even if its descendants restore display.
	// A visible child can restore visibility; hidden siblings still affect bbox.
	root = readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<g id="g" visibility="hidden" clip-path="url(#c)"><rect x="10" y="20" width="60" height="30"/><rect visibility="visible" x="20" y="25" width="20" height="20"/></g></svg>`)
	clipBox(t, cssObject(t, root, "g"), [4]float64{10, 20, 30, 30})
}

func TestSVGContainerBBoxNestedTargetsMeasureUnclippedGeometry(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<g id="outer" clip-path="url(#c)"><g id="inner" transform="matrix(1 0 .5 1 10 20)" clip-path="url(#c)"><rect width="40" height="30"/></g></g></svg>`)
	clipBox(t, cssObject(t, root, "outer"), [4]float64{10, 20, 27.5, 30})
	clipBox(t, cssObject(t, root, "inner"), [4]float64{10, 20, 35, 30})
	// A precompiled multi-child silhouette still applies to the whole group;
	// bbox geometry never counts the clip's own contours as target geometry.
	source := `<svg viewBox="0 0 100 100"><defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width=".3" height="1"/><rect x=".7" width=".3" height="1"/></clipPath></defs><g clip-path="url(#c)"><rect x="10" y="20" width="80" height="60" fill="blue"/></g></svg>`
	img := renderInstances(t, source, 100, 100)
	for _, sample := range []struct {
		x       int
		visible bool
	}{{15, true}, {50, false}, {85, true}} {
		if (img.RGBAAt(sample.x, 50).A != 0) != sample.visible {
			t.Fatal("container multi-child clip union changed", sample)
		}
	}
}

func TestSVGContainerBBoxUseChainsTextAndCSS(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<defs transform="translate(900 900)" display="none"><g id="g" transform="translate(2 3)"><rect x="10" y="20" width="40" height="30"/></g><use id="chain" href="#g" x="4" y="5" transform="scale(2 .5)"/></defs><use id="host" href="#chain" x="6" y="7" clip-path="url(#c)"/></svg>`)
	clipBox(t, cssObject(t, root, "host"), [4]float64{38, 21, 40, 15})
	// A use to text uses host-inherited font size and anchor, and includes x/y.
	root = readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<defs><text id="s" x="20" y="50">AV</text></defs><use id="host" href="#s" x="4" y="5" font-size="24" clip-path="url(#c)"/></svg>`)
	_, metrics := textRun(t, "AV", text.Regular, 24)
	clipBox(t, cssObject(t, root, "host"), [4]float64{24, 55 - metrics.Ascent, metrics.Width / 2, metrics.Ascent + metrics.Descent})
	// The same definition inherits independent styles per shadow instance.
	root = readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<style>.host .tile .r{display:none}.tile > .r{display:inherit}</style><defs><g id="tile" class="tile"><rect class="r" x="10" y="20" width="40" height="30"/></g></defs><g class="host"><use id="a" href="#tile" clip-path="url(#c)"/><use id="b" href="#tile" x="20" clip-path="url(#c)"/></g></svg>`)
	clipBox(t, cssObject(t, root, "a"), [4]float64{10, 20, 20, 30})
	clipBox(t, cssObject(t, root, "b"), [4]float64{30, 20, 20, 30})
}

func TestSVGContainerBBoxTextSpanPositionAnchorAndNestedClips(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<text id="text" x="60" y="50" font-size="24" text-anchor="end" clip-path="url(#c)">A<tspan id="span" clip-path="url(#c)">V</tspan></text></svg>`)
	_, both := textRun(t, "AV", text.Regular, 24)
	_, last := textRun(t, "V", text.Regular, 24)
	clipBox(t, cssObject(t, root, "text"), [4]float64{60 - both.Width, 50 - both.Ascent, both.Width / 2, both.Ascent + both.Descent})
	clipBox(t, cssObject(t, root, "span"), [4]float64{60 - last.Width, 50 - last.Ascent, last.Width / 2, last.Ascent + last.Descent})
	root = readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<text x="10" y="30" font-size="20">A<tspan id="span" x="40" y="60" dx="3" dy="2" font-size="12" text-anchor="middle" clip-path="url(#c)">V<tspan font-size="24" visibility="hidden">g</tspan></tspan></text></svg>`)
	_, v := textRun(t, "V", text.Regular, 12)
	_, g := textRun(t, "g", text.Regular, 24)
	clipBox(t, cssObject(t, root, "span"), [4]float64{43 - (v.Width+g.Width)/2, 62 - g.Ascent, (v.Width + g.Width) / 2, g.Ascent + g.Descent})
	// display:none text does not consume advance or an ancestor's bbox.
	root = readInstances(t, `<svg viewBox="0 0 100 100">`+bboxHalf+`<text id="text" x="10" y="50" font-size="24" clip-path="url(#c)">A<tspan display="none" x="900">WWWW</tspan>V</text></svg>`)
	clipBox(t, cssObject(t, root, "text"), [4]float64{10, 50 - both.Ascent, both.Width / 2, both.Ascent + both.Descent})
}

func TestSVGContainerBBoxDegenerateAndEmptyGeometry(t *testing.T) {
	for _, body := range []string{``, `<path/>`, `<path d="M10 20"/>`, `<line x1="10" y1="20" x2="80" y2="20" stroke="red" stroke-width="8"/>`, `<g display="none"><rect width="80" height="90"/></g>`} {
		source := `<svg viewBox="0 0 100 100"><defs><clipPath id="c" clipPathUnits="objectBoundingBox"/></defs><g id="g" clip-path="url(#c)">` + body + `</g></svg>`
		root := readInstances(t, source)
		if cssObject(t, root, "g").Clip != nil {
			t.Fatal("empty/degenerate target must ignore bbox effect", body)
		}
		plain := strings.Replace(source, ` clip-path="url(#c)"`, "", 1)
		if !bytes.Equal(renderInstances(t, source, 100, 100).Pix, renderInstances(t, plain, 100, 100).Pix) {
			t.Fatal("degenerate geometry changed rendering", body)
		}
	}
}

func TestSVGContainerBBoxUnsupportedBranchesAreAtomic(t *testing.T) {
	for _, body := range []string{
		`<svg clip-path="url(#c)"><rect width="10" height="10"/></svg>`,
		`<g clip-path="url(#c)"><svg><rect width="10" height="10"/></svg></g>`,
		`<defs><symbol id="s" overflow="visible"><rect width="10" height="10"/></symbol></defs><use href="#s" clip-path="url(#c)"/>`,
		`<defs><symbol id="s" viewBox="0 0 10 10" overflow="visible"><rect width="10" height="10"/></symbol><g id="s2"><use href="#s"/></g></defs><use href="#s2" clip-path="url(#c)"/>`,
		`<g clip-path="url(#c)"><path d="M0 0C1e308 1e308 1e308 0 1 1" transform="scale(10)"/></g>`,
		`<g clip-path="url(#c)"><rect width="1e308" height="1e308" transform="scale(10)"/></g>`,
		`<defs><g id="cycle"><use href="#cycle"/></g></defs><use href="#cycle" clip-path="url(#c)"/>`,
		`<defs><g id="a"><use href="#b"/></g><g id="b"><use href="#a"/></g></defs><g clip-path="url(#c)"><use href="#a"/></g>`,
		`<g clip-path="url(#c)"><text>G<tspan transform="scale(2)">o</tspan></text></g>`,
		`<g clip-path="url(#c)"><text font-family="Missing">Go</text></g>`,
		`<g transform="scale(0)" clip-path="url(#c)"><g transform="scale(1e200)"><g transform="scale(1e200)"/></g></g>`,
		`<defs><g id="tile"><clipPath id="local" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath><g clip-path="url(#local)"><rect width="10" height="10"/></g></g></defs><use href="#tile"/>`,
	} {
		source := `<svg viewBox="0 0 100 100">` + bboxHalf + `<rect width="10" height="10"/>` + body + `</svg>`
		object, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
		if err == nil || object != nil {
			t.Fatal("unsupported/invalid geometry produced partial output", body, err)
		}
	}
}

func containerBBoxCompareImage(t *testing.T, got *image.RGBA, filename string) {
	t.Helper()
	want, err := png.Decode(bytes.NewReader(containerBBoxFile(t, filename)))
	if err != nil {
		t.Fatal(err)
	}
	var total, maximum, over int
	for y := range 96 {
		for x := range 128 {
			a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			pixelMax := 0
			for _, v := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				d := int(math.Abs(float64(v)))
				total += d
				maximum, pixelMax = max(maximum, d), max(pixelMax, d)
			}
			if pixelMax > 8 {
				over++
			}
		}
	}
	mean, ratio := float64(total)/(128*96*4), float64(over)/(128*96)
	t.Logf("container Cairo %s MAE=%.9g max=%d pixels>8=%.9g", filename, mean, maximum, ratio)
	// Independently antialiased native clip masks multiply coverage; Cairo
	// combines geometry before coverage. SS1 corners can differ by up to 80
	// channel levels; SS4 and the exact numerical oracle constrain geometry.
	limit := 45
	if strings.HasSuffix(filename, "-ss1.png") {
		limit = 80
	}
	if mean > .35 || maximum > limit || ratio > .02 {
		t.Fatal("independent container bbox reference mismatch")
	}
}

func TestSVGContainerBBoxIndependentCairoAndCamera(t *testing.T) {
	for _, name := range []string{"group-curves", "display-visibility", "use-chain", "nested-clips", "text-cells", "text-end", "tspan-kern", "group-image"} {
		for _, ss := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/ss%d", name, ss), func(t *testing.T) {
				root := containerBBoxRead(t, containerBBoxFile(t, name+".svg"), 8, name == "tspan-kern")
				got := renderBBoxFrame(t, root, ss, nil)
				containerBBoxCompareImage(t, got, fmt.Sprintf("%s-cairo-ss%d.png", name, ss))
				root.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(.3)))
				cam := gomath.Camera2D{Center: geom.V(10, 20), Rotation: .3, FrameWidth: 8}
				moved := renderBBoxFrame(t, root, ss, &cam)
				for i, v := range got.Pix {
					if d := int(v) - int(moved.Pix[i]); d < -1 || d > 1 {
						t.Fatalf("camera changed materialized container clip at %d delta %d", i, d)
					}
				}
			})
		}
	}
}

func TestSVGContainerBBoxAnimatedIntermediateCairoAndOwnership(t *testing.T) {
	root := containerBBoxRead(t, containerBBoxFile(t, "group-curves.svg"), 8, false)
	before := root.Copy()
	target := root.Copy()
	normalize := geom.Translation(geom.V(-4, 3)).Mul(geom.Scaling(1.0/16, -1.0/16, 1))
	inverse := geom.Scaling(16, -16, 1).Mul(geom.Translation(geom.V(4, -3)))
	affine := geom.Identity()
	affine[0], affine[1], affine[3] = .8, -.1, 10
	affine[4], affine[5], affine[7] = .12, 1.05, -3
	target.ApplyMatrix(normalize.Mul(affine).Mul(inverse))
	scene := gomath.NewScene()
	scene.Add(root)
	if err := scene.Play(1, gomath.Transform(root, target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, tick := range []int{0, 25, 50, 75, 100} {
		t.Run(fmt.Sprint(tick), func(t *testing.T) {
			t.Parallel()
			frame, err := scene.Sample(float64(tick) / 100)
			if err != nil {
				t.Fatal(err)
			}
			object := frame.Find(root.ID)
			for _, ss := range []int{1, 4} {
				containerBBoxCompareImage(t, renderBBoxFrame(t, object, ss, nil), fmt.Sprintf("animated-%03d-cairo-ss%d.png", tick, ss))
			}
		})
	}
	// Play updates the authored object to its endpoint; a saved import owns
	// its original geometry and clip independently of both animation snapshots.
	fresh := containerBBoxRead(t, containerBBoxFile(t, "group-curves.svg"), 8, false)
	compareBBoxPaths(t, instanceShapes(before)[0].Path, instanceShapes(fresh)[0].Path)
	compareBBoxPaths(t, cssObject(t, before, "target").Clip.Path(), cssObject(t, fresh, "target").Clip.Path())
}

func TestSVGContainerBBoxConcurrentImportsAndSnapshots(t *testing.T) {
	source := containerBBoxFile(t, "use-chain.svg")
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			root := containerBBoxRead(t, source, 8, false)
			copy := root.Copy()
			root.Shift(geom.V(1, 2))
			fresh := containerBBoxRead(t, source, 8, false)
			compareBBoxPaths(t, cssObject(t, copy, "target").Clip.Path(), cssObject(t, fresh, "target").Clip.Path())
		})
	}
}

func BenchmarkSVGContainerBBoxImportRender(b *testing.B) {
	var source strings.Builder
	source.WriteString(`<svg viewBox="0 0 128 96">` + bboxHalf + `<defs><g id="tile"><path d="M0 0C0 12 12 12 12 0Z" transform="matrix(1 0 .2 1 0 0)"/></g></defs>`)
	for i := range 64 {
		fmt.Fprintf(&source, `<use href="#tile" x="%d" y="%d" fill="#20a0e0" clip-path="url(#c)"/>`, i%8*16, i/8*12)
	}
	source.WriteString(`</svg>`)
	input := source.String()
	for _, ss := range []int{1, 4} {
		b.Run(fmt.Sprintf("64_containers_128x96_SS%d", ss), func(b *testing.B) {
			cfg := render.DefaultConfig()
			cfg.Width, cfg.Height, cfg.FrameWidth, cfg.Supersampling, cfg.Workers = 128, 96, 8, ss, 1
			raster, err := render.NewRasterizer(cfg)
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
