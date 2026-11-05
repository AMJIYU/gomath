package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"slices"
)

const (
	// MaxPoints bounds both one cloud and the sum across a mixed frame.
	MaxPoints      = 262144
	MaxPointClouds = 1024
	// Radii are in output pixels, independent of depth, projection and SSAA.
	DefaultPointRadius = 1.0
	MinPointRadius     = 1.0 / 16
	MaxPointRadius     = 64.0
	// PointSampleLimit counts clipped bounding-box sample candidates, once per
	// visible nonzero-alpha point, before allocating image/depth buffers. It is
	// independent of the transparent fragment budget and includes opaque points.
	DefaultPointSampleLimit = 16 * 1024 * 1024
	MaxPointSamples         = 64 * 1024 * 1024
)

// PointCloud owns immutable world positions, unassociated NRGBA colors and
// optional output-pixel radii. Its private arrays never alias constructor inputs
// or accessor results. Share a completed cloud across renderers and goroutines.
// The zero value and nil pointer are invalid for rendering; use nil cloud lists
// for a scene with no points. Clouds have no mesh, normals or material texture.
type PointCloud struct {
	positions []geom.Vec3
	colors    []color.NRGBA
	radii     []float64
}

// NewPointCloud synchronously validates and copies 1..MaxPoints world positions
// and exactly as many colors. radii must be nil (radius 1 output pixel) or have
// one finite radius in [1/16,64] per position. Empty nonnil radii are not defaults.
// The caller must not mutate source slices during this synchronous copy.
func NewPointCloud(positions []geom.Vec3, colors []color.NRGBA, radii []float64) (*PointCloud, error) {
	if len(positions) < 1 || len(positions) > MaxPoints || len(colors) != len(positions) || radii != nil && len(radii) != len(positions) {
		return nil, fmt.Errorf("three: point cloud needs 1..%d positions, matching colors and nil or matching radii", MaxPoints)
	}
	for i, p := range positions {
		if !bounded(p) {
			return nil, fmt.Errorf("three: point %d is nonfinite or outside coordinate bounds", i)
		}
		if radii != nil && (!finite(radii[i]) || radii[i] < MinPointRadius || radii[i] > MaxPointRadius) {
			return nil, fmt.Errorf("three: point %d radius must be finite in [%g,%g] output pixels", i, MinPointRadius, MaxPointRadius)
		}
	}
	return &PointCloud{positions: slices.Clone(positions), colors: slices.Clone(colors), radii: slices.Clone(radii)}, nil
}

// Len returns zero for nil/zero-value clouds, without exposing owned storage.
func (p *PointCloud) Len() int {
	if p == nil {
		return 0
	}
	return len(p.positions)
}

// Point returns position, color and effective output-pixel radius by value.
func (p *PointCloud) Point(index int) (geom.Vec3, color.NRGBA, float64, error) {
	if !p.valid() || index < 0 || index >= len(p.positions) {
		return geom.Vec3{}, color.NRGBA{}, 0, fmt.Errorf("three: invalid cloud or point index %d", index)
	}
	radius := DefaultPointRadius
	if p.radii != nil {
		radius = p.radii[index]
	}
	return p.positions[index], p.colors[index], radius, nil
}

func (p *PointCloud) valid() bool {
	return p != nil && len(p.positions) > 0 && len(p.positions) <= MaxPoints && len(p.colors) == len(p.positions) && (p.radii == nil || len(p.radii) == len(p.positions))
}

// RenderWithPoints shares a camera, opaque z-buffer, transparent fragment budget
// and per-sample depth sort between circular point sprites and triangle meshes.
// Near/far clip point centers inclusively. Viewport clipping keeps visible parts
// of disks whose centers lie outside the image. Points are unlit, camera-facing
// disks at constant center depth, not spheres or reconstructed surfaces.
// Exact opaque mesh/point depth ties keep the mesh; opaque point ties choose
// the smallest packed RGB. Transparent ties use the existing fragment tie rule.
func (r *Renderer) RenderWithPoints(camera Camera, clouds []*PointCloud, instances ...Instance) (*image.RGBA, error) {
	return r.render(camera, clouds, instances)
}

// At most MaxPoints records (at most 48 bytes each), only for mixed renders.
// Bounds are clipped before conversion, so uint16 covers all internal pixels.
type rasterPoint struct {
	x, y, depth, radiusSquared float64
	x0, x1, y0, y1             uint16
	rgba                       uint32
}

