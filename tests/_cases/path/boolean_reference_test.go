package path_test

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
)

type booleanOracle struct {
	Samples [][2]float64 `json:"samples"`
	Cases   []struct {
		Name       string `json:"name"`
		A, B       [][][2]float64
		RuleA      string `json:"rule_a"`
		RuleB      string `json:"rule_b"`
		Operations map[string]struct {
			Area       float64 `json:"area"`
			Membership []bool  `json:"membership"`
		} `json:"operations"`
	} `json:"cases"`
}

func booleanReadOracle(t *testing.T) booleanOracle {
	t.Helper()
	data, err := os.ReadFile("../tests/data/path/boolean-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle booleanOracle
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	if len(oracle.Cases) != 12 || len(oracle.Samples) != 324 {
		t.Fatal("incomplete Boolean oracle")
	}
	return oracle
}
func booleanOraclePath(contours [][][2]float64) path.Path {
	var p path.Path
	for _, c := range contours {
		points := make([]geom.Vec3, len(c))
		for i, v := range c {
			points[i] = geom.V(v[0], v[1])
		}
		p.Contours = append(p.Contours, boolPolygon(points...).Contours...)
	}
	return p
}
func booleanOracleRule(s string) path.FillRule {
	if s == "evenodd" {
		return path.EvenOdd
	}
	return path.NonZero
}
func booleanOps() map[string]path.BooleanOp {
	return map[string]path.BooleanOp{"union": path.BooleanUnion, "intersection": path.BooleanIntersection, "difference": path.BooleanDifference, "xor": path.BooleanXor}
}

func TestBooleanIndependentSkiaPathopsReference(t *testing.T) {
	oracle := booleanReadOracle(t)
	for _, tc := range oracle.Cases {
		for name, op := range booleanOps() {
			t.Run(tc.Name+"/"+name, func(t *testing.T) {
				want, ok := tc.Operations[name]
				if !ok || len(want.Membership) != len(oracle.Samples) {
					t.Fatal("missing oracle result")
				}
				got, err := path.Boolean(booleanOraclePath(tc.A), booleanOraclePath(tc.B), op, path.BooleanOptions{RuleA: booleanOracleRule(tc.RuleA), RuleB: booleanOracleRule(tc.RuleB)})
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(boolArea(got)-want.Area) > 2e-5*math.Max(1, want.Area) {
					t.Fatalf("area=%g Skia=%g", boolArea(got), want.Area)
				}
				for i, v := range oracle.Samples {
					q := geom.V(v[0], v[1])
					if boolMembership(got, q, path.NonZero) != want.Membership[i] || boolMembership(got, q, path.EvenOdd) != want.Membership[i] {
						t.Fatalf("membership at %v differs from Skia", q)
					}
				}
			})
		}
	}
}

func TestBooleanIndependentCairoRasterReference(t *testing.T) {
	oracle := booleanReadOracle(t)
	tc := oracle.Cases[0]
	if tc.Name != "hole-overlap" {
		t.Fatal("wrong raster scene")
	}
	config := render.DefaultConfig()
	config.Width, config.Height, config.FrameWidth, config.Workers, config.Background = 128, 128, 16, 1, gomath.Transparent
	raster, err := render.NewRasterizer(config)
	if err != nil {
		t.Fatal(err)
	}
	for name, op := range booleanOps() {
		t.Run(name, func(t *testing.T) {
			p, err := path.Boolean(booleanOraclePath(tc.A), booleanOraclePath(tc.B), op, path.BooleanOptions{})
			if err != nil {
				t.Fatal(err)
			}
			o := gomath.NewObject(p.Transform(geom.Translation(geom.V(-8, -8))))
			o.Style.Fill = gomath.RGB(64./255, 128./255, 192./255)
			o.Style.StrokeWidth = 0
			got, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
			if err != nil {
				t.Fatal(err)
			}
			f, err := os.Open("../tests/data/path/boolean-" + name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != want.Bounds() {
				t.Fatal("reference dimensions differ")
			}
			total, maximum, changed := 0, 0, 0
			for y := range 128 {
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
			mean, ratio := float64(total)/(128*128*4), float64(changed)/(128*128)
			t.Logf("Skia/Cairo 128x128: premultiplied RGBA MAE=%.9g max=%d fraction>8=%.9g", mean, maximum, ratio)
			if mean > .08 || maximum > 24 || ratio > .003 {
				t.Fatal("Boolean reference raster mismatch")
			}
		})
	}
}
