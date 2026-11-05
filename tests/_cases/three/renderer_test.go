package three

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"sync"
	"testing"
)

var red = color.NRGBA{R: 255, A: 255}
var green = color.NRGBA{G: 255, A: 255}

func ambientRenderer(t *testing.T) *Renderer {
	t.Helper()
	c := DefaultConfig(64, 64)
	c.Light = Light{Ambient: 1}
	r, err := NewRenderer(c)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func colored(m *Mesh, c color.NRGBA) Instance { in := NewInstance(m); in.Material.Color = c; return in }

func litPixels(img *image.RGBA) int {
	n := 0
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 0 {
			n++
		}
	}
	return n
}

func TestClipCrossingNearFarAndViewport(t *testing.T) {
	r := ambientRenderer(t)
	for _, projection := range []Projection{Perspective, Orthographic} {
		t.Run(fmt.Sprint(projection), func(t *testing.T) {
			c := testCamera(projection)
			f, err := c.frame(1)
			if err != nil {
				t.Fatal(err)
			}
			for _, outside := range []float64{-.25, -12, .5} {
				m := triangleMesh()
				m.Vertices[2].Z = outside
				var input [3]clipVertex
				for i, p := range m.Vertices {
					input[i] = clipVertex{p: f.view(c, p), n: geom.Vec3{Z: 1}}
				}
				poly, count := clipTriangle(input, c, f)
				if count < 3 {
					t.Fatalf("crossing triangle dropped, z=%g", outside)
				}
				boundary := c.Near
				if outside < -c.Far {
					boundary = c.Far
				}
				hits := 0
				for _, v := range poly[:count] {
					for plane := 0; plane < 6; plane++ {
						if planeDistance(v.p, plane, c, f) < -1e-10 {
							t.Fatalf("vertex escaped plane %d: %+v", plane, v)
						}
					}
					if math.Abs(v.p.Z-boundary) < 1e-10 {
						hits++
					}
				}
				if hits < 2 {
					t.Fatalf("missing clipping intersections z=%g: %+v", outside, poly[:count])
				}
				img, err := r.Render(c, colored(m, red))
				if err != nil {
					t.Fatal(err)
				}
				if litPixels(img) < 10 {
					t.Fatalf("crossing triangle lost visible area, z=%g", outside)
				}
			}
			// All three projected vertices are outside, but the triangle covers the
			// entire viewport. Dropping offscreen vertices would lose every pixel.
			m := &Mesh{Vertices: []geom.Vec3{{X: -100, Y: -100, Z: -2}, {X: 100, Y: -100, Z: -2}, {Y: 100, Z: -2}}, Triangles: []Triangle{{0, 1, 2}}}
			img, err := r.Render(c, colored(m, red))
			if err != nil {
				t.Fatal(err)
			}
			if litPixels(img) != 64*64 {
				t.Fatalf("viewport clipping leaves holes: %d", litPixels(img))
			}
			for _, z := range []float64{-.5, -11, 1} {
				m := triangleMesh()
				for i := range m.Vertices {
					m.Vertices[i].Z = z
				}
				img, err := r.Render(c, colored(m, red))
				if err != nil {
					t.Fatal(err)
				}
				if litPixels(img) != 0 {
					t.Fatalf("out-of-range triangle visible at z=%g", z)
				}
			}
		})
	}
}

func TestDepthOpaqueOrderIndependent(t *testing.T) {
	r := ambientRenderer(t)
	for _, projection := range []Projection{Perspective, Orthographic} {
		c := testCamera(projection)
		a, b := triangleMesh(), triangleMesh()
		for i := range b.Vertices {
			b.Vertices[i].Z = -4
		}
		first, err := r.Render(c, colored(a, green), colored(b, red))
		if err != nil {
			t.Fatal(err)
		}
		second, err := r.Render(c, colored(b, red), colored(a, green))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.Pix, second.Pix) {
			t.Fatal("opaque rendering depends on submission order")
		}
		if first.RGBAAt(32, 32) != (color.RGBA{G: 255, A: 255}) {
			t.Fatal("far triangle occluded near triangle")
		}
	}
}

