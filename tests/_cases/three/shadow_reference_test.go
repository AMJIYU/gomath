package three

import (
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"testing"
)

// Independent reference: primary rays and light-texel rays use analytic
// Moller-Trumbore intersections. No production camera frame, clipping,
// projection, raster edges, shadowMap, or receiver interpolation is used.
type shadowRayTriangle struct {
	p        [3]geom.Vec3
	n        [3]geom.Vec3
	uv       [3]UV
	material Material
}

func shadowRayHit(origin, direction geom.Vec3, p [3]geom.Vec3) (depth, u, v float64, ok bool) {
	e1, e2 := p[1].Sub(p[0]), p[2].Sub(p[0])
	h := direction.Cross(e2)
	det := e1.Dot(h)
	if math.Abs(det) < 1e-14 {
		return
	}
	s := origin.Sub(p[0])
	u = s.Dot(h) / det
	q := s.Cross(e1)
	v = direction.Dot(q) / det
	depth = e2.Dot(q) / det
	ok = u >= 0 && v >= 0 && u+v <= 1
	return
}

func shadowReferenceTriangles(instances []Instance) []shadowRayTriangle {
	var result []shadowRayTriangle
	normalize := func(p geom.Vec3) geom.Vec3 { return p.Mul(1 / p.Norm()) }
	for _, in := range instances {
		if in.Material.Color.A == 0 {
			continue
		}
		m := in.Model
		if m == (geom.Mat4{}) {
			m = geom.Identity()
		}
		c0, c1, c2 := geom.Vec3{X: m[0], Y: m[4], Z: m[8]}, geom.Vec3{X: m[1], Y: m[5], Z: m[9]}, geom.Vec3{X: m[2], Y: m[6], Z: m[10]}
		det := c0.Dot(c1.Cross(c2))
		for _, indices := range in.Mesh.Triangles {
			if det < 0 {
				indices[1], indices[2] = indices[2], indices[1]
			}
			tri := shadowRayTriangle{material: in.Material}
			for j, i := range indices {
				tri.p[j] = m.Apply(in.Mesh.Vertices[i])
				if len(in.Mesh.UVs) > 0 {
					tri.uv[j] = in.Mesh.UVs[i]
				}
				if len(in.Mesh.Normals) > 0 {
					n := in.Mesh.Normals[i]
					tri.n[j] = normalize(c1.Cross(c2).Mul(n.X / det).Add(c2.Cross(c0).Mul(n.Y / det)).Add(c0.Cross(c1).Mul(n.Z / det)))
				}
			}
			if len(in.Mesh.Normals) == 0 {
				n := normalize(tri.p[1].Sub(tri.p[0]).Cross(tri.p[2].Sub(tri.p[0])))
				tri.n = [3]geom.Vec3{n, n, n}
			}
			result = append(result, tri)
		}
	}
	return result
}

