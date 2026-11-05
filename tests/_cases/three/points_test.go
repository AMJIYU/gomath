package three

import (
	"gomath/geom"
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"
	"unsafe"
)

func mustPointCloud(tb testing.TB, positions []geom.Vec3, colors []color.NRGBA, radii []float64) *PointCloud {
	tb.Helper()
	p, err := NewPointCloud(positions, colors, radii)
	if err != nil {
		tb.Fatal(err)
	}
	return p
}

func onePoint(tb testing.TB, p geom.Vec3, c color.NRGBA, radius float64) *PointCloud {
	return mustPointCloud(tb, []geom.Vec3{p}, []color.NRGBA{c}, []float64{radius})
}

func TestPointCloudCopiesAndValues(t *testing.T) {
	positions := []geom.Vec3{{X: 1, Y: 2, Z: -3}}
	colors := []color.NRGBA{{R: 73, G: 141, B: 235, A: 128}}
	radii := []float64{2.75}
	p := mustPointCloud(t, positions, colors, radii)
	positions[0], colors[0], radii[0] = geom.Vec3{}, color.NRGBA{}, 64
	v, c, radius, err := p.Point(0)
	if err != nil || v != (geom.Vec3{X: 1, Y: 2, Z: -3}) || c != (color.NRGBA{R: 73, G: 141, B: 235, A: 128}) || radius != 2.75 || p.Len() != 1 {
		t.Fatalf("owned copy: %v %v %g %v", v, c, radius, err)
	}
	v.X, c.R, radius = 0, 0, 1
	v2, c2, r2, _ := p.Point(0)
	if v2.X != 1 || c2.R != 73 || r2 != 2.75 {
		t.Fatal("accessor exposed mutable storage")
	}
	defaults := mustPointCloud(t, positions, colors, nil)
	_, _, radius, _ = defaults.Point(0)
	if radius != DefaultPointRadius || defaults.radii != nil {
		t.Fatal("nil radii default")
	}
	for _, index := range []int{-1, 1, int(^uint(0) >> 1)} {
		if _, _, _, err := p.Point(index); err == nil {
			t.Fatal("out-of-range point accessor accepted")
		}
	}
	for _, invalid := range []*PointCloud{nil, {}} {
		if invalid.Len() != 0 {
			t.Fatal("invalid Len")
		}
		if _, _, _, err := invalid.Point(0); err == nil {
			t.Fatal("invalid accessor accepted")
		}
	}
	if unsafe.Sizeof(rasterPoint{}) > 48 {
		t.Fatal("point preparation payload changed; update documented budget")
	}
}

