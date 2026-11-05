package scene3d

import (
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"slices"
)

// Independent vector Rodrigues construction: no geom.Rotation, matrix blend,
// production pose/keyframe lookup, camera projection or raster helpers.
func referenceTransform(v, axis geom.Vec3, pose Pose) geom.Vec3 {
	n := math.Sqrt(axis.Dot(axis))
	if n == 0 {
		axis, n = geom.Vec3{Z: 1}, 1
	}
	a := axis.Mul(1 / n)
	v = geom.Vec3{X: v.X * pose.Scale.X, Y: v.Y * pose.Scale.Y, Z: v.Z * pose.Scale.Z}
	c, s := math.Cos(pose.Angle), math.Sin(pose.Angle)
	return v.Mul(c).Add(a.Cross(v).Mul(s)).Add(a.Mul(a.Dot(v) * (1 - c))).Add(pose.Position)
}

func referenceMatrix(axis geom.Vec3, pose Pose) geom.Mat4 {
	o := referenceTransform(geom.Vec3{}, axis, pose)
	x := referenceTransform(geom.Vec3{X: 1}, axis, pose).Sub(o)
	y := referenceTransform(geom.Vec3{Y: 1}, axis, pose).Sub(o)
	z := referenceTransform(geom.Vec3{Z: 1}, axis, pose).Sub(o)
	return geom.Mat4{x.X, y.X, z.X, o.X, x.Y, y.Y, z.Y, o.Y, x.Z, y.Z, z.Z, o.Z, 0, 0, 0, 1}
}

func referenceRay(origin, direction geom.Vec3, p [3]geom.Vec3) (depth, u, v float64, hit bool) {
	e1, e2 := p[1].Sub(p[0]), p[2].Sub(p[0])
	h := direction.Cross(e2)
	det := e1.Dot(h)
	if math.Abs(det) < 1e-12 {
		return
	}
	s := origin.Sub(p[0])
	u = s.Dot(h) / det
	q := s.Cross(e1)
	v, depth = direction.Dot(q)/det, e2.Dot(q)/det
	hit = u >= -1e-12 && v >= -1e-12 && u+v <= 1+1e-12
	return
}

