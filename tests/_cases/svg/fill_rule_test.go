package svg_test

import (
	"bytes"
	"fmt"
	"gomath"
	"gomath/path"
	"gomath/render"
	"gomath/svg"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSVGOrdinaryFillRuleCascadeInheritanceAndUse(t *testing.T) {
	const outline = "M0 0H40V40H0Z M10 10H30V30H10Z"
	for _, tc := range []struct {
		name, sheet, group, leaf string
		want                     path.FillRule
	}{
		{name: "default"},
		{name: "inherited", group: `fill-rule="evenodd"`, want: path.EvenOdd},
		{name: "presentation override", group: `fill-rule="evenodd"`, leaf: `fill-rule="nonzero"`},
		{name: "inline", leaf: `style="fill-rule:evenodd"`, want: path.EvenOdd},
		{name: "specificity", sheet: `path {fill-rule:nonzero} .odd {fill-rule:nonzero} #leaf {fill-rule:evenodd}`, leaf: `class="odd"`, want: path.EvenOdd},
		{name: "source order", sheet: `#leaf {fill-rule:nonzero} #leaf {fill-rule:evenodd}`, want: path.EvenOdd},
		{name: "inline precedence", sheet: `#leaf {fill-rule:nonzero}`, leaf: `fill-rule="nonzero" style="fill-rule:evenodd"`, want: path.EvenOdd},
		{name: "selector list child descendant", sheet: `svg path {fill-rule:nonzero} .absent, g > path {fill-rule:evenodd}`, want: path.EvenOdd},
		{name: "inherit", group: `fill-rule="evenodd"`, leaf: `style="fill-rule:inherit"`, want: path.EvenOdd},
		{name: "unset", group: `fill-rule="evenodd"`, leaf: `fill-rule="unset"`, want: path.EvenOdd},
		{name: "initial", group: `fill-rule="evenodd"`, leaf: `style="fill-rule:initial"`},
		{name: "clip rule independent", leaf: `clip-rule="evenodd"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf(`<svg viewBox="0 0 100 100"><style>%s</style><g %s><path id="leaf" d="%s" %s/></g></svg>`, tc.sheet, tc.group, outline, tc.leaf)
			o := cssObject(t, readInstances(t, source), "leaf")
			if o.Style.FillRule != tc.want {
				t.Fatal("computed fill rule", o.Style.FillRule, tc.want)
			}
			img := renderInstances(t, source, 100, 100)
			if (img.RGBAAt(20, 20).A == 0) != (tc.want == path.EvenOdd) {
				t.Fatal("cascade did not change actual fill")
			}
		})
	}
	source := `<svg viewBox="0 0 100 100"><style>.odd {fill-rule:evenodd} #inherited {fill-rule:inherit}</style>
 <defs fill-rule="evenodd"><path id="inherited" d="` + outline + `"/><path id="explicit" d="` + outline + `" fill-rule="nonzero"/></defs>
 <use class="odd" href="#inherited"/><use href="#inherited" x="45"/>
 <use class="odd" href="#explicit" y="45"/></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 3 {
		t.Fatal("use shape count")
	}
	for i, want := range []path.FillRule{path.EvenOdd, path.NonZero, path.NonZero} {
		if shapes[i].Style.FillRule != want {
			t.Fatal("use rule inherited from definition ancestor/leaked between instances", i)
		}
	}
	shapes[0].SetFillRule(path.NonZero)
	if shapes[1].Style.FillRule != path.NonZero || shapes[0].ID == shapes[1].ID {
		t.Fatal("instance ownership")
	}
	// Native font outlines go through styledShape as well; group-level rule
	// assignments must not overwrite tspan's independently computed rule.
	text := readInstances(t, `<svg viewBox="0 0 100 100"><text id="text" y="40" font-family="Go" fill-rule="evenodd">O<tspan id="span" fill-rule="nonzero">O</tspan></text></svg>`)
	seen := map[path.FillRule]bool{}
	for _, o := range instanceShapes(text) {
		seen[o.Style.FillRule] = true
	}
	if !seen[path.EvenOdd] || !seen[path.NonZero] {
		t.Fatal("glyph/tspan fill rules lost", seen)
	}
	for _, source := range []string{
		`<svg><rect fill-rule="winding"/></svg>`, `<svg><style>.unused{fill-rule:winding}</style></svg>`,
		`<svg><rect style="fill-rule:evenodd!important"/></svg>`,
		`<svg><clipPath id="c" clipPathUnits="invalidUnits"><path/></clipPath></svg>`,
		`<svg><rect fill="rgb(100%,0,0%)"/></svg>`, `<svg><rect fill="rgb(101%,0%,0%)"/></svg>`,
		`<svg><rect fill="rgb(NaN%,0%,0%)"/></svg>`, `<svg><rect fill="rgb(0%,0%)"/></svg>`,
	} {
		if o, e := svg.Read(strings.NewReader(source), svg.DefaultOptions()); e == nil || o != nil {
			t.Fatal("unsupported fill/CSS/clip boundary", source)
		}
	}
	precise := readInstances(t, `<svg><path id="precise" d="M0 0H1V1Z" fill="rgb(12.3456789%, 0%, 100%)" fill-rule="evenodd"/></svg>`)
	if got := cssObject(t, precise, "precise").Style.Fill; math.Abs(got.R-.123456789) > 1e-14 || got.G != 0 || got.B != 1 {
		t.Fatal("export-compatible percentage RGB precision", got)
	}
}

func TestSVGOrdinaryEvenOddIndependentCairoReferences(t *testing.T) {
	const directory = "../tests/data/render/fill-evenodd"
	for _, name := range []string{"compound", "duplicate", "triple", "curve", "crossings", "thin", "linear", "radial", "partial", "nested-clip", "stroke-dash", "css-use"} {
		t.Run(name, func(t *testing.T) {
			source, e := os.ReadFile(filepath.Join(directory, name+".svg"))
			if e != nil {
				t.Fatal(e)
			}
			o, e := svg.Read(bytes.NewReader(source), svg.Options{Width: 128})
			if e != nil {
				t.Fatal(e)
			}
			for _, scale := range []int{1, 4} {
				c := render.DefaultConfig()
				c.Width, c.Height, c.FrameWidth, c.Workers, c.Supersampling, c.Background = 128, 96, 128, 1, scale, gomath.Transparent
				r, e := render.NewRasterizer(c)
				if e != nil {
					t.Fatal(e)
				}
				got, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
				if e != nil {
					t.Fatal(e)
				}
				f, e := os.Open(filepath.Join(directory, fmt.Sprintf("%s-cairo-ss%d.png", name, scale)))
				if e != nil {
					t.Fatal(e)
				}
				want, e := png.Decode(f)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
				sum, squares, mx := 0.0, 0.0, 0
				for y := range 96 {
					for x := range 128 {
						a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
						for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
							sum += math.Abs(float64(d))
							squares += float64(d * d)
							mx = max(mx, int(math.Abs(float64(d))))
						}
					}
				}
				mae, rms := sum/(128*96*4), math.Sqrt(squares/(128*96*4))
				t.Logf("ss%d MAE=%.6f RMS=%.6f max=%d", scale, mae, rms, mx)
				if mae > .4 || rms > 2 || mx > 32 {
					t.Fatal("import/direct Cairo mismatch")
				}
			}
		})
	}
}
