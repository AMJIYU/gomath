package render_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"
)

func cameraNear(t *testing.T, a, b float64) {
	t.Helper()
	if math.IsNaN(a) || math.Abs(a-b) > 1e-9 {
		t.Fatalf("got %g want %g", a, b)
	}
}
func cameraVectorNear(t *testing.T, a, b geom.Vec3) {
	t.Helper()
	cameraNear(t, a.X, b.X)
	cameraNear(t, a.Y, b.Y)
	cameraNear(t, a.Z, b.Z)
}
func cameraConfig() render.Config {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FPS, c.Workers = 160, 80, 24, 2
	c.FrameWidth = 16
	c.CurveTolerance = .1
	c.Background = gomath.RGB(.1, .2, .3)
	return c
}

func TestProjectFrameNoCameraFastPath(t *testing.T) {
	c := cameraConfig()
	f := &gomath.Frame{Time: .25, Objects: []*gomath.Object{gomath.Square(1)}}
	projected, effective, err := render.ProjectFrame(f, c)
	if err != nil || projected != f || effective != c {
		t.Fatal("no-camera fastpath changed inputs", err)
	}
	if allocs := testing.AllocsPerRun(10, func() { _, _, _ = render.ProjectFrame(f, c) }); allocs != 0 {
		t.Fatal("no-camera fastpath allocated", allocs)
	}
	if _, _, err := render.ProjectFrame(nil, c); err == nil {
		t.Fatal("nil frame accepted")
	}
}

func TestProjectFrameGeometryIdentityIsolationAndConfig(t *testing.T) {
	c := cameraConfig()
	vector := gomath.Rectangle(4, 2).Shift(geom.V(3, 4)).SetOpacity(.6).SetStroke(gomath.Blue, .2)
	vector.Name = "vector"
	vector.ZIndex = 3
	group := gomath.Group(vector, gomath.Group(gomath.Dot(geom.V(4, 4), .2))).SetOpacity(.4)
	camera := gomath.Camera2D{Center: geom.V(3, 4), FrameWidth: 8, Rotation: math.Pi / 2}
	f := &gomath.Frame{Time: 1.5, Camera: &camera, Objects: []*gomath.Object{group}}
	originalPath := vector.Path.Clone()
	projected, effective, err := render.ProjectFrame(f, c)
	if err != nil {
		t.Fatal(err)
	}
	expected := c
	expected.FrameWidth = 8
	if effective != expected || c.FrameWidth != 16 {
		t.Fatal("unrelated config changed", effective)
	}
	if projected == f || projected.Camera != nil || projected.Time != f.Time {
		t.Fatal("invalid projected frame")
	}
	if projected.Objects[0] == group || projected.Find(vector.ID) == vector {
		t.Fatal("input object aliased")
	}
	p := projected.Find(vector.ID)
	if p.Name != vector.Name || p.ZIndex != 3 || p.Style != vector.Style || projected.Objects[0].Style != group.Style {
		t.Fatal("identity, style or order changed")
	}
	cameraVectorNear(t, p.Center(), geom.V(0, 0))
	cameraVectorNear(t, p.Bounds().Size(), geom.V(2, 4))
	for i, contour := range originalPath.Contours {
		for j, curve := range contour.Curves {
			got := p.Path.Contours[i].Curves[j]
			pairs := [][2]geom.Vec3{{curve.P0, got.P0}, {curve.P1, got.P1}, {curve.P2, got.P2}, {curve.P3, got.P3}}
			for _, pair := range pairs {
				v := pair[0]
				cameraVectorNear(t, pair[1], geom.V(v.Y-4, 3-v.X))
			}
		}
	}
	p.Path.Contours[0].Curves[0].P1.X = 777
	p.Style.Opacity = 0
	projected.Objects[0].Children = nil
	if !reflect.DeepEqual(vector.Path, originalPath) || vector.Style.Opacity != .6 || len(group.Children) != 2 || *f.Camera != camera {
		t.Fatal("projection mutation reached source")
	}
	again, _, err := render.ProjectFrame(f, c)
	if err != nil {
		t.Fatal(err)
	}
	cameraVectorNear(t, again.Find(vector.ID).Center(), geom.V(0, 0))
	// Projected frames are idempotent when paired with their effective config.
	same, sameConfig, err := render.ProjectFrame(again, effective)
	if err != nil || same != again || sameConfig != effective {
		t.Fatal("double projection")
	}
}

