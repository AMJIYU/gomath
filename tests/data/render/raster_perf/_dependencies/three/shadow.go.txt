package three

import (
	"fmt"
	"gomath/geom"
	"math"
)

const (
	MaxShadowResolution      = 2048
	DefaultShadowSampleLimit = 16 * 1024 * 1024
	MaxShadowSamples         = 64 * 1024 * 1024
)

// ShadowConfig enables a directional, nearest-texel hard shadow map. Center is
// in world coordinates; Size is the full square side and Depth the full depth
// along -Light.Direction. Neither the volume nor resolution depends on Camera.
// Enabled=false ignores the other fields and retains the original renderer.
// Visible mesh materials must be fully opaque, including all texture texels.
// Points remain unlit and neither cast nor receive shadows.
type ShadowConfig struct {
	Enabled     bool
	Center      geom.Vec3
	Size, Depth float64
	Resolution  int
	// Bias is a constant nonnegative world/light-depth offset, at most Depth.
	// A receiver is shadowed only when receiverDepth > storedDepth + Bias.
	Bias float64
	// Zero budgets select DefaultShadowSampleLimit; explicit budgets are
	// 1..MaxShadowSamples. RasterSampleLimit counts clipped triangle bounding
	// boxes; LookupLimit counts depth-passing, positive-Lambert receiving samples
	// when Diffuse > 0 (including queries outside the local shadow volume).
	RasterSampleLimit, LookupLimit int
}

func (s ShadowConfig) validate(direction geom.Vec3) error {
	if !s.Enabled {
		return nil
	}
	if !bounded(s.Center) || !primitiveDimension(s.Size) || !primitiveDimension(s.Depth) || s.Resolution < 1 || s.Resolution > MaxShadowResolution || !finite(s.Bias) || s.Bias < 0 || s.Bias > s.Depth {
		return fmt.Errorf("three: shadows need bounded center, size/depth [1e-6,%g], resolution [1,%d], and finite bias [0,depth]", MaxCoordinate, MaxShadowResolution)
	}
	if _, ok := unit(direction); !ok {
		return fmt.Errorf("three: shadows require a nonzero directional light")
	}
	for _, limit := range []int{s.RasterSampleLimit, s.LookupLimit} {
		if limit < 0 || limit > MaxShadowSamples {
			return fmt.Errorf("three: shadow sample limits must be 0 (default %d) or 1..%d", DefaultShadowSampleLimit, MaxShadowSamples)
		}
	}
	return nil
}

// A fresh map and counters belong to one Render call, never the Renderer.
type shadowMap struct {
	config                 ShadowConfig
	frame                  cameraFrame
	depth                  []float64
	rasterSamples, lookups int64
}

func (s *shadowMap) view(p geom.Vec3) geom.Vec3 {
	p = p.Sub(s.config.Center)
	return geom.Vec3{X: p.Dot(s.frame.right), Y: p.Dot(s.frame.up), Z: p.Dot(s.frame.forward)}
}

func (r *Renderer) buildShadowMap(instances []preparedInstance) (*shadowMap, error) {
	config := r.config.Shadow
	if !config.Enabled {
		return nil, nil
	}
	if config.RasterSampleLimit == 0 {
		config.RasterSampleLimit = DefaultShadowSampleLimit
	}
	if config.LookupLimit == 0 {
		config.LookupLimit = DefaultShadowSampleLimit
	}
	forward := r.lightDirection.Mul(-1)
	auxiliary := geom.Vec3{Z: 1}
	if math.Abs(forward.Z) > .99 {
		auxiliary = geom.Vec3{Y: 1}
	}
	right, _ := unit(forward.Cross(auxiliary))
	frame := cameraFrame{forward: forward, right: right, up: right.Cross(forward), sx: config.Size / 2, sy: config.Size / 2}
	s := &shadowMap{config: config, frame: frame, depth: make([]float64, config.Resolution*config.Resolution)}
	for i := range s.depth {
		s.depth[i] = math.Inf(1)
	}
	// Shared clipping equations also work for a signed orthographic depth box;
	// no public Camera is created/validated with these signed internal bounds.
	box := Camera{Near: -config.Depth / 2, Far: config.Depth / 2}
	for index, in := range instances {
		vertices := make([]geom.Vec3, len(in.instance.Mesh.Vertices))
		visible := in.instance.Material.Color.A > 0
		for i, p := range in.instance.Mesh.Vertices {
			world := in.model.matrix.Apply(p)
			if !bounded(world) {
				return nil, fmt.Errorf("three: shadow instance %d transformed vertex %d outside coordinate bounds", index, i)
			}
			vertices[i] = s.view(world)
			// An upstream caster outside the depth box could block an in-box
			// receiver. Reject it rather than silently truncate that occlusion.
			if visible && vertices[i].Z < box.Near {
				return nil, fmt.Errorf("three: shadow instance %d vertex %d is upstream of shadow depth volume; enlarge/recenter Depth", index, i)
			}
		}
		for _, indices := range in.instance.Mesh.Triangles {
			a, b, c := vertices[indices[0]], vertices[indices[1]], vertices[indices[2]]
			if _, ok := unit(b.Sub(a).Cross(c.Sub(a))); !ok {
				return nil, fmt.Errorf("three: shadow instance %d transformed triangle is degenerate", index)
			}
			if !visible {
				continue
			}
			// Use canonical edge intersections for both light and observer clips;
			// opposite traversals of a shared clipped edge must not open cracks.
			var input [3]worldClipVertex
			for i, p := range [3]geom.Vec3{a, b, c} {
				input[i].p = p
			}
			polygon, count := clipWorldTriangle(input, box, frame)
			for k := 1; k+1 < count; k++ {
				a := projectVertex(polygon[0].clipVertex, frame, config.Resolution, config.Resolution)
				b := projectVertex(polygon[k].clipVertex, frame, config.Resolution, config.Resolution)
				c := projectVertex(polygon[k+1].clipVertex, frame, config.Resolution, config.Resolution)
				if err := s.rasterize(a, b, c); err != nil {
					return nil, fmt.Errorf("three: shadow instance %d: %w", index, err)
				}
			}
		}
	}
	return s, nil
}

