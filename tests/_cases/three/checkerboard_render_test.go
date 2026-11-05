package three_test

import (
	"bytes"
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"testing"
)

func checkerCamera(p three.Projection) three.Camera {
	return three.Camera{Position: geom.Vec3{X: 3.1, Y: -3.7, Z: 3.2}, Up: geom.Vec3{Z: 1}, Projection: p, FOVY: .8, OrthoHeight: 3.5, Near: .1, Far: 20}
}
func checkerLights(c three.Config) three.Config {
	c.Light = three.Light{Kind: three.PointLightList, Ambient: .23}
	c.PointLights = []three.PointLightSource{{Position: geom.Vec3{X: 2, Y: -1, Z: 3}, RGB: [3]float64{1, .1, .03}, Diffuse: .85, Attenuation: .09}, {Position: geom.Vec3{X: -1.7, Y: .8, Z: 2.1}, RGB: [3]float64{.1, .95, .2}, Diffuse: .7, Attenuation: .05}, {Position: geom.Vec3{X: .1, Y: 3, Z: 1.1}, RGB: [3]float64{.03, .2, 1}, Diffuse: .8, Attenuation: .12}}
	return c
}

func checkerStyles(tb testing.TB, style string) [2]checkerStyle {
	tb.Helper()
	var styles [2]checkerStyle
	for i, c := range []color.NRGBA{{R: 239, G: 157, B: 47, A: 255}, {R: 37, G: 137, B: 229, A: 255}} {
		styles[i].material = three.Material{Color: c}
		if style != "opaque" {
			styles[i].material.Color.A = []uint8{137, 181}[i]
		}
		if style == "uv-lights" {
			src := image.NewNRGBA(image.Rect(0, 0, 13, 9))
			for y := range 9 {
				for x := range 13 {
					src.SetNRGBA(x, y, color.NRGBA{R: uint8(19 * x), G: uint8(29 * y), B: uint8(80 + 7*i), A: []uint8{0, 128, 255}[(x/3+y/2+i)%3]})
				}
			}
			tex, e := three.NewTexture(src)
			if e != nil {
				tb.Fatal(e)
			}
			styles[i].source = src
			styles[i].material.Texture = tex
			styles[i].material.Sampler = three.Sampler{Filter: three.Bilinear, AddressU: three.Repeat, AddressV: three.Clamp}
		}
	}
	return styles
}

func checkerInstances(groups [2]*three.Mesh, styles [2]checkerStyle, model geom.Mat4) []three.Instance {
	var instances []three.Instance
	for i, mesh := range groups {
		if mesh != nil {
			in := three.NewInstance(mesh)
			in.Material = styles[i].material
			in.Model = model
			instances = append(instances, in)
		}
	}
	return instances
}

func checkerRender(tb testing.TB, c three.Config, camera three.Camera, instances ...three.Instance) *image.RGBA {
	tb.Helper()
	r, e := three.NewRenderer(c)
	if e != nil {
		tb.Fatal(e)
	}
	img, e := r.Render(camera, instances...)
	if e != nil {
		tb.Fatal(e)
	}
	return img
}
func checkerPixels(t *testing.T, got image.Image, want *image.RGBA) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatal("bounds", got.Bounds(), want.Bounds())
	}
	for y := range want.Bounds().Dy() {
		for x := range want.Bounds().Dx() {
			a := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			b := want.RGBAAt(x, y)
			for i, d := range [4]int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				if d > 1 || d < -1 {
					t.Fatalf("checker ray pixel(%d,%d) channel%d got%v want%v", x, y, i, a, b)
				}
			}
		}
	}
}

func checkerModel(frame int) geom.Mat4 {
	model := geom.Translation(geom.Vec3{X: .07, Y: -.11, Z: .03})
	if frame == 0 {
		return model
	}
	scale := geom.Vec3{X: .87, Y: 1.17, Z: .91}
	angle := .43
	if frame == 2 {
		scale.X *= -1
		angle = 1.13
	}
	shear := geom.Identity()
	shear[1] = .21
	shear[6] = -.17
	shear[8] = .09
	return model.Mul(geom.Rotation(geom.Vec3{X: .3, Y: 1, Z: .7}, angle)).Mul(geom.Scaling(scale.X, scale.Y, scale.Z)).Mul(shear)
}