func TestProjectFrameBitmapPlaneAndRaster(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pixels.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	pixels.SetRGBA(1, 0, color.RGBA{G: 255, A: 255})
	pixels.SetRGBA(0, 1, color.RGBA{B: 255, A: 255})
	pixels.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	bitmap, err := gomath.NewBitmap(pixels)
	if err != nil {
		t.Fatal(err)
	}
	object, err := gomath.NewImage(bitmap, 2)
	if err != nil {
		t.Fatal(err)
	}
	object.Shift(geom.V(2, 3)).SetOpacity(.8)
	group := gomath.Group(object).SetOpacity(.5)
	c := cameraConfig()
	c.Background = gomath.Black
	cam := gomath.Camera2D{Center: geom.V(2, 3), FrameWidth: 4, Rotation: math.Pi / 2}
	source := &gomath.Frame{Objects: []*gomath.Object{group}, Camera: &cam}
	before := object.Path.Clone()
	projected, effective, err := render.ProjectFrame(source, c)
	if err != nil {
		t.Fatal(err)
	}
	p := projected.Find(object.ID)
	if p.Bitmap != bitmap || p.ID != object.ID || p.Style != object.Style || !reflect.DeepEqual(before, object.Path) {
		t.Fatal("bitmap changed or aliasing geometry")
	}
	tl, tr, bl, err := p.ImageCorners()
	if err != nil {
		t.Fatal(err)
	}
	cameraVectorNear(t, tl, geom.V(1, 1))
	cameraVectorNear(t, tr, geom.V(1, -1))
	cameraVectorNear(t, bl, geom.V(-1, 1))
	r, err := render.NewRasterizer(effective)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Render(projected)
	if err != nil {
		t.Fatal(err)
	}
	// Camera rotates the image clockwise. The original red top-left texel is
	// now in the top-right quadrant, with both group and image opacity applied.
	pixel := got.RGBAAt(110, 10)
	if pixel.R < 100 || pixel.R > 103 || pixel.G > 2 || pixel.B > 2 {
		t.Fatal("camera bitmap orientation/opacity wrong", pixel)
	}
	// Compare against an independently authored inverse image transform.
	expectedObject := object.Copy().Shift(geom.V(-2, -3)).ApplyMatrix(geom.RotationZ(-math.Pi / 2))
	expectedGroup := gomath.Group(expectedObject).SetOpacity(.5)
	expected, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{expectedGroup}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, expected.Pix) {
		t.Fatal("camera projection differs from explicit affine image transform")
	}
}

func TestProjectFramePanZoomAndWorldStrokeWidths(t *testing.T) {
	c := cameraConfig()
	c.Background = gomath.Black
	object := gomath.Square(1).SetFill(gomath.White).SetStroke(gomath.White, .1).Shift(geom.V(4, 0))
	camera := gomath.Camera2D{Center: geom.V(4, 0), FrameWidth: 4}
	projected, effective, err := render.ProjectFrame(&gomath.Frame{Camera: &camera, Objects: []*gomath.Object{object}}, c)
	if err != nil {
		t.Fatal(err)
	}
	cameraNear(t, projected.Objects[0].Style.StrokeWidth, .1)
	r, err := render.NewRasterizer(effective)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []image.Point{{80, 40}, {99, 40}, {100, 40}} {
		if img.RGBAAt(p.X, p.Y).R < 240 {
			t.Fatal("camera zoom or stroke width lost", p)
		}
	}
	if img.RGBAAt(104, 40).R != 0 {
		t.Fatal("camera frame width not applied")
	}
}

