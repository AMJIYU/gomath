package svg_test

import (
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"math"
	"strings"
	"testing"
)

func TestAllPathCommands(t *testing.T) {
	cases := []string{`M0 0 10 0 10 10z`, `m1 1 l2 0 h2 v2 H0 V0 Z`, `M0 0C1 0 1 1 2 1S3 2 4 1`, `M0 0Q1 2 2 0T4 0`, `M0 0 A10 20 30 0 1 20 0`, `M0 0a10 10 0 0110 10`, `M.5-.5L1e1-2.5e-1`}
	for _, data := range cases {
		t.Run(data, func(t *testing.T) {
			p, e := svg.ParsePath(data)
			if e != nil {
				t.Fatal(e)
			}
			if e = p.Validate(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestSmoothControlReflection(t *testing.T) {
	p, e := svg.ParsePath(`M0 0 C1 0 1 1 2 1 S3 2 4 1`)
	if e != nil {
		t.Fatal(e)
	}
	c := p.Contours[0].Curves
	if c[1].P1.Sub(geom.V(3, 1)).Norm() > 1e-10 {
		t.Fatal("smooth cubic control wrong")
	}
}
func TestEllipticalArcEndpointsAndBounds(t *testing.T) {
	p, e := svg.ParsePath(`M10 0 A10 10 0 1 1 -10 0 A10 10 0 1 1 10 0 Z`)
	if e != nil {
		t.Fatal(e)
	}
	b := p.Bounds()
	if math.Abs(b.Min.X+10) > 1e-8 || math.Abs(b.Max.Y-10) > 1e-8 || math.Abs(b.Min.Y+10) > 1e-8 {
		t.Fatal("circle arc bounds wrong", b)
	}
}
func TestImportViewBoxTransformsAndStyle(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><g transform="translate(10,20) scale(2)" fill="#f00" opacity="0.5"><rect id="box" x="0" y="0" width="10" height="20"/></g></svg>`
	o, e := svg.Read(strings.NewReader(source), svg.Options{Width: 4})
	if e != nil {
		t.Fatal(e)
	}
	var box *gomath.Object
	o.Walk(func(c *gomath.Object) {
		if c.Name == "box" {
			box = c
		}
	})
	if box == nil {
		t.Fatal("missing imported shape")
	}
	b := box.Bounds()
	if math.Abs(b.Min.X+1.6) > 1e-10 || math.Abs(b.Max.Y-1.2) > 1e-10 || math.Abs(b.Size().Y-1.6) > 1e-10 {
		t.Fatal("transform wrong", b)
	}
	if box.Style.Fill != gomath.RGB(1, 0, 0) {
		t.Fatal("fill inheritance wrong")
	}
	if o.Children[0].Children[0].Style.Opacity != .5 {
		t.Fatal("group opacity missing")
	}
}
func TestRoundRectAndBasicShapes(t *testing.T) {
	s := `<svg width="100" height="100"><rect x="0" y="0" width="20" height="30" rx="4"/><ellipse cx="40" cy="50" rx="20" ry="10"/><line x1="0" y1="0" x2="50" y2="50" stroke="blue"/><polygon points="0,0 10,0 10,10"/></svg>`
	o, e := svg.Read(strings.NewReader(s), svg.DefaultOptions())
	if e != nil {
		t.Fatal(e)
	}
	o.Walk(func(c *gomath.Object) {
		if e := c.Path.Validate(); e != nil {
			t.Fatal(e)
		}
	})
}
func TestRejectUnsupportedAndMalformed(t *testing.T) {
	for _, s := range []string{`<svg><use href="#x"/></svg>`, `<svg><style>.x:hover{fill:red}</style></svg>`, `<svg><path d="M0 0L1 1" fill="url(#gradient)"/></svg>`, `<svg><path d="L1 1"/></svg>`, `<svg><rect width="20" height="10" fill-rule="winding"/></svg>`, `<svg><circle r="10" transform="scale(2,1)" stroke="red"/></svg>`} {
		if _, e := svg.Read(strings.NewReader(s), svg.DefaultOptions()); e == nil {
			t.Errorf("accepted unsupported SVG %s", s)
		}
	}
	for _, s := range []string{`M0`, `M0 0L1`, `M0 0A1 1 0 2 0 1 1`, `M0 0R1 2`} {
		if _, e := svg.ParsePath(s); e == nil {
			t.Errorf("accepted malformed path %s", s)
		}
	}
}
func FuzzParsePath(f *testing.F) {
	f.Add("M0 0L1 1Z")
	f.Add("M0 0a1 1 0 0 1 1 1")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			t.Skip()
		}
		p, e := svg.ParsePath(s)
		if e == nil {
			if e = p.Validate(); e != nil {
				t.Fatal(e)
			}
		}
	})
}