// rayHit is an independent ray/plane intersection plus world-space barycentric
// solution. It tests the rasterizer against geometry rather than its screen
// interpolation formula.
func rayHit(origin, direction geom.Vec3, m *Mesh) (distance float64, weights [3]float64, ok bool) {
	a, b, c := m.Vertices[0], m.Vertices[1], m.Vertices[2]
	u, v := b.Sub(a), c.Sub(a)
	normal := u.Cross(v)
	denom := normal.Dot(direction)
	if math.Abs(denom) < 1e-12 {
		return 0, weights, false
	}
	distance = normal.Dot(a.Sub(origin)) / denom
	if distance <= 0 {
		return 0, weights, false
	}
	p := origin.Add(direction.Mul(distance)).Sub(a)
	uu, uv, vv := u.Dot(u), u.Dot(v), v.Dot(v)
	pu, pv := p.Dot(u), p.Dot(v)
	det := uu*vv - uv*uv
	weights[1] = (pu*vv - pv*uv) / det
	weights[2] = (pv*uu - pu*uv) / det
	weights[0] = 1 - weights[1] - weights[2]
	return distance, weights, weights[0] > 1e-7 && weights[1] > 1e-7 && weights[2] > 1e-7
}

func perspectiveTriangle() *Mesh {
	// All three project onto the same footprint as a triangle at z=-3. The
	// first corner is near, so correct depth differs sharply from linear z.
	return &Mesh{Vertices: []geom.Vec3{{X: -.8, Y: -.8, Z: -1}, {X: 6.4, Y: -6.4, Z: -8}, {Y: 6.4, Z: -8}}, Triangles: []Triangle{{0, 1, 2}}}
}

func TestIntersectingTrianglesAgainstRayReference(t *testing.T) {
	r := ambientRenderer(t)
	for _, projection := range []Projection{Perspective, Orthographic} {
		c := testCamera(projection)
		c.OrthoHeight = 2
		a := perspectiveTriangle()
		if projection == Orthographic {
			a.Vertices = []geom.Vec3{{X: -.8, Y: -.8, Z: -1}, {X: .8, Y: -.8, Z: -8}, {Y: .8, Z: -8}}
		}
		b := &Mesh{Vertices: []geom.Vec3{{X: -2.4, Y: -2.4, Z: -3}, {X: 2.4, Y: -2.4, Z: -3}, {Y: 2.4, Z: -3}}, Triangles: []Triangle{{0, 1, 2}}}
		if projection == Orthographic {
			b.Vertices = []geom.Vec3{{X: -.8, Y: -.8, Z: -3}, {X: .8, Y: -.8, Z: -3}, {Y: .8, Z: -3}}
		}
		img, err := r.Render(c, colored(a, red), colored(b, green))
		if err != nil {
			t.Fatal(err)
		}
		reversed, err := r.Render(c, colored(b, green), colored(a, red))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(img.Pix, reversed.Pix) {
			t.Fatal("intersecting triangles depend on submission order")
		}
		reds, greens := 0, 0
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				sx, sy := 2*(float64(x)+.5)/64-1, 1-2*(float64(y)+.5)/64
				origin, direction := geom.Vec3{}, geom.Vec3{X: sx, Y: sy, Z: -1}
				if projection == Orthographic {
					origin = geom.Vec3{X: sx, Y: sy}
					direction = geom.Vec3{Z: -1}
				}
				da, _, oka := rayHit(origin, direction, a)
				db, _, okb := rayHit(origin, direction, b)
				if !oka || !okb || math.Abs(da-db) < 1e-7 {
					continue
				}
				want := color.RGBA{G: 255, A: 255}
				greens++
				if da < db {
					want = color.RGBA{R: 255, A: 255}
					reds++
					greens--
				}
				if got := img.RGBAAt(x, y); got != want {
					t.Fatalf("projection %d pixel (%d,%d) = %v, want %v; ray depths %g,%g", projection, x, y, got, want, da, db)
				}
			}
		}
		if reds < 20 || greens < 20 {
			t.Fatalf("insufficient intersection coverage: %d red %d green", reds, greens)
		}
	}
}

