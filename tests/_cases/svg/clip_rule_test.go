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

func TestSVGClipRuleCascadeInheritanceAndUseContexts(t *testing.T) {
	const outline = "M0 0H40V40H0Z M10 10H30V30H10Z"
	for _, tc := range []struct {
		name, style, defs, definition, child, host string
		want                                       path.FillRule
	}{
		{name: "default", want: path.NonZero},
		{name: "defs inherited", defs: `clip-rule="evenodd"`, want: path.EvenOdd},
		{name: "definition inherited", definition: `clip-rule="evenodd"`, want: path.EvenOdd},
		{name: "child override", definition: `clip-rule="evenodd"`, child: `clip-rule="nonzero"`, want: path.NonZero},
		{name: "child inline", definition: `clip-rule="nonzero"`, child: `style="clip-rule:evenodd"`, want: path.EvenOdd},
		{name: "css specificity", style: `path {clip-rule:nonzero} #shape {clip-rule:evenodd}`, want: path.EvenOdd},
		{name: "css inherit", definition: `clip-rule="evenodd"`, child: `style="clip-rule:inherit"`, want: path.EvenOdd},
		{name: "css unset inherited", definition: `clip-rule="evenodd"`, child: `style="clip-rule:unset"`, want: path.EvenOdd},
		{name: "css initial", definition: `clip-rule="evenodd"`, child: `style="clip-rule:initial"`, want: path.NonZero},
		{name: "graphic host cannot override definition", host: `clip-rule="evenodd"`, want: path.NonZero},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := fmt.Sprintf(`<svg viewBox="0 0 100 100"><style>%s</style><defs %s><clipPath id="c" %s><path id="shape" d="%s" %s/></clipPath></defs><rect id="visible" width="100" height="100" clip-path="url(#c)" %s/></svg>`, tc.style, tc.defs, tc.definition, outline, tc.child, tc.host)
			root := readInstances(t, source)
			o := cssObject(t, root, "visible")
			if o.Clip == nil || o.Clip.FillRule() != tc.want {
				t.Fatal("computed rule lost", o.Clip)
			}
			image := renderInstances(t, source, 100, 100)
			if (image.RGBAAt(20, 20).A == 0) != (tc.want == path.EvenOdd) {
				t.Fatal("wrong compound-hole pixels", image.RGBAAt(20, 20))
			}
		})
	}
	// Same target, three rules resolved through distinct use contexts. Target's
	// own explicit nonzero overrides its host; inheritance never comes from the
	// source target's original defs ancestors once it is instantiated by use.
	source := `<svg viewBox="0 0 100 100"><style>.odd {clip-rule:evenodd} #inherit {clip-rule:inherit}</style>
 <defs clip-rule="nonzero"><path id="inherit" d="` + outline + `"/><path id="explicit" d="` + outline + `" clip-rule="nonzero"/>
 <clipPath id="a"><use class="odd" href="#inherit"/></clipPath><clipPath id="b"><use href="#inherit"/></clipPath>
 <clipPath id="c"><use class="odd" href="#explicit"/></clipPath></defs>
 <rect id="a-leaf" width="40" height="40" clip-path="url(#a)"/><rect id="b-leaf" width="40" height="40" clip-path="url(#b)"/>
 <rect id="c-leaf" width="40" height="40" clip-path="url(#c)"/></svg>`
	root := readInstances(t, source)
	for id, want := range map[string]path.FillRule{"a-leaf": path.EvenOdd, "b-leaf": path.NonZero, "c-leaf": path.NonZero} {
		if got := cssObject(t, root, id).Clip.FillRule(); got != want {
			t.Fatal("source cache leaked a computed use rule", id, got)
		}
	}
}

func TestSVGClipEvenOddIndependentCairoFixtures(t *testing.T) {
	const directory = "../tests/data/render/clip-evenodd"
	for _, name := range []string{"compound", "duplicate", "curve", "crossings", "thin", "nested-bitmap", "gradient", "css-use"} {
		t.Run(name, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(directory, name+".svg"))
			if err != nil {
				t.Fatal(err)
			}
			o, err := svg.Read(bytes.NewReader(source), svg.Options{Width: 128})
			if err != nil {
				t.Fatal(err)
			}
			for _, scale := range []int{1, 4} {
				config := render.DefaultConfig()
				config.Width, config.Height, config.FrameWidth, config.Workers, config.Background, config.Supersampling = 128, 96, 128, 1, gomath.Transparent, scale
				r, err := render.NewRasterizer(config)
				if err != nil {
					t.Fatal(err)
				}
				got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Open(filepath.Join(directory, fmt.Sprintf("%s-cairo-ss%d.png", name, scale)))
				if err != nil {
					t.Fatal(err)
				}
				ref, err := png.Decode(file)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
				sum, squares, maximum := 0.0, 0.0, 0
				for y := range 96 {
					for x := range 128 {
						a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(ref.At(x, y)).(color.RGBA)
						for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
							sum += math.Abs(float64(d))
							squares += float64(d * d)
							maximum = max(maximum, int(math.Abs(float64(d))))
						}
					}
				}
				mae, rms := sum/(128*96*4), math.Sqrt(squares/(128*96*4))
				t.Logf("scale=%d MAE=%.6f RMS=%.6f max=%d", scale, mae, rms, maximum)
				if mae > .4 || rms > 2 || maximum > 32 {
					t.Fatal("imported evenodd reference bounds exceeded")
				}
				if name == "css-use" && (got.RGBAAt(28, 44).A != 0 || got.RGBAAt(96, 44).A != 255) {
					t.Fatal("use rule contexts were conflated")
				}
			}
		})
	}
}

func TestSVGClipEvenOddRetainsUnsupportedBoundaries(t *testing.T) {
	for _, source := range []string{
		`<svg><clipPath clip-rule="evenodd"><rect/><rect/></clipPath></svg>`,
		`<svg><clipPath clip-rule="evenodd" clipPathUnits="invalidUnits"><rect/></clipPath></svg>`,
		`<svg><clipPath clip-rule="evenodd"><g><rect/></g></clipPath></svg>`,
		`<svg><style>clipPath {clip-rule:evenodd!important}</style><clipPath/></svg>`,
		`<svg><clipPath id="c" clip-rule="evenodd" clip-path="url(#c)"/></svg>`,
		`<svg><rect style="fill-rule:winding" width="1" height="1"/></svg>`,
	} {
		if o, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err == nil || o != nil {
			t.Fatal("unsupported semantics accepted", source)
		}
	}
}
