package svg_test

import (
	"gomath"
	"gomath/svg"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestSVGDashCascadeInheritanceUseAndScaling(t *testing.T) {
	source := `<svg viewBox="0 0 100 100" stroke="red" fill="none" stroke-dasharray="4, 2" stroke-dashoffset="-1">
 <style>.s{stroke-dasharray:3 1 2;stroke-dashoffset:2} #a{stroke-dasharray:7 1}</style>
 <defs><g stroke-dasharray="50 50"><path id="tile" d="M0 0L20 0"/></g></defs>
 <path id="a" class="s" d="M0 0L20 0" style="stroke-dasharray:5px 1px"/>
 <g transform="translate(0 10) scale(2)"><path id="b" d="M0 0L20 0"/></g>
 <path id="reset" d="M0 20L20 20" style="stroke-dasharray:initial;stroke-dashoffset:initial"/>
 <path id="newarray" d="M0 30L20 30" stroke-dasharray="2 3"/>
 <use href="#tile" x="10" y="40" stroke-dasharray="3 2" stroke-dashoffset="1"/>
 <path id="odd" class="s" d="M0 50L20 50"/>
 </svg>`
	root, err := svg.Read(strings.NewReader(source), svg.Options{Width: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		lengths []float64
		offset  float64
	}{
		"a": {[]float64{.5, .1}, .2}, "b": {[]float64{.8, .4}, 1}, "reset": {nil, 0},
		"newarray": {[]float64{.2, .3}, .4}, "tile": {[]float64{.3, .2}, .1}, "odd": {[]float64{.3, .1, .2, .3, .1, .2}, .2},
	}
	root.Walk(func(o *gomath.Object) {
		w, ok := want[o.Name]
		if !ok {
			return
		}
		delete(want, o.Name)
		got := o.Style.Dash
		if len(got.Lengths()) != len(w.lengths) {
			t.Errorf("%s lengths %v want %v", o.Name, got.Lengths(), w.lengths)
			return
		}
		for i, v := range got.Lengths() {
			if math.Abs(v-w.lengths[i]) > 1e-12 {
				t.Errorf("%s lengths %v want %v", o.Name, got.Lengths(), w.lengths)
			}
		}
		if math.Abs(got.Offset()-w.offset) > 1e-12 {
			t.Errorf("%s offset %g want %g", o.Name, got.Offset(), w.offset)
		}
	})
	if len(want) != 0 {
		t.Fatal("missing dash objects", want)
	}
}
func TestSVGDashIndependentOffsetInheritanceAndKeywords(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100" stroke-dasharray="4 2" stroke-dashoffset="7"><path id="a" d="M0 0L10 0" stroke-dasharray="2 3"/><path id="b" d="M0 0L10 0" style="stroke-dasharray:inherit;stroke-dashoffset:unset"/><path id="c" d="M0 0L10 0" stroke-dasharray="none"/><path id="d" d="M0 0L10 0" stroke-dashoffset="initial"/></svg>`)
	values := map[string]float64{"a": 2, "b": 1, "c": 0, "d": 0}
	for _, o := range instanceShapes(root) {
		if o.Style.Dash.Offset() != values[o.Name] {
			t.Errorf("%s offset %g want %g", o.Name, o.Style.Dash.Offset(), values[o.Name])
		}
	}
	// An image ignores vector dashes, as it ignores fill and stroke paint.
	uri := imageURI(t, "embedded.png", "png")
	image := readInstances(t, imageDoc(uri, `stroke-dasharray="1 2" stroke-dashoffset="3"`))
	if instanceShapes(image)[0].Style.Dash != nil {
		t.Fatal("image gained a vector stroke pattern")
	}
	// CSS lengths accept absolute units and exponent notation, not percentages.
	units := readInstances(t, `<svg viewBox="0 0 100 100"><path d="M0 0L10 0" style="stroke-dasharray:1pt, 2e0px;stroke-dashoffset:-1px"/></svg>`)
	if got := instanceShapes(units)[0].Style.Dash.Lengths(); !reflect.DeepEqual(got, []float64{96.0 / 72, 2}) {
		t.Fatal("length units", got)
	}
}
func TestSVGDashRejectUnsupportedAndInvalid(t *testing.T) {
	for _, attr := range []string{
		`stroke-dasharray="0 0"`, `stroke-dasharray="-1 2"`, `stroke-dasharray="NaN 2"`, `stroke-dasharray="Inf 2"`,
		`stroke-dasharray=""`, `stroke-dasharray="2,,3"`, `stroke-dasharray="2,"`, `stroke-dasharray=",2"`,
		`stroke-dasharray="10% 5%"`, `stroke-dasharray="calc(2) 3"`, `stroke-dashoffset="10%"`, `stroke-dashoffset="NaN"`,
		`stroke-dasharray="2 3" transform="scale(2 1)"`, `stroke-dasharray="2 3" pathLength="10"`,
	} {
		source := `<svg><path d="M0 0L10 0" stroke="red" ` + attr + `/></svg>`
		if _, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err == nil {
			t.Errorf("accepted invalid SVG dash: %s", source)
		}
	}
	for _, css := range []string{`path{stroke-dasharray:0 0}`, `.unmatched{stroke-dasharray:-1 2}`, `path{stroke-dashoffset:NaN}`, `path{stroke-dasharray:1 2 !important}`} {
		if _, err := svg.Read(strings.NewReader(`<svg><style>`+css+`</style></svg>`), svg.DefaultOptions()); err == nil {
			t.Errorf("accepted CSS %s", css)
		}
	}
	tooMany := strings.Repeat("1 ", gomath.MaxDashEntries+1)
	if _, err := svg.Read(strings.NewReader(`<svg stroke-dasharray="`+tooMany+`"/>`), svg.DefaultOptions()); err == nil {
		t.Fatal("unbounded dash list")
	}
}