func TestPerspectiveCorrectNormals(t *testing.T) {
	cfg := DefaultConfig(64, 64)
	cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := perspectiveTriangle()
	m.Normals = []geom.Vec3{{Z: 1}, {X: 1}, {Y: 1}}
	img, err := r.Render(testCamera(Perspective), NewInstance(m))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for y := 10; y < 55; y += 5 {
		for x := 10; x < 55; x += 5 {
			d := geom.Vec3{X: 2*(float64(x)+.5)/64 - 1, Y: 1 - 2*(float64(y)+.5)/64, Z: -1}
			_, weights, ok := rayHit(geom.Vec3{}, d, m)
			if !ok {
				continue
			}
			n := m.Normals[0].Mul(weights[0]).Add(m.Normals[1].Mul(weights[1])).Add(m.Normals[2].Mul(weights[2]))
			n, _ = unit(n)
			want := math.Round(255 * n.Z)
			got := float64(img.RGBAAt(x, y).R)
			if math.Abs(got-want) > 1 {
				t.Fatalf("normal at %d,%d: intensity %g want %g", x, y, got, want)
			}
			checked++
		}
	}
	if checked < 10 {
		t.Fatal("insufficient smooth-normal checks")
	}
}

func TestClippingAgainstSeededRayReference(t *testing.T) {
	const width, height = 32, 24
	cfg := DefaultConfig(width, height)
	cfg.Light = Light{Ambient: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewPCG(41, 73))
	for _, projection := range []Projection{Perspective, Orthographic} {
		camera := testCamera(projection)
		for frame := 0; frame < 120; frame++ {
			m := &Mesh{Triangles: []Triangle{{0, 1, 2}}}
			for range 3 {
				m.Vertices = append(m.Vertices, geom.Vec3{X: 20*random.Float64() - 10, Y: 20*random.Float64() - 10, Z: 3 - 18*random.Float64()})
			}
			img, err := r.Render(camera, colored(m, red))
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					sy := 1 - 2*(float64(y)+.5)/height
					sx := (2*(float64(x)+.5)/width - 1) * width / height
					origin, direction := geom.Vec3{}, geom.Vec3{X: sx, Y: sy, Z: -1}
					if projection == Orthographic {
						origin = geom.Vec3{X: sx * 2, Y: sy * 2}
						direction = geom.Vec3{Z: -1}
					}
					depth, _, hit := rayHit(origin, direction, m)
					want := hit && depth >= camera.Near && depth <= camera.Far
					if got := img.RGBAAt(x, y).A == 255; got != want {
						t.Fatalf("projection %d frame %d pixel (%d,%d): visible=%t want %t, ray depth %g", projection, frame, x, y, got, want, depth)
					}
				}
			}
		}
	}
}

func TestClippedPerspectiveNormalsAgainstRayReference(t *testing.T) {
	cfg := DefaultConfig(64, 64)
	cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := perspectiveTriangle()
	m.Vertices[0] = geom.Vec3{X: -.2, Y: -.2, Z: -.25}
	m.Normals = []geom.Vec3{{Z: 1}, {X: 1}, {Y: 1}}
	img, err := r.Render(testCamera(Perspective), NewInstance(m))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			d := geom.Vec3{X: 2*(float64(x)+.5)/64 - 1, Y: 1 - 2*(float64(y)+.5)/64, Z: -1}
			depth, weights, ok := rayHit(geom.Vec3{}, d, m)
			if !ok || depth < 1 || depth > 10 {
				continue
			}
			n := m.Normals[0].Mul(weights[0]).Add(m.Normals[1].Mul(weights[1])).Add(m.Normals[2].Mul(weights[2]))
			n, _ = unit(n)
			want := math.Round(255 * n.Z)
			got := img.RGBAAt(x, y)
			if got.A != 255 || math.Abs(float64(got.R)-want) > 1 {
				t.Fatalf("clipped normal %d,%d: %v want %g", x, y, got, want)
			}
			checked++
		}
	}
	if checked < 100 {
		t.Fatal("insufficient clipped normal coverage")
	}
}

