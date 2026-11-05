package three

import (
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"slices"
	"testing"
)

type polyhedronPose struct {
	position, scale, axis geom.Vec3
	angle                 float64
}

func polyhedronRotate(p, axis geom.Vec3, angle float64) geom.Vec3 {
	a := axis.Mul(1 / math.Sqrt(axis.Dot(axis)))
	c, s := math.Cos(angle), math.Sin(angle)
	return p.Mul(c).Add(a.Cross(p).Mul(s)).Add(a.Mul(a.Dot(p) * (1 - c)))
}

func polyhedronInverse(p geom.Vec3, pose polyhedronPose, point bool) geom.Vec3 {
	if point {
		p = p.Sub(pose.position)
	}
	p = polyhedronRotate(p, pose.axis, -pose.angle)
	return geom.Vec3{X: p.X / pose.scale.X, Y: p.Y / pose.scale.Y, Z: p.Z / pose.scale.Z}
}

func polyhedronModel(p polyhedronPose) geom.Mat4 {
	return geom.Translation(p.position).Mul(geom.Rotation(p.axis, p.angle)).Mul(geom.Scaling(p.scale.X, p.scale.Y, p.scale.Z))
}

// The oracle intersects the convex halfspace interval from independently
// discovered supporting planes. It never reads production Mesh vertices,
// triangles/normals/UVs, raster edges, clipping, camera frame or model helpers.
// Independent Rodrigues math inverts actual TRS, including signed scales.
func polyhedronPlaneReference(t *testing.T, c Config, camera Camera, control polyhedronControl, edge float64, pose polyhedronPose, material Material) *image.RGBA {
	t.Helper()
	vec := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	normalize := func(v geom.Vec3) geom.Vec3 { return v.Mul(1 / math.Sqrt(v.Dot(v))) }
	type plane struct {
		normal, worldNormal    geom.Vec3
		offset                 float64
		anchor, u, v           geom.Vec3
		minU, maxU, minV, maxV float64
	}
	planes := make([]plane, len(control.Planes))
	for i, p := range control.Planes {
		n := vec(p.Normal)
		wn := normalize(polyhedronRotate(geom.Vec3{X: n.X / pose.scale.X, Y: n.Y / pose.scale.Y, Z: n.Z / pose.scale.Z}, pose.axis, pose.angle))
		var face []int
		for _, f := range control.Faces {
			if len(f) != len(p.Vertices) {
				continue
			}
			same := true
			for _, j := range f {
				same = same && slices.Contains(p.Vertices, j)
			}
			if same {
				face = f
				break
			}
		}
		if face == nil {
			t.Fatal("plane lacks pinned face")
		}
		a, b := vec(control.Vertices[face[0]]).Mul(edge), vec(control.Vertices[face[1]]).Mul(edge)
		u := normalize(b.Sub(a))
		v := n.Cross(u)
		pp := plane{normal: n, worldNormal: wn, offset: p.Offset * edge, anchor: a, u: u, v: v, minU: math.Inf(1), maxU: math.Inf(-1), minV: math.Inf(1), maxV: math.Inf(-1)}
		for _, index := range face {
			q := vec(control.Vertices[index]).Mul(edge).Sub(a)
			x, y := q.Dot(u), q.Dot(v)
			pp.minU, pp.maxU = math.Min(pp.minU, x), math.Max(pp.maxU, x)
			pp.minV, pp.maxV = math.Min(pp.minV, y), math.Max(pp.maxV, y)
		}
		planes[i] = pp
	}
	forward := normalize(camera.Target.Sub(camera.Position))
	right := normalize(forward.Cross(camera.Up))
	up := right.Cross(forward)
	scale := max(1, c.Supersampling)
	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	for y := range c.Height {
		for x := range c.Width {
			var sum [4]int
			for sy := range scale {
				for sx := range scale {
					px, py := float64(x)+(float64(sx)+.5)/float64(scale), float64(y)+(float64(sy)+.5)/float64(scale)
					rx, ry := (2*px/float64(c.Width)-1)*float64(c.Width)/float64(c.Height), 1-2*py/float64(c.Height)
					origin, direction := camera.Position, forward
					if camera.Projection == Perspective {
						direction = direction.Add(right.Mul(rx * math.Tan(camera.FOVY/2))).Add(up.Mul(ry * math.Tan(camera.FOVY/2)))
					} else {
						origin = origin.Add(right.Mul(rx * camera.OrthoHeight / 2)).Add(up.Mul(ry * camera.OrthoHeight / 2))
					}
					localOrigin, localDirection := polyhedronInverse(origin, pose, true), polyhedronInverse(direction, pose, false)
					enter, exit := math.Inf(-1), math.Inf(1)
					entryFace, exitFace := -1, -1
					miss := false
					for i, p := range planes {
						denom := p.normal.Dot(localDirection)
						distance := p.offset - p.normal.Dot(localOrigin)
						if denom == 0 {
							if distance < 0 {
								miss = true
							}
							continue
						}
						hit := distance / denom
						if denom < 0 {
							if hit > enter {
								enter, entryFace = hit, i
							}
						} else {
							if hit < exit {
								exit, exitFace = hit, i
							}
						}
					}
					var layers []texturedRayLayer
					if !miss && enter <= exit {
						for k, depth := range [2]float64{enter, exit} {
							face := entryFace
							if k == 1 {
								face = exitFace
							}
							if face < 0 || depth < camera.Near || depth > camera.Far || k == 1 && c.CullBackfaces {
								continue
							}
							p := planes[face]
							rgb := multiLightReferenceIntensity(c, origin.Add(direction.Mul(depth)), p.worldNormal)
							if c.Light.Kind != PointLightList {
								lambert := 0.
								if c.Light.Diffuse > 0 {
									lambert = math.Max(0, p.worldNormal.Dot(normalize(c.Light.Direction)))
								}
								i := math.Min(1, c.Light.Ambient+c.Light.Diffuse*lambert)
								rgb = [3]float64{i, i, i}
							}
							base := material.Color
							red, green, blue, alpha := float64(base.R), float64(base.G), float64(base.B), float64(base.A)/255
							if material.Texture != nil {
								hit := localOrigin.Add(localDirection.Mul(depth)).Sub(p.anchor)
								uv := UV{U: (hit.Dot(p.u) - p.minU) / (p.maxU - p.minU), V: 1 - (hit.Dot(p.v)-p.minV)/(p.maxV-p.minV)}
								r, g, b, a := pointLightTextureReference(material.Texture, uv, material.Sampler)
								if a == 0 {
									continue
								}
								red *= r / a / 255
								green *= g / a / 255
								blue *= b / a / 255
								alpha *= a
							}
							if alpha == 0 {
								continue
							}
							shade := color.NRGBA{R: uint8(math.Round(red * rgb[0])), G: uint8(math.Round(green * rgb[1])), B: uint8(math.Round(blue * rgb[2]))}
							layers = append(layers, texturedRayLayer{depth: depth, rgb: shade, alpha: alpha})
						}
					}
					out := referenceTextureLayers(c.Background, layers...)
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
