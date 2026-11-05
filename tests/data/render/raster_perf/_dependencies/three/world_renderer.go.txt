package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"math"
)

// World attributes exist only on the opt-in point-light/shadow paths. The
// original directional untextured and textured vertex records remain unchanged.
type worldClipVertex struct {
	texturedClipVertex
	world geom.Vec3
}

type worldScreenVertex struct {
	texturedScreenVertex
	world geom.Vec3
}

func (r *Renderer) renderWorldInstance(img *image.RGBA, depth []float64, camera Camera, f cameraFrame, prepared preparedInstance, index int, fragments *transparentBuffer, shadow *shadowMap, lightWork *pointLightWork) error {
	in, model := prepared.instance, prepared.model
	world := make([]geom.Vec3, len(in.Mesh.Vertices))
	view := make([]geom.Vec3, len(world))
	for i, p := range in.Mesh.Vertices {
		world[i] = model.matrix.Apply(p)
		if !bounded(world[i]) {
			return fmt.Errorf("three: instance %d transformed vertex %d outside coordinate bounds", index, i)
		}
		view[i] = f.view(camera, world[i])
	}
	var normals []geom.Vec3
	if len(in.Mesh.Normals) > 0 {
		normals = make([]geom.Vec3, len(world))
		for i, n := range in.Mesh.Normals {
			var ok bool
			normals[i], ok = model.transformNormal(n)
			if !ok {
				return fmt.Errorf("three: instance %d transformed normal %d is invalid", index, i)
			}
		}
	}
	for _, indices := range in.Mesh.Triangles {
		if model.reflected {
			indices[1], indices[2] = indices[2], indices[1]
		}
		face, ok := unit(world[indices[1]].Sub(world[indices[0]]).Cross(world[indices[2]].Sub(world[indices[0]])))
		if !ok {
			return fmt.Errorf("three: instance %d transformed triangle is degenerate", index)
		}
		if in.Material.Color.A == 0 {
			continue
		}
		var input [3]worldClipVertex
		for k, i := range indices {
			input[k] = worldClipVertex{world: world[i], texturedClipVertex: texturedClipVertex{clipVertex: clipVertex{p: view[i], n: face}}}
			if normals != nil {
				input[k].n = normals[i]
			}
			if in.Material.Texture != nil {
				input[k].uv = in.Mesh.UVs[i]
			}
		}
		polygon, count := clipWorldTriangle(input, camera, f)
		project := func(v worldClipVertex) worldScreenVertex {
			return worldScreenVertex{world: v.world, texturedScreenVertex: projectTexturedVertex(v.texturedClipVertex, f, img.Bounds().Dx(), img.Bounds().Dy())}
		}
		for k := 1; k+1 < count; k++ {
			if err := r.rasterizeWorld(img, depth, project(polygon[0]), project(polygon[k]), project(polygon[k+1]), in.Material, f.perspective, fragments, shadow, lightWork); err != nil {
				return fmt.Errorf("three: instance %d: %w", index, err)
			}
		}
	}
	return nil
}

func clipWorldTriangle(input [3]worldClipVertex, c Camera, f cameraFrame) ([12]worldClipVertex, int) {
	var a, b [12]worldClipVertex
	copy(a[:], input[:])
	count := 3
	for plane := 0; plane < 6 && count > 0; plane++ {
		next := 0
		prev := a[count-1]
		dp := planeDistance(prev.p, plane, c, f)
		for i := 0; i < count; i++ {
			cur := a[i]
			dc := planeDistance(cur.p, plane, c, f)
			if (dp >= 0) != (dc >= 0) {
				// Adjacent triangles traverse a shared edge in opposite directions.
				// Canonicalize its interpolation so clipping produces identical
				// endpoint bits and cannot open a sample-sized crack on that edge.
				lo, hi, dl, dh := prev, cur, dp, dc
				if lo.p.X > hi.p.X || (lo.p.X == hi.p.X && (lo.p.Y > hi.p.Y || (lo.p.Y == hi.p.Y && lo.p.Z > hi.p.Z))) {
					lo, hi, dl, dh = hi, lo, dh, dl
				}
				t := dl / (dl - dh)
				b[next] = worldClipVertex{
					world: lo.world.Lerp(hi.world, t),
					texturedClipVertex: texturedClipVertex{
						clipVertex: clipVertex{p: lo.p.Lerp(hi.p, t), n: lo.n.Lerp(hi.n, t)},
						uv:         UV{U: lo.uv.U + (hi.uv.U-lo.uv.U)*t, V: lo.uv.V + (hi.uv.V-lo.uv.V)*t},
					},
				}
				if plane == 0 {
					b[next].p.Z = c.Near
				}
				if plane == 1 {
					b[next].p.Z = c.Far
				}
				next++
			}
			if dc >= 0 {
				b[next] = cur
				next++
			}
			prev, dp = cur, dc
		}
		a, b = b, a
		count = next
	}
	return a, count
}