func (s *shadowMap) rasterize(a, b, c screenVertex) error {
	n := s.config.Resolution
	for _, v := range [3]screenVertex{a, b, c} {
		if !shadowProjectionBounded(v, n, n) {
			return fmt.Errorf("shadow projection lost finite clipped bounds")
		}
	}
	area := edge(a, b, c.x, c.y)
	if area == 0 || !finite(area) {
		return nil
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	x0 := max(0, int(math.Ceil(math.Min(a.x, math.Min(b.x, c.x))-.5)))
	x1 := min(n-1, int(math.Floor(math.Max(a.x, math.Max(b.x, c.x))-.5)))
	y0 := max(0, int(math.Ceil(math.Min(a.y, math.Min(b.y, c.y))-.5)))
	y1 := min(n-1, int(math.Floor(math.Max(a.y, math.Max(b.y, c.y))-.5)))
	if x0 > x1 || y0 > y1 {
		return nil
	}
	s.rasterSamples += int64(x1-x0+1) * int64(y1-y0+1)
	if s.rasterSamples > int64(s.config.RasterSampleLimit) {
		return fmt.Errorf("shadow raster sample budget exceeded (limit %d, clipped bounding-box candidates)", s.config.RasterSampleLimit)
	}
	t0, t1, t2 := topLeft(b, c), topLeft(c, a), topLeft(a, b)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			if !covered(transparentEdge(b, c, px, py), t0) || !covered(transparentEdge(c, a, px, py), t1) || !covered(transparentEdge(a, b, px, py), t2) {
				continue
			}
			z := (edge(b, c, px, py)*a.z + edge(c, a, px, py)*b.z + edge(a, b, px, py)*c.z) / area
			if !finite(z) {
				return fmt.Errorf("nonfinite shadow depth")
			}
			at := y*n + x
			if z < s.depth[at] {
				s.depth[at] = z
			}
		}
	}
	return nil
}

// Check before float-to-int bounds conversion. Normally six-plane clipping
// keeps x/y inside the viewport with only rounding error; extreme scale ratios
// must fail explicitly if their clipped projection loses that bound.
func shadowProjectionBounded(v screenVertex, width, height int) bool {
	return finite(v.x) && finite(v.y) && finite(v.z) && finite(v.q) && v.x >= -1 && v.x <= float64(width)+1 && v.y >= -1 && v.y <= float64(height)+1
}

func (s *shadowMap) occluded(world geom.Vec3) (bool, error) {
	s.lookups++
	if s.lookups > int64(s.config.LookupLimit) {
		return false, fmt.Errorf("three: shadow lookup budget exceeded (limit %d)", s.config.LookupLimit)
	}
	p := s.view(world)
	if !p.Finite() {
		return false, fmt.Errorf("three: nonfinite shadow receiver")
	}
	if math.Abs(p.X) > s.config.Size/2 || math.Abs(p.Y) > s.config.Size/2 || math.Abs(p.Z) > s.config.Depth/2 {
		return false, nil
	}
	n := s.config.Resolution
	x := min(n-1, int(math.Floor((p.X/s.config.Size+.5)*float64(n))))
	y := min(n-1, int(math.Floor((.5-p.Y/s.config.Size)*float64(n))))
	return p.Z > s.depth[y*n+x]+s.config.Bias, nil
}
