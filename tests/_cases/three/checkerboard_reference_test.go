package three_test

import (
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"sort"
)

type checkerReference struct {
	positions, normals []geom.Vec3
	uvs                []three.UV
	triangles          []checkerTriangle
}
type checkerTriangle struct {
	indices [3]int
	group   int
}

func checkerUnit(v geom.Vec3) geom.Vec3 { return v.Mul(1 / math.Sqrt(v.Dot(v))) }

// Expected triangles come from parameter-cell corners, never from a returned
// Mesh. Global area-weighted vertex normals are independently summed first.
func checkerReferenceGrid(u0, u1, v0, v1 float64, n, m int, point func(float64, float64) geom.Vec3) checkerReference {
	var r checkerReference
	for row := 0; row <= m; row++ {
		for col := 0; col <= n; col++ {
			a, b := float64(col)/float64(n), float64(row)/float64(m)
			r.positions = append(r.positions, point((1-a)*u0+a*u1, (1-b)*v0+b*v1))
			r.uvs = append(r.uvs, three.UV{U: a, V: 1 - b})
		}
	}
	r.normals = make([]geom.Vec3, len(r.positions))
	for row := 0; row < m; row++ {
		for col := 0; col < n; col++ {
			corners := [4]int{row*(n+1) + col, row*(n+1) + col + 1, (row+1)*(n+1) + col + 1, (row+1)*(n+1) + col}
			for _, half := range [][3]int{{0, 1, 2}, {0, 2, 3}} {
				t := checkerTriangle{indices: [3]int{corners[half[0]], corners[half[1]], corners[half[2]]}, group: (col + row) & 1}
				r.triangles = append(r.triangles, t)
				a, b, c := r.positions[t.indices[0]], r.positions[t.indices[1]], r.positions[t.indices[2]]
				d, e := b.Sub(a), c.Sub(a)
				normal := geom.Vec3{X: d.Y*e.Z - d.Z*e.Y, Y: d.Z*e.X - d.X*e.Z, Z: d.X*e.Y - d.Y*e.X}
				for _, id := range t.indices {
					r.normals[id] = r.normals[id].Add(normal)
				}
			}
		}
	}
	for i, v := range r.normals {
		r.normals[i] = checkerUnit(v)
	}
	return r
}

type checkerStyle struct {
	material three.Material
	source   *image.NRGBA
}
type checkerWorldTriangle struct {
	p, n  [3]geom.Vec3
	uv    [3]three.UV
	style checkerStyle
}

// Explicit affine point multiply and cofactor normal inverse. This does not
// call the production model preparation, Apply or normal-matrix helpers.
func checkerWorld(r checkerReference, model geom.Mat4, styles [2]checkerStyle) []checkerWorldTriangle {
	if model == (geom.Mat4{}) {
		model = geom.Identity()
	}
	c0, c1, c2 := geom.Vec3{X: model[0], Y: model[4], Z: model[8]}, geom.Vec3{X: model[1], Y: model[5], Z: model[9]}, geom.Vec3{X: model[2], Y: model[6], Z: model[10]}
	det := c0.Dot(c1.Cross(c2))
	var result []checkerWorldTriangle
	for _, t := range r.triangles {
		ids := t.indices
		if det < 0 {
			ids[1], ids[2] = ids[2], ids[1]
		}
		tri := checkerWorldTriangle{style: styles[t.group]}
		for j, id := range ids {
			p, n := r.positions[id], r.normals[id]
			tri.p[j] = geom.Vec3{X: model[0]*p.X + model[1]*p.Y + model[2]*p.Z + model[3], Y: model[4]*p.X + model[5]*p.Y + model[6]*p.Z + model[7], Z: model[8]*p.X + model[9]*p.Y + model[10]*p.Z + model[11]}
			tri.n[j] = checkerUnit(c1.Cross(c2).Mul(n.X / det).Add(c2.Cross(c0).Mul(n.Y / det)).Add(c0.Cross(c1).Mul(n.Z / det)))
			tri.uv[j] = r.uvs[id]
		}
		result = append(result, tri)
	}
	return result
}

