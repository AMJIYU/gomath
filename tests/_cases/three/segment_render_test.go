package three

import (
	"bytes"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"testing"
)

func segmentRenderMeshes(tb testing.TB, detailed bool) (meshes, references []*Mesh) {
	tb.Helper()
	start, end := geom.Vec3{X: -.7, Y: -.37, Z: -1.1}, geom.Vec3{X: .8, Y: .23, Z: 1.2}
	o := ArrowOptions{ShaftRadius: .22, HeadRadius: .52, HeadLength: .8, RadialSegments: 7, ShaftSegments: 2, HeadSegments: 2}
	if detailed {
		o.RadialSegments = 48
		o.ShaftSegments = 4
		o.HeadSegments = 4
	}
	c, e := CylinderBetween(start, end, .32, o.RadialSegments, o.ShaftSegments)
	if e != nil {
		tb.Fatal(e)
	}
	a, e := ArrowBetween(start, end, o)
	if e != nil {
		tb.Fatal(e)
	}
	return []*Mesh{c, a}, []*Mesh{segmentReference(start, end, .32, o.RadialSegments, o.ShaftSegments, nil), segmentReference(start, end, 0, o.RadialSegments, 0, &o)}
}

func segmentLighting(c Config) func(geom.Vec3, geom.Vec3) [3]float64 {
	return func(p, n geom.Vec3) [3]float64 {
		if c.Light.Kind == PointLightList {
			return multiLightReferenceIntensity(c, p, n)
		}
		d := c.Light.Direction
		v := c.Light.Ambient
		if c.Light.Diffuse > 0 {
			v += c.Light.Diffuse * math.Max(0, n.Dot(d)/math.Sqrt(d.Dot(d)))
		}
		v = math.Min(1, v)
		return [3]float64{v, v, v}
	}
}

func segmentPixelCheck(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	for i, ch := range got.Pix {
		if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
			t.Fatalf("ray mismatch pixel(%d,%d) channel%d: %d want%d", i/4%got.Bounds().Dx(), i/4/got.Bounds().Dx(), i%4, ch, want.Pix[i])
		}
	}
}

func TestSegmentIndependentRays(t *testing.T) {
	meshes, refs := segmentRenderMeshes(t, false)
	for shape, m := range meshes {
		for _, projection := range []Projection{Perspective, Orthographic} {
			for _, ssaa := range []int{1, 2, 4} {
				for frame, angle := range []float64{0, .43, 1.13} {
					for _, style := range []string{"opaque", "alpha", "uv-lights"} {
						t.Run(fmt.Sprintf("shape%d/p%d/s%d/f%d/%s", shape, projection, ssaa, frame, style), func(t *testing.T) {
							c := DefaultConfig(41, 31)
							c.Supersampling = ssaa
							c.CullBackfaces = style == "opaque"
							c.Background = color.NRGBA{R: 19, G: 37, B: 61, A: 109}
							if style == "uv-lights" {
								c = multiLightConfig(c)
							}
							camera := primitiveCamera(projection)
							pose := polyhedronPose{scale: geom.Vec3{X: 1, Y: 1, Z: 1}, axis: geom.Vec3{X: .3, Y: 1, Z: .7}, angle: angle, position: geom.Vec3{X: .07, Y: -.11, Z: .03}}
							if frame > 0 {
								pose.scale = geom.Vec3{X: .87, Y: 1.17, Z: .91}
							}
							if frame == 2 {
								pose.scale.X *= -1
								camera.Near = 5.1
								camera.Far = 6.9
								camera.FOVY = .32
								camera.OrthoHeight = 1.9
							}
							in := NewInstance(m)
							in.Model = polyhedronModel(pose)
							in.Material.Color = color.NRGBA{R: 227, G: 173, B: 97, A: 255}
							if style != "opaque" {
								in.Material.Color.A = 169
							}
							if style == "uv-lights" {
								in.Material.Texture = primitiveTexture(t, "mixed")
								in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
							}
							got := mustShadowRender(t, c, camera, in)
							ref := NewInstance(segmentReferencePose(refs[shape], pose))
							ref.Material = in.Material
							want := pointLightRayReferenceWithLighting(t, c, camera, segmentLighting(c), ref)
							segmentPixelCheck(t, got, want)
						})
					}
				}
			}
		}
	}
}

func TestSegmentConcurrentOrderAndRendererBudgets(t *testing.T) {
	meshes, _ := segmentRenderMeshes(t, false)
	c := multiLightConfig(DefaultConfig(71, 53))
	c.Supersampling = 2
	camera := primitiveCamera(Perspective)
	var instances []Instance
	for i, m := range meshes {
		in := NewInstance(m)
		in.Model = geom.Translation(geom.Vec3{X: float64(i)*.61 - .3})
		in.Material.Texture = primitiveTexture(t, "mixed")
		in.Material.Color.A = 173
		in.Material.Sampler.Filter = Bilinear
		instances = append(instances, in)
	}
	r, e := NewRenderer(c)
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
			for j, in := range order {
				copyMesh := *in.Mesh
				copyMesh.Triangles = slices.Clone(copyMesh.Triangles)
				slices.Reverse(copyMesh.Triangles)
				order[j].Mesh = &copyMesh
			}
			for range 3 {
				got, e := r.Render(camera, order...)
				if e != nil || !bytes.Equal(got.Pix, want.Pix) {
					t.Errorf("concurrent/order mismatch: %v", e)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, limit := range []string{"fragment", "light"} {
		cc := c
		switch limit {
		case "fragment":
			cc.TransparentFragmentLimit = 1
		case "light":
			cc.PointLightEvaluationLimit = 1
		}
		r, e := NewRenderer(cc)
		if e != nil {
			t.Fatal(e)
		}
		if img, e := r.Render(camera, instances...); e == nil || img != nil {
			t.Fatal("ignored budget", limit, e)
		}
		if img, e := r.Render(camera); e != nil || img == nil {
			t.Fatal("failed render poisoned renderer", limit, e)
		}
	}
	o := segmentOptions()
	o.RadialSegments = 256
	o.ShaftSegments = 256
	o.HeadSegments = 256
	large, e := ArrowBetween(geom.Vec3{}, geom.Vec3{Z: 2}, o)
	if e != nil {
		t.Fatal(e)
	}
	if img, e := r.Render(camera, NewInstance(large), NewInstance(large)); e == nil || img != nil {
		t.Fatal("ignored aggregate geometry quota", e)
	}

}
