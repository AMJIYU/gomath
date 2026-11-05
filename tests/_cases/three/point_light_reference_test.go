package three

import (
	"cmp"
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"slices"
	"testing"
)

// Independent four-tap premultiplied sampler. It reads owned source values,
// without calling the production sample, addressing or lerp functions.
func pointLightTextureReference(tex *Texture, uv UV, sampler Sampler) (float64, float64, float64, float64) {
	address := func(i, n int, repeat bool) int {
		if repeat {
			return (i%n + n) % n
		}
		return max(0, min(n-1, i))
	}
	coord := func(v float64, repeat bool) float64 {
		if repeat {
			return v - math.Floor(v)
		}
		return math.Max(0, math.Min(1, v))
	}
	x, y := coord(uv.U, sampler.AddressU == Repeat)*float64(tex.width), coord(uv.V, sampler.AddressV == Repeat)*float64(tex.height)
	var taps [4]struct {
		x, y   int
		weight float64
	}
	count := 1
	if sampler.Filter == Nearest {
		taps[0].x, taps[0].y, taps[0].weight = int(math.Floor(x)), int(math.Floor(y)), 1
	} else {
		x -= .5
		y -= .5
		ix, iy := int(math.Floor(x)), int(math.Floor(y))
		a, b := x-float64(ix), y-float64(iy)
		count = 4
		for i, w := range [4]float64{(1 - a) * (1 - b), a * (1 - b), (1 - a) * b, a * b} {
			taps[i].x, taps[i].y, taps[i].weight = ix+i%2, iy+i/2, w
		}
	}
	var red, green, blue, alpha float64
	for _, tap := range taps[:count] {
		i, j := address(tap.x, tex.width, sampler.AddressU == Repeat), address(tap.y, tex.height, sampler.AddressV == Repeat)
		c := tex.pixels[j*tex.width+i]
		a := float64(c.A) / 255 * tap.weight
		red += float64(c.R) * a
		green += float64(c.G) * a
		blue += float64(c.B) * a
		alpha += a
	}
	return red, green, blue, alpha
}

type pointLightReferenceLayer struct {
	texturedRayLayer
	instance int
	indices  Triangle
}

// Primary rays, inverse-transpose world normals and Moller-Trumbore hits use
// independent test math shared with the shadow reference. Lighting is computed
// from the actual ray intersection, not projected/interpolated screen vertices.
func pointLightRayReference(t *testing.T, cfg Config, camera Camera, instances ...Instance) *image.RGBA {
	return pointLightRayReferenceWithLighting(t, cfg, camera, func(world, n geom.Vec3) [3]float64 {
		delta := cfg.Light.Position.Sub(world)
		distanceSquared := delta.Dot(delta)
		diffuse := 0.0
		if distanceSquared > 0 {
			diffuse = math.Max(0, n.Dot(delta)/math.Sqrt(distanceSquared)) / (1 + cfg.Light.Attenuation*distanceSquared)
		}
		intensity := math.Min(1, cfg.Light.Ambient+cfg.Light.Diffuse*diffuse)
		return [3]float64{intensity, intensity, intensity}
	}, instances...)
}

// Callback substitutes only illumination, retaining independent primary rays,
// clipping, world normals, filtering and composition for the point-list checks.
func pointLightRayReferenceWithLighting(t *testing.T, cfg Config, camera Camera, lighting func(world, normal geom.Vec3) [3]float64, instances ...Instance) *image.RGBA {
	t.Helper()
	triangles := shadowReferenceTriangles(instances)
	var owners []int
	var indices []Triangle
	for owner, in := range instances {
		if in.Material.Color.A > 0 {
			for _, idx := range in.Mesh.Triangles {
				owners = append(owners, owner)
				indices = append(indices, idx)
			}
		}
	}
	normalize := func(v geom.Vec3) geom.Vec3 {
		length := math.Sqrt(v.Dot(v))
		if length == 0 {
			return geom.Vec3{}
		}
		return v.Mul(1 / length)
	}
	forward := normalize(camera.Target.Sub(camera.Position))
	right := normalize(forward.Cross(camera.Up))
	up := right.Cross(forward)
	scale := max(1, cfg.Supersampling)
	img := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
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
					var layers []pointLightReferenceLayer
					for index, tri := range triangles {
						depth, u, v, _ := shadowRayHit(origin, direction, tri.p)
						// Independent determinant arithmetic can put a mathematically
						// shared-edge hit just outside BOTH triangles. Include that
						// 1e-12 barycentric roundoff band, then deduplicate the edge.
						hit := u >= -1e-12 && v >= -1e-12 && u+v <= 1+1e-12
						if !hit || depth < camera.Near || depth > camera.Far {
							continue
						}
						if cfg.CullBackfaces && tri.p[1].Sub(tri.p[0]).Cross(tri.p[2].Sub(tri.p[0])).Dot(direction) >= 0 {
							continue
						}
						// An exactly shared indexed edge is one surface, not two alpha
						// layers. Deduplicate only same-instance shared-index edge hits.
						shared := false
						for _, old := range layers {
							if old.instance != owners[index] || math.Abs(old.depth-depth) > 1e-10 {
								continue
							}
							count := 0
							for _, a := range old.indices {
								for _, b := range indices[index] {
									if a == b {
										count++
									}
								}
							}
							if count >= 2 {
								shared = true
								break
							}
						}
						if shared {
							continue
						}
						world := origin.Add(direction.Mul(depth))
						n := normalize(tri.n[0].Mul(1 - u - v).Add(tri.n[1].Mul(u)).Add(tri.n[2].Mul(v)))
						intensity := lighting(world, n)
						base := tri.material.Color
						r, g, b, alpha := float64(base.R), float64(base.G), float64(base.B), float64(base.A)/255
						if tri.material.Texture != nil {
							uv := UV{U: (1-u-v)*tri.uv[0].U + u*tri.uv[1].U + v*tri.uv[2].U, V: (1-u-v)*tri.uv[0].V + u*tri.uv[1].V + v*tri.uv[2].V}
							tr, tg, tb, ta := pointLightTextureReference(tri.material.Texture, uv, tri.material.Sampler)
							if ta == 0 {
								continue
							}
							r *= tr / ta / 255
							g *= tg / ta / 255
							b *= tb / ta / 255
							alpha *= ta
						}
						if alpha == 0 {
							continue
						}
						shade := color.NRGBA{R: uint8(math.Round(r * intensity[0])), G: uint8(math.Round(g * intensity[1])), B: uint8(math.Round(b * intensity[2]))}
						layers = append(layers, pointLightReferenceLayer{texturedRayLayer{depth, shade, alpha}, owners[index], indices[index]})
					}
					slices.SortStableFunc(layers, func(a, b pointLightReferenceLayer) int {
						if c := cmp.Compare(a.depth, b.depth); c != 0 {
							return c
						}
						if a.alpha == 1 || b.alpha == 1 {
							return cmp.Compare(b.alpha, a.alpha)
						}
						pack := func(c color.NRGBA) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }
						if c := cmp.Compare(pack(b.rgb), pack(a.rgb)); c != 0 {
							return c
						}
						return cmp.Compare(b.alpha, a.alpha)
					})
					ordered := make([]texturedRayLayer, len(layers))
					for i, layer := range layers {
						ordered[i] = layer.texturedRayLayer
					}
					c := referenceTextureLayers(cfg.Background, ordered...)
					for i, ch := range [4]uint8{c.R, c.G, c.B, c.A} {
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