// Separate source-image sampler, including premultiplied four-tap filtering.
// It never calls three.Texture.Sample, including for reference address modes.
func referenceTexture(src *image.NRGBA, uv three.UV, sampler three.Sampler) (r, g, b, a float64) {
	coordinate := func(v float64, address three.TextureAddress) float64 {
		if address == three.Repeat {
			return v - math.Floor(v)
		}
		return math.Max(0, math.Min(1, v))
	}
	address := func(i, n int, mode three.TextureAddress) int {
		if mode == three.Repeat {
			return (i%n + n) % n
		}
		return max(0, min(n-1, i))
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	x, y := coordinate(uv.U, sampler.AddressU)*float64(w), coordinate(uv.V, sampler.AddressV)*float64(h)
	var xs, ys [4]int
	weights, count := [4]float64{1}, 1
	if sampler.Filter == three.Nearest {
		xs[0], ys[0] = int(math.Floor(x)), int(math.Floor(y))
	} else {
		x, y = x-.5, y-.5
		i, j := int(math.Floor(x)), int(math.Floor(y))
		u, v := x-float64(i), y-float64(j)
		weights, count = [4]float64{(1 - u) * (1 - v), u * (1 - v), (1 - u) * v, u * v}, 4
		for k := range 4 {
			xs[k], ys[k] = i+k%2, j+k/2
		}
	}
	for k := range count {
		c := src.NRGBAAt(address(xs[k], w, sampler.AddressU), address(ys[k], h, sampler.AddressV))
		weight := weights[k] * float64(c.A) / 255
		r, g, b, a = r+float64(c.R)*weight, g+float64(c.G)*weight, b+float64(c.B)*weight, a+weight
	}
	return
}

type referenceInstance struct {
	mesh     *three.Mesh
	axis     geom.Vec3
	pose     Pose
	material three.Material
	texture  *image.NRGBA
}

type rayLayer struct {
	depth, r, g, b, alpha float64
}

// Ambient-only independent ray reference. It covers real triangle intersections,
// UV barycentrics, near/far, opaque depth, alpha sorting and each SSAA sample.
func referencePixels(cfg Config, camera three.Camera, instances ...referenceInstance) *image.RGBA {
	return referenceMixedPixels(cfg, camera, nil, instances...)
}

type referencePoint struct {
	position geom.Vec3
	color    color.NRGBA
	radius   float64
}

func referenceMixedPixels(cfg Config, camera three.Camera, points []referencePoint, instances ...referenceInstance) *image.RGBA {
	normalize := func(v geom.Vec3) geom.Vec3 { return v.Mul(1 / math.Sqrt(v.Dot(v))) }
	f := normalize(camera.Target.Sub(camera.Position))
	r := normalize(f.Cross(camera.Up))
	u := r.Cross(f)
	w, h, scale := cfg.Renderer.Width, cfg.Renderer.Height, max(1, cfg.Renderer.Supersampling)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBAModel.Convert(cfg.Renderer.Background).(color.RGBA)
	for y := range h {
		for x := range w {
			var sums [4]int
			for sy := range scale {
				for sx := range scale {
					nx := (2*(float64(x)+(float64(sx)+.5)/float64(scale))/float64(w) - 1) * float64(w) / float64(h)
					ny := 1 - 2*(float64(y)+(float64(sy)+.5)/float64(scale))/float64(h)
					origin, direction := camera.Position, f
					if camera.Projection == three.Perspective {
						direction = f.Add(r.Mul(nx * math.Tan(camera.FOVY/2))).Add(u.Mul(ny * math.Tan(camera.FOVY/2)))
					} else {
						origin = origin.Add(r.Mul(nx * camera.OrthoHeight / 2)).Add(u.Mul(ny * camera.OrthoHeight / 2))
					}
					var layers []rayLayer
					for _, in := range instances {
						for _, tri := range in.mesh.Triangles {
							var p [3]geom.Vec3
							for k, index := range tri {
								p[k] = referenceTransform(in.mesh.Vertices[index], in.axis, in.pose)
							}
							depth, u, v, hit := referenceRay(origin, direction, p)
							if !hit || depth < camera.Near || depth > camera.Far {
								continue
							}
							base := in.material.Color
							red, green, blue, alpha := float64(base.R), float64(base.G), float64(base.B), float64(base.A)/255
							if in.texture != nil {
								a, b, c := in.mesh.UVs[tri[0]], in.mesh.UVs[tri[1]], in.mesh.UVs[tri[2]]
								uv := three.UV{U: a.U*(1-u-v) + b.U*u + c.U*v, V: a.V*(1-u-v) + b.V*u + c.V*v}
								tr, tg, tb, ta := referenceTexture(in.texture, uv, in.material.Sampler)
								if ta == 0 {
									continue
								}
								red, green, blue, alpha = red*tr/ta/255, green*tg/ta/255, blue*tb/ta/255, alpha*ta
							}
							if alpha > 0 {
								layers = append(layers, rayLayer{depth, math.Round(red), math.Round(green), math.Round(blue), alpha})
							}
						}
					}
					// Independent point projection/coverage in output-pixel units;
					// no three cloud projection, bounds, sample or sort helpers.
					for _, p := range points {
						v := p.position.Sub(camera.Position)
						depth := v.Dot(f)
						if depth < camera.Near || depth > camera.Far || p.color.A == 0 {
							continue
						}
						span := camera.OrthoHeight / 2
						if camera.Projection == three.Perspective {
							span = depth * math.Tan(camera.FOVY/2)
						}
						cx := float64(w)/2 + v.Dot(r)/span*float64(h)/2
						cy := float64(h)/2 - v.Dot(u)/span*float64(h)/2
						dx := float64(x) + (float64(sx)+.5)/float64(scale) - cx
						dy := float64(y) + (float64(sy)+.5)/float64(scale) - cy
						if dx*dx+dy*dy <= p.radius*p.radius {
							c := p.color
							layers = append(layers, rayLayer{depth, float64(c.R), float64(c.G), float64(c.B), float64(c.A) / 255})
						}
					}
					slices.SortStableFunc(layers, func(a, b rayLayer) int {
						if a.depth < b.depth {
							return -1
						}
						if a.depth > b.depth {
							return 1
						}
						return 0
					})
					remaining := 1.0
					var out [4]float64
					for _, layer := range layers {
						weight := remaining * layer.alpha
						out[0] += layer.r * weight
						out[1] += layer.g * weight
						out[2] += layer.b * weight
						out[3] += 255 * weight
						remaining *= 1 - layer.alpha
					}
					for k, c := range [4]uint8{bg.R, bg.G, bg.B, bg.A} {
						sums[k] += int(math.Round(out[k] + float64(c)*remaining))
					}
				}
			}
			n := scale * scale
			img.SetRGBA(x, y, color.RGBA{R: uint8((sums[0] + n/2) / n), G: uint8((sums[1] + n/2) / n), B: uint8((sums[2] + n/2) / n), A: uint8((sums[3] + n/2) / n)})
		}
	}
	return img
}