func TestProjectFrameRejectsInvalidInputsWithoutMutation(t *testing.T) {
	c := cameraConfig()
	goodCam := gomath.Camera2D{FrameWidth: 8}
	obj := gomath.Square(1)
	cycle := gomath.Group()
	cycle.Children = []*gomath.Object{cycle}
	badPath := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(math.Inf(1), 0)}}})
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	bitmap, _ := gomath.NewBitmap(pixels)
	badImage, _ := gomath.NewImage(bitmap, 1)
	badImage.Path.Contours[0].Curves[0].P1.Y += 1
	cases := []struct {
		name  string
		frame *gomath.Frame
		c     render.Config
	}{
		{"zero camera", &gomath.Frame{Camera: &gomath.Camera2D{}, Objects: []*gomath.Object{obj}}, c},
		{"nil object", &gomath.Frame{Camera: &goodCam, Objects: []*gomath.Object{nil}}, c},
		{"cycle", &gomath.Frame{Camera: &goodCam, Objects: []*gomath.Object{cycle}}, c},
		{"shared child", &gomath.Frame{Camera: &goodCam, Objects: []*gomath.Object{gomath.Group(obj, obj)}}, c},
		{"invalid geometry", &gomath.Frame{Camera: &goodCam, Objects: []*gomath.Object{badPath}}, c},
		{"curved image", &gomath.Frame{Camera: &goodCam, Objects: []*gomath.Object{badImage}}, c},
		{"invalid config", &gomath.Frame{Camera: &goodCam}, render.Config{}},
		{"overflow scale", &gomath.Frame{Camera: &gomath.Camera2D{FrameWidth: math.SmallestNonzeroFloat64}}, c},
	}
	// Valid finite world coordinates can overflow after subtracting the center.
	huge := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(math.MaxFloat64, 0)}}})
	hugeCam := gomath.Camera2D{FrameWidth: 1, Center: geom.V(-math.MaxFloat64, 0)}
	cases = append(cases, struct {
		name  string
		frame *gomath.Frame
		c     render.Config
	}{"overflow projection", &gomath.Frame{Camera: &hugeCam, Objects: []*gomath.Object{huge}}, c})
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			camera := *test.frame.Camera
			got, effective, err := render.ProjectFrame(test.frame, test.c)
			if err == nil || got != nil || effective != test.c {
				t.Fatal("invalid projection accepted or config changed", err)
			}
			if *test.frame.Camera != camera {
				t.Fatal("failed projection mutated camera")
			}
		})
	}
	cameraNear(t, obj.Center().X, 0)
}

func TestProjectFrameArbitraryOrder(t *testing.T) {
	s := gomath.NewScene()
	if err := s.Add(gomath.Square(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCamera(gomath.Camera2D{FrameWidth: 8}); err != nil {
		t.Fatal(err)
	}
	if err := s.AnimateCameraWithRate(2, gomath.Camera2D{FrameWidth: 4, Center: geom.V(2, 1), Rotation: math.Pi}, gomath.Linear); err != nil {
		t.Fatal(err)
	}
	expected := map[float64]*gomath.Frame{}
	for _, time := range []float64{0, .3, 1, 1.8, 2, 1, .3, 0, 2} {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		p, _, err := render.ProjectFrame(f, cameraConfig())
		if err != nil {
			t.Fatal(err)
		}
		if previous := expected[time]; previous != nil && !reflect.DeepEqual(previous, p) {
			t.Fatal("projection depends on sample order", time)
		}
		expected[time] = p
	}
}

func BenchmarkProjectFrame(b *testing.B) {
	c := cameraConfig()
	camera := gomath.Camera2D{FrameWidth: 8, Center: geom.V(1, 2), Rotation: .4}
	frame := &gomath.Frame{Camera: &camera}
	for i := range 100 {
		frame.Objects = append(frame.Objects, gomath.Square(1).Shift(geom.V(float64(i%10), float64(i/10))))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := render.ProjectFrame(frame, c); err != nil {
			b.Fatal(err)
		}
	}
}
