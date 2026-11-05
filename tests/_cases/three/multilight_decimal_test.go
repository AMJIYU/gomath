package three

import (
	"encoding/json"
	"gomath/geom"
	"math"
	"os"
	"testing"
)

func TestMultiLightDecimalOracle(t *testing.T) {
	data, err := os.ReadFile("../tests/data/three/multilight/numerical.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Oracle string
		Cases  []struct {
			Name                string
			World, Normal, Want [3]float64
			Ambient             float64
			Lights              []struct {
				Position, RGB        [3]float64
				Diffuse, Attenuation float64
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 70 || fixture.Oracle == "" {
		t.Fatal("incomplete independent oracle")
	}
	vector := func(a [3]float64) geom.Vec3 { return geom.Vec3{X: a[0], Y: a[1], Z: a[2]} }
	for _, tc := range fixture.Cases {
		c := DefaultConfig(1, 1)
		c.Light = Light{Kind: PointLightList, Ambient: tc.Ambient}
		for _, p := range tc.Lights {
			c.PointLights = append(c.PointLights, PointLightSource{Position: vector(p.Position), RGB: p.RGB, Diffuse: p.Diffuse, Attenuation: p.Attenuation})
		}
		r, err := NewRenderer(c)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.pointIntensities(vector(tc.World), vector(tc.Normal), &pointLightWork{remaining: len(c.PointLights)})
		for i, ch := range got {
			if err != nil || !finite(ch) || math.Abs(ch-tc.Want[i]) > math.Abs(tc.Want[i])*2e-14 {
				t.Fatalf("%s channel%d: %.17g want %.17g %v", tc.Name, i, ch, tc.Want[i], err)
			}
		}
	}
}