func TestModelNormalsCullingAndOrientation(t *testing.T) {
	cfg := DefaultConfig(64, 64)
	cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}
	cfg.CullBackfaces = true
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := triangleMesh()
	m.Normals = []geom.Vec3{{X: 1, Z: 1}, {X: 1, Z: 1}, {X: 1, Z: 1}}
	c := testCamera(Orthographic)
	for _, sx := range []float64{2, -2} {
		in := NewInstance(m)
		in.Model = geom.Translation(geom.Vec3{Z: -2}).Mul(geom.Scaling(sx, 1, .5))
		img, err := r.Render(c, in)
		if err != nil {
			t.Fatal(err)
		}
		want := uint8(math.Round(255 * 2 / math.Hypot(.5, 2)))
		if got := img.RGBAAt(32, 32); got.R != want || got.A != 255 {
			t.Fatalf("inverse-transpose/reflection sx=%g: %v want %d", sx, got, want)
		}
	}
	back := triangleMesh()
	back.Triangles[0] = Triangle{0, 2, 1}
	img, err := r.Render(c, NewInstance(back))
	if err != nil {
		t.Fatal(err)
	}
	if litPixels(img) != 0 {
		t.Fatal("backface survived culling")
	}
	cfg.CullBackfaces = false
	r, err = NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	img, err = r.Render(c, NewInstance(back))
	if err != nil {
		t.Fatal(err)
	}
	if litPixels(img) == 0 || img.RGBAAt(32, 32).R != 0 {
		t.Fatal("backface should be visible but receive no front light")
	}
	// Translating the camera and every object together preserves the image.
	r = ambientRenderer(t)
	base, err := r.Render(testCamera(Perspective), NewInstance(m))
	if err != nil {
		t.Fatal(err)
	}
	delta := geom.Vec3{X: 3, Y: 4, Z: 5}
	c = testCamera(Perspective)
	c.Position = c.Position.Add(delta)
	c.Target = c.Target.Add(delta)
	in := NewInstance(m)
	in.Model = geom.Translation(delta)
	translated, err := r.Render(c, in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base.Pix, translated.Pix) {
		t.Fatal("camera/model translation changed image")
	}
}

func TestLightingAlphaAndSharedEdges(t *testing.T) {
	cfg := DefaultConfig(64, 64)
	cfg.Background = color.NRGBA{R: 200, G: 100, B: 50, A: 128}
	cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Ambient: .2, Diffuse: .5}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	in := colored(triangleMesh(), color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	img, err := r.Render(testCamera(Orthographic), in)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(32, 32); got != (color.RGBA{R: 140, G: 70, B: 35, A: 255}) {
		t.Fatalf("Lambert/ambient: %v", got)
	}
	if got, want := img.RGBAAt(0, 0), color.RGBAModel.Convert(cfg.Background).(color.RGBA); got != want {
		t.Fatalf("premultiplied background %v != %v", got, want)
	}
	for _, alpha := range []uint8{0, 1, 128, 254} {
		in.Material.Color.A = alpha
		got, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		want := referenceTransparentLayers(cfg.Background, color.NRGBA{R: 140, G: 70, B: 35, A: alpha})
		if pixel := got.RGBAAt(32, 32); pixel != want {
			t.Fatalf("material alpha %d: %v want %v", alpha, pixel, want)
		}
	}
	// Two triangles tile exactly across pixel centers on their shared diagonal.
	quad := &Mesh{Vertices: []geom.Vec3{{X: -2, Y: -2, Z: -2}, {X: 2, Y: -2, Z: -2}, {X: 2, Y: 2, Z: -2}, {X: -2, Y: 2, Z: -2}}, Triangles: []Triangle{{0, 1, 2}, {0, 2, 3}}}
	r = ambientRenderer(t)
	img, err = r.Render(testCamera(Orthographic), colored(quad, red))
	if err != nil {
		t.Fatal(err)
	}
	if litPixels(img) != 4096 {
		t.Fatal("crack in shared triangle edge")
	}
	// Equal-depth separate faces obey a documented first-submitted tie rule.
	in = colored(triangleMesh(), red)
	other := colored(triangleMesh(), green)
	img, err = r.Render(testCamera(Orthographic), in, other)
	if err != nil {
		t.Fatal(err)
	}
	if img.RGBAAt(32, 32) != (color.RGBA{R: 255, A: 255}) {
		t.Fatal("depth tie did not keep first triangle")
	}
}