func TestCheckerboardIndependentRayFrames(t *testing.T) {
	for _, curved := range []bool{false, true} {
		point := func(u, v float64) geom.Vec3 {
			if curved {
				return checkerPoint(u, v)
			}
			return geom.Vec3{X: u, Y: v, Z: .13*u - .21*v}
		}
		groups := checkerBuild(t, 5, 4, point)
		ref := checkerReferenceGrid(-1, 1, -.8, .9, 5, 4, point)
		for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
			for _, ssaa := range []int{1, 2, 4} {
				for frame := range 3 {
					for _, style := range []string{"opaque", "alpha", "uv-lights"} {
						t.Run(fmt.Sprintf("curved%t/p%d/s%d/f%d/%s", curved, projection, ssaa, frame, style), func(t *testing.T) {
							c := three.DefaultConfig(41, 31)
							c.Supersampling = ssaa
							c.CullBackfaces = style == "opaque"
							c.Background = color.NRGBA{R: 17, G: 39, B: 61, A: 109}
							if style == "uv-lights" {
								c = checkerLights(c)
							}
							camera := checkerCamera(projection)
							if frame == 2 {
								camera.Near = 4.4
								camera.Far = 6.4
								camera.FOVY = .31
								camera.OrthoHeight = 1.6
							}
							styles := checkerStyles(t, style)
							model := checkerModel(frame)
							instances := checkerInstances(groups, styles, model)
							triangles := checkerWorld(ref, model, styles)
							if frame == 1 {
								// Existing opaque mesh masks the two material partitions in
								// the same depth buffer, including their translucent cells.
								p := func(u, v float64) geom.Vec3 { return geom.Vec3{X: .3*u + .12, Y: .25*v - .07, Z: .85} }
								mesh, e := three.ParametricSurface(-1, 1, -1, 1, 1, 1, p)
								if e != nil {
									t.Fatal(e)
								}
								material := three.Material{Color: color.NRGBA{R: 41, G: 223, B: 103, A: 255}}
								in := three.NewInstance(mesh)
								in.Material = material
								instances = append(instances, in)
								styles := [2]checkerStyle{{material: material}, {material: material}}
								triangles = append(triangles, checkerWorld(checkerReferenceGrid(-1, 1, -1, 1, 1, 1, p), geom.Identity(), styles)...)
							}
							got := checkerRender(t, c, camera, instances...)
							want := checkerRayReference(c, camera, triangles)
							checkerPixels(t, got, want)
						})
					}
				}
			}
		}
	}
}

// This reference classifies analytic plane hits directly by floor(cell UV),
// without using any triangle lists, normals or barycentric interpolation.
func TestCheckerboardAnalyticPlaneParityPixels(t *testing.T) {
	groups := checkerBuild(t, 5, 4, func(u, v float64) geom.Vec3 { return geom.Vec3{X: u, Y: v} })
	styles := checkerStyles(t, "alpha")
	for _, ssaa := range []int{1, 2, 4} {
		c := three.DefaultConfig(31, 23)
		c.Supersampling = ssaa
		c.Light = three.Light{Ambient: 1}
		c.Background = color.NRGBA{R: 29, G: 47, B: 83, A: 101}
		camera := three.Camera{Position: geom.Vec3{Z: 4}, Up: geom.Vec3{Y: 1}, Projection: three.Orthographic, OrthoHeight: 2.73, Near: .1, Far: 10}
		got := checkerRender(t, c, camera, checkerInstances(groups, styles, geom.Identity())...)
		want := image.NewRGBA(got.Bounds())
		base := color.RGBAModel.Convert(c.Background).(color.RGBA)
		for y := range c.Height {
			for x := range c.Width {
				var sum [4]int
				for sy := range ssaa {
					for sx := range ssaa {
						wx := (2*(float64(x)+(float64(sx)+.5)/float64(ssaa))/float64(c.Width) - 1) * float64(c.Width) / float64(c.Height) * camera.OrthoHeight / 2
						wy := (1 - 2*(float64(y)+(float64(sy)+.5)/float64(ssaa))/float64(c.Height)) * camera.OrthoHeight / 2
						out := base
						if wx >= -1 && wx < 1 && wy >= -.8 && wy < .9 {
							group := (int(math.Floor((wx+1)/2*5)) + int(math.Floor((wy+.8)/1.7*4))) % 2
							s := styles[group].material.Color
							a := float64(s.A) / 255
							out = color.RGBA{R: uint8(math.Round(float64(s.R)*a + float64(base.R)*(1-a))), G: uint8(math.Round(float64(s.G)*a + float64(base.G)*(1-a))), B: uint8(math.Round(float64(s.B)*a + float64(base.B)*(1-a))), A: uint8(math.Round(255*a + float64(base.A)*(1-a)))}
						}
						for j, ch := range [4]uint8{out.R, out.G, out.B, out.A} {
							sum[j] += int(ch)
						}
					}
				}
				n := ssaa * ssaa
				want.SetRGBA(x, y, color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)})
			}
		}
		checkerPixels(t, got, want)
	}
}

func TestCheckerboardConcurrentRenderOrderAndBudgets(t *testing.T) {
	groups := checkerBuild(t, 7, 5, checkerPoint)
	styles := checkerStyles(t, "uv-lights")
	c := checkerLights(three.DefaultConfig(67, 49))
	c.Supersampling = 2
	camera := checkerCamera(three.Perspective)
	instances := checkerInstances(groups, styles, checkerModel(2))
	r, e := three.NewRenderer(c)
	if e != nil {
		t.Fatal(e)
	}
	want, e := r.Render(camera, instances...)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order := slices.Clone(instances)
			if worker%2 == 1 {
				slices.Reverse(order)
			}
			for i, in := range order {
				m := *in.Mesh
				m.Triangles = slices.Clone(m.Triangles)
				slices.Reverse(m.Triangles)
				order[i].Mesh = &m
			}
			for range 3 {
				img, e := r.Render(camera, order...)
				if e != nil || !bytes.Equal(img.Pix, want.Pix) {
					t.Errorf("checker concurrent/order pixels: %v", e)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, mode := range []string{"fragment", "light"} {
		cc := c
		if mode == "fragment" {
			cc.TransparentFragmentLimit = 1
		} else {
			cc.PointLightEvaluationLimit = 1
		}
		r, e := three.NewRenderer(cc)
		if e != nil {
			t.Fatal(e)
		}
		if img, e := r.Render(camera, instances...); e == nil || img != nil {
			t.Fatal("ignored work budget", mode, e)
		}
		if img, e := r.Render(camera); e != nil || img == nil {
			t.Fatal("failed budget poisoned renderer", mode, e)
		}
	}
}
