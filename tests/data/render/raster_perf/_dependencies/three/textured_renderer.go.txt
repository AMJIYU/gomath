package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"math"
)

func materialHasTransparency(m Material) bool {
	return m.Color.A > 0 && (m.Color.A < 255 || (m.Texture != nil && !m.Texture.opaque))
}

// UV attributes live only in textured clipping/projection records, preserving
// the sizes and arithmetic of the original untextured vertex/clip fast paths.
type texturedClipVertex struct {
	clipVertex
	uv UV
}
type texturedScreenVertex struct {
	screenVertex
	uv UV
}

// Keep textured per-triangle branches, larger vertex records and clip scratch
// out of renderInstance's untextured loop. Only world/view/normal preparation is
// shared, followed by one dispatch per textured instance.
func (r *Renderer) renderTexturedMesh(img *image.RGBA, depth []float64, camera Camera, f cameraFrame, prepared preparedInstance, index int, world, view, normals []geom.Vec3, fragments *transparentBuffer) error {
	in, model := prepared.instance, prepared.model
	size := img.Bounds().Size()
	for _, indices := range in.Mesh.Triangles {
		if model.reflected {
			indices[1], indices[2] = indices[2], indices[1]
		}
		face, ok := unit(world[indices[1]].Sub(world[indices[0]]).Cross(world[indices[2]].Sub(world[indices[0]])))
		if !ok {
			return fmt.Errorf("three: instance %d transformed triangle is degenerate", index)
		}
		var input [3]texturedClipVertex
		for k, i := range indices {
			input[k] = texturedClipVertex{clipVertex: clipVertex{p: view[i], n: face}, uv: in.Mesh.UVs[i]}
			if normals != nil {
				input[k].n = normals[i]
			}
		}
		polygon, count := clipTexturedTriangle(input, camera, f)
		for k := 1; k+1 < count; k++ {
			a := projectTexturedVertex(polygon[0], f, size.X, size.Y)
			b := projectTexturedVertex(polygon[k], f, size.X, size.Y)
			c := projectTexturedVertex(polygon[k+1], f, size.X, size.Y)
			if err := r.rasterizeTextured(img, depth, a, b, c, in.Material, f.perspective, fragments); err != nil {
				return fmt.Errorf("three: instance %d: %w", index, err)
			}
		}
	}
	return nil
}

func clipTexturedTriangle(input [3]texturedClipVertex, c Camera, f cameraFrame) ([12]texturedClipVertex, int) {
	var a, b [12]texturedClipVertex
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
				t := dp / (dp - dc)
				b[next] = texturedClipVertex{
					clipVertex: clipVertex{p: prev.p.Lerp(cur.p, t), n: prev.n.Lerp(cur.n, t)},
					uv:         UV{U: prev.uv.U + (cur.uv.U-prev.uv.U)*t, V: prev.uv.V + (cur.uv.V-prev.uv.V)*t},
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

func projectTexturedVertex(v texturedClipVertex, f cameraFrame, width, height int) texturedScreenVertex {
	return texturedScreenVertex{screenVertex: projectVertex(v.clipVertex, f, width, height), uv: v.uv}
}

// A nil fragments buffer denotes the opaque pass. Every texel with exactly
// opaque effective alpha writes depth there. The second pass emits only partial
// alpha through the same fragment budget/sort/resolve chain as other materials.
func (r *Renderer) rasterizeTextured(img *image.RGBA, depth []float64, a, b, c texturedScreenVertex, material Material, perspective bool, fragments *transparentBuffer) error {
	area := edge(a.screenVertex, b.screenVertex, c.x, c.y)
	if area == 0 || !finite(area) || (r.config.CullBackfaces && area >= 0) {
		return nil
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
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
			e0, e1, e2 := edge(b.screenVertex, c.screenVertex, px, py), edge(c.screenVertex, a.screenVertex, px, py), edge(a.screenVertex, b.screenVertex, px, py)
			wa, wb, wc := e0/area, e1/area, e2/area
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
			uv := UV{U: wa*a.uv.U + wb*b.uv.U + wc*c.uv.U, V: wa*a.uv.V + wb*b.uv.V + wc*c.uv.V}
			if !finite(uv.U) || !finite(uv.V) {
				return fmt.Errorf("three: interpolated texture UV is nonfinite")
			}
			texel := material.Texture.sample(uv, material.Sampler)
			alpha := texel.a * float64(base.A) / 255
			if alpha == 0 || (fragments == nil && alpha < 1) || (fragments != nil && alpha == 1) {
				continue
			}
			n, ok := unit(a.n.Mul(wa).Add(b.n.Mul(wb)).Add(c.n.Mul(wc)))
			lambert := 0.0
			if ok {
				lambert = math.Max(0, n.Dot(r.lightDirection))
			}
			intensity := math.Min(1, r.config.Light.Ambient+r.config.Light.Diffuse*lambert)
			red := textureByte(texel.r / texel.a * (float64(base.R) / 255) * intensity)
			green := textureByte(texel.g / texel.a * (float64(base.G) / 255) * intensity)
			blue := textureByte(texel.b / texel.a * (float64(base.B) / 255) * intensity)
			if fragments == nil {
				p := y*img.Stride + x*4
				img.Pix[p], img.Pix[p+1], img.Pix[p+2], img.Pix[p+3] = red, green, blue, 255
				depth[i] = z
			} else {
				rgb := uint32(red)<<16 | uint32(green)<<8 | uint32(blue)
				if err := fragments.addTextured(i, z, rgb, alpha); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