func checkerTexture(src *image.NRGBA, uv three.UV, s three.Sampler) (rgb [3]float64, alpha float64) {
	if src == nil {
		return [3]float64{255, 255, 255}, 1
	}
	coord := func(x float64, repeat bool) float64 {
		if repeat {
			return x - math.Floor(x)
		}
		return math.Max(0, math.Min(1, x))
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	x, y := coord(uv.U, s.AddressU == three.Repeat)*float64(w), coord(uv.V, s.AddressV == three.Repeat)*float64(h)
	type tap struct {
		x, y   int
		weight float64
	}
	taps := []tap{{int(math.Floor(x)), int(math.Floor(y)), 1}}
	if s.Filter == three.Bilinear {
		x -= .5
		y -= .5
		a, b := int(math.Floor(x)), int(math.Floor(y))
		fx, fy := x-float64(a), y-float64(b)
		taps = []tap{{a, b, (1 - fx) * (1 - fy)}, {a + 1, b, fx * (1 - fy)}, {a, b + 1, (1 - fx) * fy}, {a + 1, b + 1, fx * fy}}
	}
	address := func(i, n int, repeat bool) int {
		if repeat {
			return (i%n + n) % n
		}
		return max(0, min(n-1, i))
	}
	for _, tap := range taps {
		c := src.NRGBAAt(address(tap.x, w, s.AddressU == three.Repeat), address(tap.y, h, s.AddressV == three.Repeat))
		a := float64(c.A) / 255 * tap.weight
		alpha += a
		for j, ch := range [3]uint8{c.R, c.G, c.B} {
			rgb[j] += float64(ch) * a
		}
	}
	if alpha > 0 {
		for j := range rgb {
			rgb[j] /= alpha
		}
	}
	return
}

func checkerIntensity(c three.Config, p, n geom.Vec3) [3]float64 {
	v := [3]float64{c.Light.Ambient, c.Light.Ambient, c.Light.Ambient}
	if c.Light.Kind == three.PointLightList {
		for _, s := range c.PointLights {
			d := s.Position.Sub(p)
			d2 := d.Dot(d)
			if d2 == 0 {
				continue
			}
			energy := s.Diffuse * math.Max(0, n.Dot(d)/math.Sqrt(d2)) / (1 + s.Attenuation*d2)
			for j := range v {
				v[j] += s.RGB[j] * energy
			}
		}
	} else if c.Light.Diffuse > 0 {
		d := checkerUnit(c.Light.Direction)
		l := c.Light.Diffuse * math.Max(0, n.Dot(d))
		for j := range v {
			v[j] += l
		}
	}
	for j := range v {
		v[j] = math.Min(1, v[j])
	}
	return v
}

// Independent camera rays, Moller-Trumbore intersection and front-to-back
// transmittance. Render uses projected edges, z buffers and reverse source-over.
func checkerRayReference(c three.Config, camera three.Camera, triangles []checkerWorldTriangle) *image.RGBA {
	forward := checkerUnit(camera.Target.Sub(camera.Position))
	right := checkerUnit(forward.Cross(camera.Up))
	up := right.Cross(forward)
	ssaa := max(1, c.Supersampling)
	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	type layer struct {
		depth float64
		rgb   [3]float64
		alpha float64
	}
	for y := range c.Height {
		for x := range c.Width {
			var sum [4]int
			for sy := range ssaa {
				for sx := range ssaa {
					px, py := float64(x)+(float64(sx)+.5)/float64(ssaa), float64(y)+(float64(sy)+.5)/float64(ssaa)
					rx, ry := (2*px/float64(c.Width)-1)*float64(c.Width)/float64(c.Height), 1-2*py/float64(c.Height)
					origin, direction := camera.Position, forward
					if camera.Projection == three.Perspective {
						direction = direction.Add(right.Mul(rx * math.Tan(camera.FOVY/2))).Add(up.Mul(ry * math.Tan(camera.FOVY/2)))
					} else {
						origin = origin.Add(right.Mul(rx * camera.OrthoHeight / 2)).Add(up.Mul(ry * camera.OrthoHeight / 2))
					}
					var layers []layer
					for _, tri := range triangles {
						e1, e2 := tri.p[1].Sub(tri.p[0]), tri.p[2].Sub(tri.p[0])
						h := direction.Cross(e2)
						det := e1.Dot(h)
						if math.Abs(det) < 1e-14 || c.CullBackfaces && det <= 0 {
							continue
						}
						s := origin.Sub(tri.p[0])
						u := s.Dot(h) / det
						q := s.Cross(e1)
						v := direction.Dot(q) / det
						depth := e2.Dot(q) / det
						if u < 0 || v < 0 || u+v > 1 || depth < camera.Near || depth > camera.Far {
							continue
						}
						p := origin.Add(direction.Mul(depth))
						normal := checkerUnit(tri.n[0].Mul(1 - u - v).Add(tri.n[1].Mul(u)).Add(tri.n[2].Mul(v)))
						uv := three.UV{U: (1-u-v)*tri.uv[0].U + u*tri.uv[1].U + v*tri.uv[2].U, V: (1-u-v)*tri.uv[0].V + u*tri.uv[1].V + v*tri.uv[2].V}
						rgb, a := checkerTexture(tri.style.source, uv, tri.style.material.Sampler)
						alpha := a * float64(tri.style.material.Color.A) / 255
						if alpha == 0 {
							continue
						}
						intensity := checkerIntensity(c, p, normal)
						for j, ch := range [3]uint8{tri.style.material.Color.R, tri.style.material.Color.G, tri.style.material.Color.B} {
							rgb[j] = math.Round(float64(ch) * rgb[j] / 255 * intensity[j])
						}
						layers = append(layers, layer{depth, rgb, alpha})
					}
					sort.Slice(layers, func(i, j int) bool { return layers[i].depth < layers[j].depth })
					var value [4]float64
					remaining := 1.0
					for _, l := range layers {
						weight := remaining * l.alpha
						for j := range l.rgb {
							value[j] += weight * l.rgb[j]
						}
						value[3] += 255 * weight
						remaining *= 1 - l.alpha
					}
					bg := color.RGBAModel.Convert(c.Background).(color.RGBA)
					for j, ch := range [4]uint8{bg.R, bg.G, bg.B, bg.A} {
						sum[j] += int(math.Round(value[j] + float64(ch)*remaining))
					}
				}
			}
			count := ssaa * ssaa
			img.SetRGBA(x, y, color.RGBA{R: uint8((sum[0] + count/2) / count), G: uint8((sum[1] + count/2) / count), B: uint8((sum[2] + count/2) / count), A: uint8((sum[3] + count/2) / count)})
		}
	}
	return img
}