func (r *Renderer) rasterizeWorld(img *image.RGBA, depth []float64, a, b, c worldScreenVertex, material Material, perspective bool, fragments *transparentBuffer, shadow *shadowMap, lightWork *pointLightWork) error {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for _, v := range [3]screenVertex{a.screenVertex, b.screenVertex, c.screenVertex} {
		if !shadowProjectionBounded(v, w, h) {
			return fmt.Errorf("three: world-attribute receiver projection lost finite clipped bounds")
		}
	}
	area := edge(a.screenVertex, b.screenVertex, c.x, c.y)
	if area == 0 || !finite(area) || (r.config.CullBackfaces && area >= 0) {
		return nil
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	x0 := max(0, int(math.Ceil(math.Min(a.x, math.Min(b.x, c.x))-.5)))
	x1 := min(w-1, int(math.Floor(math.Max(a.x, math.Max(b.x, c.x))-.5)))
	y0 := max(0, int(math.Ceil(math.Min(a.y, math.Min(b.y, c.y))-.5)))
	y1 := min(h-1, int(math.Floor(math.Max(a.y, math.Max(b.y, c.y))-.5)))
	t0, t1, t2 := topLeft(b.screenVertex, c.screenVertex), topLeft(c.screenVertex, a.screenVertex), topLeft(a.screenVertex, b.screenVertex)
	base := material.Color
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			if !covered(transparentEdge(b.screenVertex, c.screenVertex, px, py), t0) || !covered(transparentEdge(c.screenVertex, a.screenVertex, px, py), t1) || !covered(transparentEdge(a.screenVertex, b.screenVertex, px, py), t2) {
				continue
			}
			wa, wb, wc := edge(b.screenVertex, c.screenVertex, px, py)/area, edge(c.screenVertex, a.screenVertex, px, py)/area, edge(a.screenVertex, b.screenVertex, px, py)/area
			z := wa*a.z + wb*b.z + wc*c.z
			if perspective {
				q := wa*a.q + wb*b.q + wc*c.q
				z = 1 / q
				wa, wb, wc = wa*a.q/q, wb*b.q/q, wc*c.q/q
			}
			i := y*w + x
			if !finite(z) || z >= depth[i] {
				continue
			}
			alpha := float64(base.A) / 255
			var texel textureSample
			if material.Texture != nil {
				uv := UV{U: wa*a.uv.U + wb*b.uv.U + wc*c.uv.U, V: wa*a.uv.V + wb*b.uv.V + wc*c.uv.V}
				if !finite(uv.U) || !finite(uv.V) {
					return fmt.Errorf("three: interpolated world texture UV is nonfinite")
				}
				texel = material.Texture.sample(uv, material.Sampler)
				alpha = texel.a * float64(base.A) / 255
			}
			// Retain texel/product alpha precision until the shared fragment
			// compositor. Opaque samples write depth only in the opaque pass.
			if alpha == 0 || (fragments == nil && alpha < 1) || (fragments != nil && alpha == 1) {
				continue
			}
			n, ok := unit(a.n.Mul(wa).Add(b.n.Mul(wb)).Add(c.n.Mul(wc)))
			lambert := 0.0
			if ok && r.config.Light.Kind == PointLight && r.config.Light.Diffuse > 0 {
				world := a.world.Mul(wa).Add(b.world.Mul(wb)).Add(c.world.Mul(wc))
				var err error
				lambert, err = r.config.Light.pointDiffuse(world, n)
				if err != nil {
					return err
				}
			} else if ok && r.config.Light.Kind == DirectionalLight {
				lambert = math.Max(0, n.Dot(r.lightDirection))
			}
			if shadow != nil && lambert > 0 && r.config.Light.Diffuse > 0 {
				world := a.world.Mul(wa).Add(b.world.Mul(wb)).Add(c.world.Mul(wc))
				blocked, err := shadow.occluded(world)
				if err != nil {
					return err
				}
				if blocked {
					lambert = 0
				}
			}
			intensity := math.Min(1, r.config.Light.Ambient+r.config.Light.Diffuse*lambert)
			intensities := [3]float64{intensity, intensity, intensity}
			if r.config.Light.Kind == PointLightList {
				world := a.world.Mul(wa).Add(b.world.Mul(wb)).Add(c.world.Mul(wc))
				var err error
				intensities, err = r.pointIntensities(world, n, lightWork)
				if err != nil {
					return err
				}
			}
			red := uint8(math.Round(float64(base.R) * intensities[0]))
			green := uint8(math.Round(float64(base.G) * intensities[1]))
			blue := uint8(math.Round(float64(base.B) * intensities[2]))
			if material.Texture != nil {
				red = textureByte(texel.r / texel.a * (float64(base.R) / 255) * intensities[0])
				green = textureByte(texel.g / texel.a * (float64(base.G) / 255) * intensities[1])
				blue = textureByte(texel.b / texel.a * (float64(base.B) / 255) * intensities[2])
			}
			if fragments == nil {
				p := y*img.Stride + x*4
				img.Pix[p], img.Pix[p+1], img.Pix[p+2], img.Pix[p+3] = red, green, blue, 255
				depth[i] = z
			} else {
				rgb := uint32(red)<<16 | uint32(green)<<8 | uint32(blue)
				if material.Texture != nil {
					if err := fragments.addTextured(i, z, rgb, alpha); err != nil {
						return err
					}
				} else if err := fragments.add(i, z, rgb<<8|uint32(base.A)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