func (r *Renderer) preparePoints(camera Camera, f cameraFrame, clouds []*PointCloud) ([]rasterPoint, bool, error) {
	if len(clouds) == 0 {
		return nil, false, nil
	}
	if len(clouds) > MaxPointClouds {
		return nil, false, fmt.Errorf("three: too many point clouds (maximum %d)", MaxPointClouds)
	}
	total := 0
	for i, cloud := range clouds {
		if !cloud.valid() {
			return nil, false, fmt.Errorf("three: invalid point cloud %d", i)
		}
		if cloud.Len() > MaxPoints-total {
			return nil, false, fmt.Errorf("three: frame point count exceeds %d", MaxPoints)
		}
		total += cloud.Len()
	}
	points := make([]rasterPoint, 0, total)
	work := int64(0)
	hasOpaque, hasTransparency := false, false
	w, h, scale := r.sampling.width, r.sampling.height, float64(r.sampling.scale)
	for _, cloud := range clouds {
		for i, p := range cloud.positions {
			c := cloud.colors[i]
			if c.A == 0 {
				continue
			}
			v := f.view(camera, p)
			if v.Z < camera.Near || v.Z > camera.Far {
				continue
			}
			x, y := v.X/f.sx, v.Y/f.sy
			if f.perspective {
				x /= v.Z
				y /= v.Z
			}
			x, y = (x+1)*float64(w)/2, (1-y)*float64(h)/2
			radius := DefaultPointRadius
			if cloud.radii != nil {
				radius = cloud.radii[i]
			}
			radius *= scale
			// Keep these as floats until after clipping and empty-range checks:
			// an offscreen center can be far outside the native int range.
			x0, x1 := math.Max(0, math.Ceil(x-radius-.5)), math.Min(float64(w-1), math.Floor(x+radius-.5))
			y0, y1 := math.Max(0, math.Ceil(y-radius-.5)), math.Min(float64(h-1), math.Floor(y+radius-.5))
			if x0 > x1 || y0 > y1 {
				continue
			}
			work += int64(x1-x0+1) * int64(y1-y0+1)
			if work > int64(r.config.PointSampleLimit) {
				return nil, false, fmt.Errorf("three: point sample budget exceeded (limit %d, clipped bounding-box candidates)", r.config.PointSampleLimit)
			}
			rgba := uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A)
			points = append(points, rasterPoint{x, y, v.Z, radius * radius, uint16(x0), uint16(x1), uint16(y0), uint16(y1), rgba})
			hasOpaque = hasOpaque || c.A == 255
			hasTransparency = hasTransparency || c.A < 255
		}
	}
	if hasOpaque {
		// Sorted traversal gives deterministic opaque point ties with a strict
		// depth test, without a second per-sample ownership buffer. Meshes run
		// first and therefore retain existing exact-depth priority over points.
		slices.SortFunc(points, func(a, b rasterPoint) int {
			if a.rgba < b.rgba {
				return -1
			}
			if a.rgba > b.rgba {
				return 1
			}
			return 0
		})
	}
	return points, hasTransparency, nil
}

func rasterizePoints(img *image.RGBA, depth []float64, points []rasterPoint, fragments *transparentBuffer) error {
	width := img.Bounds().Dx()
	for _, p := range points {
		opaque := p.rgba&255 == 255
		if opaque != (fragments == nil) {
			continue
		}
		for y := int(p.y0); y <= int(p.y1); y++ {
			dy := float64(y) + .5 - p.y
			for x := int(p.x0); x <= int(p.x1); x++ {
				i := y*width + x
				dx := float64(x) + .5 - p.x
				if p.depth >= depth[i] || dx*dx+dy*dy > p.radiusSquared {
					continue
				}
				if opaque {
					at := y*img.Stride + x*4
					img.Pix[at], img.Pix[at+1], img.Pix[at+2], img.Pix[at+3] = uint8(p.rgba>>24), uint8(p.rgba>>16), uint8(p.rgba>>8), 255
					depth[i] = p.depth
				} else if err := fragments.add(i, p.depth, p.rgba); err != nil {
					return fmt.Errorf("three: point cloud: %w", err)
				}
			}
		}
	}
	return nil
}