func TestPointCloudInputAndFrameBounds(t *testing.T) {
	pos := []geom.Vec3{{Z: -2}}
	colors := []color.NRGBA{{A: 255}}
	for _, radii := range [][]float64{{}, {1, 2}, {0}, {-1}, {MinPointRadius / 2}, {MaxPointRadius + 1}, {math.NaN()}, {math.Inf(1)}} {
		if p, err := NewPointCloud(pos, colors, radii); p != nil || err == nil {
			t.Fatalf("invalid radii accepted: %v", radii)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), MaxCoordinate + 1} {
		for _, p := range []geom.Vec3{{X: value}, {Y: value}, {Z: value}} {
			if cloud, err := NewPointCloud([]geom.Vec3{p}, []color.NRGBA{{}}, nil); cloud != nil || err == nil {
				t.Fatalf("invalid position accepted even with alpha zero: %v", p)
			}
		}
	}
	for _, pair := range []struct {
		p []geom.Vec3
		c []color.NRGBA
	}{{nil, nil}, {pos, nil}, {pos, make([]color.NRGBA, 2)}, {make([]geom.Vec3, MaxPoints+1), make([]color.NRGBA, MaxPoints+1)}} {
		if p, err := NewPointCloud(pair.p, pair.c, nil); p != nil || err == nil {
			t.Fatal("invalid count accepted")
		}
	}
	for _, radius := range []float64{MinPointRadius, MaxPointRadius} {
		_ = onePoint(t, geom.Vec3{X: MaxCoordinate, Y: -MaxCoordinate, Z: MaxCoordinate}, colors[0], radius)
	}
	cfg := DefaultConfig(1, 1)
	r, _ := NewRenderer(cfg)
	camera := testCamera(Orthographic)
	for _, clouds := range [][]*PointCloud{{nil}, {{}}, make([]*PointCloud, MaxPointClouds+1)} {
		if img, err := r.RenderWithPoints(camera, clouds); img != nil || err == nil {
			t.Fatal("invalid cloud list accepted")
		}
	}
	if img, err := (*Renderer)(nil).RenderWithPoints(camera, nil); img != nil || err == nil {
		t.Fatal("nil renderer accepted")
	}
	large := mustPointCloud(t, make([]geom.Vec3, MaxPoints), make([]color.NRGBA, MaxPoints), nil)
	if large.Len() != MaxPoints {
		t.Fatal("maximum count rejected")
	}
	if _, err := r.RenderWithPoints(camera, []*PointCloud{large}); err != nil {
		t.Fatalf("valid maximum cloud: %v", err)
	}
	small := onePoint(t, pos[0], color.NRGBA{}, 1)
	if img, err := r.RenderWithPoints(camera, []*PointCloud{large, small}); img != nil || err == nil || !strings.Contains(err.Error(), "point count") {
		t.Fatalf("aggregate count not bounded: %v", err)
	}
	clouds := make([]*PointCloud, MaxPointClouds)
	for i := range clouds {
		clouds[i] = small
	}
	if _, err := r.RenderWithPoints(camera, clouds); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int{-1, MaxPointSamples + 1, int(^uint(0) >> 1)} {
		cfg.PointSampleLimit = limit
		if _, err := NewRenderer(cfg); err == nil {
			t.Fatal("invalid point sample limit accepted")
		}
	}
	for _, limit := range []int{0, 1, MaxPointSamples} {
		cfg.PointSampleLimit = limit
		if _, err := NewRenderer(cfg); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPointSampleAndFragmentBudgets(t *testing.T) {
	// Orthographic height=3 and output=3x3 map world units to output pixels.
	// A center radius-1 disk has a 3x3 candidate box and five covered centers.
	cfg := DefaultConfig(3, 3)
	cfg.Light = Light{Ambient: 1}
	cfg.PointSampleLimit, cfg.TransparentFragmentLimit = 9, 5
	camera := testCamera(Orthographic)
	camera.OrthoHeight = 3
	cloud := onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{R: 201, G: 43, B: 78, A: 128}, 1)
	r, _ := NewRenderer(cfg)
	if _, err := r.RenderWithPoints(camera, []*PointCloud{cloud}); err != nil {
		t.Fatal(err)
	}
	cfg.PointSampleLimit = 8
	r, _ = NewRenderer(cfg)
	if img, err := r.RenderWithPoints(camera, []*PointCloud{cloud}); img != nil || err == nil || !strings.Contains(err.Error(), "point sample budget") {
		t.Fatalf("sample overflow: %v", err)
	}
	cfg.PointSampleLimit, cfg.TransparentFragmentLimit = 9, 4
	r, _ = NewRenderer(cfg)
	if img, err := r.RenderWithPoints(camera, []*PointCloud{cloud}); img != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatalf("fragment overflow: %v", err)
	}
	// Final opaque mesh depth rejects all disk fragments before accounting.
	blocker := NewInstance(uvQuad(2, 2, 1.5))
	if _, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, blocker); err != nil {
		t.Fatal(err)
	}
	// A failed render must not poison subsequent frames.
	if _, err := r.RenderWithPoints(camera, nil); err != nil {
		t.Fatal(err)
	}
	// Candidate work is charged before occlusion; opaque/duplicate points also
	// consume it. Invisible and near/far/viewport-clipped points do not.
	cfg.PointSampleLimit, cfg.TransparentFragmentLimit = 17, 1
	r, _ = NewRenderer(cfg)
	opaque := onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{A: 255}, 1)
	if img, err := r.RenderWithPoints(camera, []*PointCloud{opaque, opaque}, blocker); img != nil || err == nil || !strings.Contains(err.Error(), "point sample") {
		t.Fatalf("opaque work was unbounded: %v", err)
	}
	cfg.PointSampleLimit = 1
	r, _ = NewRenderer(cfg)
	for _, skipped := range []*PointCloud{
		onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{}, 64),
		onePoint(t, geom.Vec3{Z: -.5}, color.NRGBA{A: 255}, 64),
		onePoint(t, geom.Vec3{Z: -11}, color.NRGBA{A: 255}, 64),
		onePoint(t, geom.Vec3{X: MaxCoordinate, Z: -2}, color.NRGBA{A: 255}, 64),
	} {
		if _, err := r.RenderWithPoints(camera, []*PointCloud{skipped}); err != nil {
			t.Fatal(err)
		}
	}
	for _, scale := range []int{2, 4} {
		cfg.Supersampling = scale
		candidates := 4 * scale * scale
		covered := 0
		for y := range 3 * scale {
			for x := range 3 * scale {
				if math.Hypot((float64(x)+.5)/float64(scale)-1.5, (float64(y)+.5)/float64(scale)-1.5) <= 1 {
					covered++
				}
			}
		}
		cfg.PointSampleLimit, cfg.TransparentFragmentLimit = candidates, covered
		r, _ = NewRenderer(cfg)
		if _, err := r.RenderWithPoints(camera, []*PointCloud{cloud}); err != nil {
			t.Fatal(err)
		}
		cfg.PointSampleLimit--
		r, _ = NewRenderer(cfg)
		if img, err := r.RenderWithPoints(camera, []*PointCloud{cloud}); img != nil || err == nil {
			t.Fatal("SSAA sample budget not counted")
		}
		cfg.PointSampleLimit++
		cfg.TransparentFragmentLimit--
		r, _ = NewRenderer(cfg)
		if img, err := r.RenderWithPoints(camera, []*PointCloud{cloud}); img != nil || err == nil {
			t.Fatal("SSAA fragment budget not counted")
		}
	}
}

func TestPointAndMeshShareFragmentLimit(t *testing.T) {
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Ambient: 1}
	cfg.TransparentFragmentLimit = 2
	camera := testCamera(Orthographic)
	mesh := NewInstance(triangleMesh())
	mesh.Material.Color = color.NRGBA{B: 255, A: 128}
	cloud := onePoint(t, geom.Vec3{Z: -1.5}, color.NRGBA{R: 255, A: 128}, 1)
	for _, textured := range []bool{false, true} {
		if textured {
			mesh.Mesh.UVs = slices.Repeat([]UV{{}}, 3)
			mesh.Material.Texture = testTexture(t, 1, 1, func(_, _ int) color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: 173} })
		}
		cfg.TransparentFragmentLimit = 2
		r, _ := NewRenderer(cfg)
		if _, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, mesh); err != nil {
			t.Fatal(err)
		}
		cfg.TransparentFragmentLimit = 1
		r, _ = NewRenderer(cfg)
		if img, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, mesh); img != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
			t.Fatalf("shared texture=%t: %v", textured, err)
		}
	}
}