func shadowRayReference(t *testing.T, cfg Config, camera Camera, instances ...Instance) *image.RGBA {
	t.Helper()
	normalize := func(p geom.Vec3) geom.Vec3 { return p.Mul(1 / p.Norm()) }
	triangles := shadowReferenceTriangles(instances)
	forward := normalize(camera.Target.Sub(camera.Position))
	right := normalize(forward.Cross(camera.Up))
	up := right.Cross(forward)
	light := normalize(cfg.Light.Direction)
	lf := light.Mul(-1)
	axis := geom.Vec3{Z: 1}
	if math.Abs(lf.Z) > .99 {
		axis = geom.Vec3{Y: 1}
	}
	lr := normalize(lf.Cross(axis))
	lu := lr.Cross(lf)
	s := cfg.Shadow
	cache := make(map[int]float64)
	blocked := func(world geom.Vec3) bool {
		p := world.Sub(s.Center)
		x, y, z := p.Dot(lr), p.Dot(lu), p.Dot(lf)
		if math.Abs(x) > s.Size/2 || math.Abs(y) > s.Size/2 || math.Abs(z) > s.Depth/2 {
			return false
		}
		i := min(s.Resolution-1, int(math.Floor((x/s.Size+.5)*float64(s.Resolution))))
		j := min(s.Resolution-1, int(math.Floor((.5-y/s.Size)*float64(s.Resolution))))
		key := j*s.Resolution + i
		depth, ok := cache[key]
		if !ok {
			origin := s.Center.Add(lr.Mul(((float64(i)+.5)/float64(s.Resolution) - .5) * s.Size)).Add(lu.Mul((.5 - (float64(j)+.5)/float64(s.Resolution)) * s.Size)).Sub(lf.Mul(s.Depth / 2))
			depth = math.Inf(1)
			for _, tri := range triangles {
				d, _, _, hit := shadowRayHit(origin, lf, tri.p)
				if hit && d >= 0 && d <= s.Depth && d < depth {
					depth = d
				}
			}
			cache[key] = depth
		}
		return z+s.Depth/2 > depth+s.Bias
	}
	img := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	scale := max(1, cfg.Supersampling)
	for y := range cfg.Height {
		for x := range cfg.Width {
			var sum [4]int
			for sy := range scale {
				for sx := range scale {
					px, py := float64(x)+(float64(sx)+.5)/float64(scale), float64(y)+(float64(sy)+.5)/float64(scale)
					rx, ry := (2*px/float64(cfg.Width)-1)*float64(cfg.Width)/float64(cfg.Height), 1-2*py/float64(cfg.Height)
					origin, direction := camera.Position, forward
					if camera.Projection == Perspective {
						direction = direction.Add(right.Mul(rx * math.Tan(camera.FOVY/2))).Add(up.Mul(ry * math.Tan(camera.FOVY/2)))
					} else {
						origin = origin.Add(right.Mul(rx * camera.OrthoHeight / 2)).Add(up.Mul(ry * camera.OrthoHeight / 2))
					}
					nearest := math.Inf(1)
					out := color.RGBAModel.Convert(cfg.Background).(color.RGBA)
					for _, tri := range triangles {
						d, u, v, hit := shadowRayHit(origin, direction, tri.p)
						if !hit || d < camera.Near || d > camera.Far || d >= nearest {
							continue
						}
						if cfg.CullBackfaces && tri.p[1].Sub(tri.p[0]).Cross(tri.p[2].Sub(tri.p[0])).Dot(direction) >= 0 {
							continue
						}
						nearest = d
						n := normalize(tri.n[0].Mul(1 - u - v).Add(tri.n[1].Mul(u)).Add(tri.n[2].Mul(v)))
						lambert := math.Max(0, n.Dot(light))
						if lambert > 0 && blocked(origin.Add(direction.Mul(d))) {
							lambert = 0
						}
						intensity := math.Min(1, cfg.Light.Ambient+cfg.Light.Diffuse*lambert)
						base := tri.material.Color
						r, g, b := float64(base.R), float64(base.G), float64(base.B)
						if tri.material.Texture != nil {
							uv := UV{U: (1-u-v)*tri.uv[0].U + u*tri.uv[1].U + v*tri.uv[2].U, V: (1-u-v)*tri.uv[0].V + u*tri.uv[1].V + v*tri.uv[2].V}
							// Public Sample rounds to eight bits. The renderer retains
							// bilinear precision until the final tint/lighting round.
							texel, err := tri.material.Texture.Sample(uv, tri.material.Sampler)
							if err != nil {
								t.Fatal(err)
							}
							r *= float64(texel.R) / 255
							g *= float64(texel.G) / 255
							b *= float64(texel.B) / 255
						}
						out = color.RGBA{R: uint8(math.Round(r * intensity)), G: uint8(math.Round(g * intensity)), B: uint8(math.Round(b * intensity)), A: 255}
					}
					for i, ch := range [4]uint8{out.R, out.G, out.B, out.A} {
						sum[i] += int(ch)
					}
				}
			}
			n := scale * scale
			img.SetRGBA(x, y, color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)})
		}
	}
	return img
}