func TestRendererRejectsUnsupportedAndNonfiniteInputs(t *testing.T) {
	for _, dim := range [][2]int{{0, 1}, {-1, 1}, {MaxDimension + 1, 1}, {MaxDimension, MaxDimension}} {
		if _, err := NewRenderer(DefaultConfig(dim[0], dim[1])); err == nil {
			t.Errorf("invalid dimensions %v accepted", dim)
		}
	}
	for _, light := range []Light{{Ambient: math.NaN()}, {Ambient: -1}, {Diffuse: 1}, {Direction: geom.Vec3{X: math.Inf(1)}}} {
		c := DefaultConfig(64, 64)
		c.Light = light
		if _, err := NewRenderer(c); err == nil {
			t.Errorf("bad light %+v accepted", light)
		}
	}
	r := ambientRenderer(t)
	camera := testCamera(Perspective)
	for _, model := range []geom.Mat4{geom.Scaling(1, 0, 1), geom.Scaling(1, 1, 1e-13), {0: 1, 5: 1, 10: 1, 12: .1, 15: 1}, {0: math.NaN(), 5: 1, 10: 1, 15: 1}, geom.Translation(geom.Vec3{X: MaxCoordinate})} {
		in := NewInstance(triangleMesh())
		in.Model = model
		if img, err := r.Render(camera, in); err == nil || img != nil {
			t.Errorf("invalid model accepted: %v", model)
		}
	}
	var nilRenderer *Renderer
	if _, err := nilRenderer.Render(camera); err == nil {
		t.Fatal("nil renderer accepted")
	}
	if _, err := r.Render(camera, NewInstance(nil)); err == nil {
		t.Fatal("nil mesh accepted")
	}
	if _, err := r.Render(camera, make([]Instance, MaxInstances+1)...); err == nil {
		t.Fatal("too many instances accepted")
	}
	camera.Near = 0
	if _, err := r.Render(camera); err == nil {
		t.Fatal("invalid camera accepted")
	}
}

func TestConcurrentRenderers(t *testing.T) {
	m, err := Sphere(1, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	in := NewInstance(m)
	camera := testCamera(Perspective)
	camera.Position = geom.Vec3{Z: 4}
	camera.Target = geom.Vec3{}
	cfg := DefaultConfig(96, 64)
	shared, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := shared.Render(camera, in)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(reference.Pix)
	var wg sync.WaitGroup
	errors := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := shared
			if i%2 == 0 {
				var err error
				r, err = NewRenderer(cfg)
				if err != nil {
					errors <- err
					return
				}
			}
			for range 3 {
				img, err := r.Render(camera, in)
				if err != nil {
					errors <- err
					return
				}
				if sha256.Sum256(img.Pix) != want {
					errors <- fmt.Errorf("worker %d changed frame", i)
					return
				}
				img.Pix[0] ^= 255
			}
		}(i)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func BenchmarkRenderSphere(b *testing.B) {
	m, err := Sphere(1, 64, 32)
	if err != nil {
		b.Fatal(err)
	}
	r, err := NewRenderer(DefaultConfig(320, 180))
	if err != nil {
		b.Fatal(err)
	}
	c := testCamera(Perspective)
	c.Position = geom.Vec3{Z: 3}
	c.Target = geom.Vec3{}
	in := NewInstance(m)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := r.Render(c, in); err != nil {
			b.Fatal(err)
		}
	}
}
