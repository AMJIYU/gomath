package three

import (
	"bytes"
	"fmt"
	"gomath/geom"
	"image/color"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestPlatonicIndependentPlanePixels(t *testing.T) {
	for _, control := range loadPolyhedronControls(t) {
		mesh, err := polyhedronConstructor(t, control.Name)(1.2)
		if err != nil {
			t.Fatal(err)
		}
		for _, style := range []string{"opaque", "alpha", "uv-alpha", "uv-lights"} {
			for _, projection := range []Projection{Perspective, Orthographic} {
				for _, ssaa := range []int{1, 2, 4} {
					for frame, angle := range []float64{0, .43, 1.17, 2.23} {
						t.Run(fmt.Sprintf("%s/%s/p%d/s%d/f%d", control.Name, style, projection, ssaa, frame), func(t *testing.T) {
							c := DefaultConfig(43, 31)
							c.Supersampling = ssaa
							c.Background = color.NRGBA{R: 17, G: 39, B: 61, A: 107}
							camera := primitiveCamera(projection)
							pose := polyhedronPose{position: geom.Vec3{X: .13, Y: -.09, Z: .07}, scale: geom.Vec3{X: 1, Y: 1, Z: 1}, axis: geom.Vec3{X: .3, Y: 1, Z: .7}, angle: angle}
							if frame > 1 {
								pose.scale = geom.Vec3{X: -.93, Y: 1.17, Z: .81}
							}
							if frame == 3 {
								camera.Near, camera.Far, camera.FOVY, camera.OrthoHeight = 6, 7.2, .31, 1.7
							}
							in := NewInstance(mesh)
							in.Model = polyhedronModel(pose)
							in.Material.Color = color.NRGBA{R: 231, G: 157, B: 83, A: 255}
							c.CullBackfaces = style == "opaque"
							if style == "alpha" {
								in.Material.Color.A = 137
							}
							if style == "uv-alpha" || style == "uv-lights" {
								in.Material.Color.A = 193
								in.Material.Texture = primitiveTexture(t, "mixed")
								in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Clamp, AddressV: Clamp}
							}
							if style == "uv-lights" {
								c = multiLightConfig(c)
							}
							got := mustShadowRender(t, c, camera, in)
							want := polyhedronPlaneReference(t, c, camera, control, 1.2, pose, in.Material)
							for i, ch := range got.Pix {
								if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
									t.Fatalf("pixel%d,%d channel%d: %d want%d", i/4%c.Width, i/4/c.Width, i%4, ch, want.Pix[i])
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestPolyhedronConcurrentRenderAndOrder(t *testing.T) {
	c := multiLightConfig(DefaultConfig(67, 49))
	c.Supersampling = 2
	camera := primitiveCamera(Perspective)
	var instances []Instance
	for i, control := range loadPolyhedronControls(t) {
		v, f := polyhedronInput(control, .7)
		m, err := Polyhedron(v, f)
		if err != nil {
			t.Fatal(err)
		}
		in := NewInstance(m)
		in.Model = geom.Translation(geom.Vec3{X: float64(i%2)*1.3 - .65, Y: float64(i/2)*1.3 - .65}).Mul(geom.Rotation(geom.Vec3{X: .3, Y: 1, Z: .7}, .43))
		in.Material.Color = color.NRGBA{R: 231, G: uint8(73 + 30*i), B: uint8(53 + 45*i), A: 137}
		in.Material.Texture = primitiveTexture(t, "mixed")
		in.Material.Sampler.Filter = Bilinear
		instances = append(instances, in)
		for j := range v {
			v[j] = geom.Vec3{X: math.NaN()}
		}
		for _, face := range f {
			for j := range face {
				face[j] = -1
			}
		}
	}
	r, err := NewRenderer(c)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(camera, instances...)
	if err != nil {
		t.Fatal(err)
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
			for range 4 {
				img, err := r.Render(camera, order...)
				if err != nil || !bytes.Equal(img.Pix, want.Pix) {
					t.Errorf("parallel polyhedron pixels changed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	c.TransparentFragmentLimit = 1
	r, err = NewRenderer(c)
	if err != nil {
		t.Fatal(err)
	}
	if img, err := r.Render(camera, instances...); img != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatal("polyhedron bypassed fragment quota", err)
	}
	if img, err := r.Render(camera); img == nil || err != nil {
		t.Fatal("fragment overflow poisoned renderer", err)
	}
	c.TransparentFragmentLimit = 0
	c.PointLightEvaluationLimit = 1
	r, err = NewRenderer(c)
	if err != nil {
		t.Fatal(err)
	}
	if img, err := r.Render(camera, instances...); img != nil || err == nil || !strings.Contains(err.Error(), "light evaluation budget") {
		t.Fatal("polyhedron bypassed lighting quota", err)
	}
}
