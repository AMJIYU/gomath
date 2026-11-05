package scenes

import (
	"fmt"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/graph"
	"gomath/mathtex"
	"gomath/network"
	"gomath/path"
	"gomath/rasterimage"
	"gomath/render"
	"gomath/three"
	"image/color"
	"math"
)

// Names returns a fresh inventory of runnable integration scenes.
func Names() []string {
	return []string{"transform", "calculus", "lissajous", "linear-algebra", "images", "dynamics", "networks", "three", "replay", "mathematics", "paths", "composition", "textures", "spectral", "primitives", "angle-marks", "arc-connections", "tangential-arcs", "annotations", "braces", "arc-braces", "pointwise", "image-pixels", "transform-paths", "svg-container-bbox", "code-display", "curve-edit", "math-parts", "matching-parts", "path-direction", "math-braces", "svg-text-gradients", "updater-lifecycle", "network-mutations", "math-arrays", "numbers", "checkerboard-surface", "radical-boxes", "curved-networks", "svg-clip-unions", "segments3d", "adaptive-streamlines", "streamline-flow", "succession", "pointcloud", "dynamic-topology", "crossfade", "shadows", "operator-layout", "clipping", "clip-rules", "fill-rules", "svg-bbox-clips", "schur", "point-light", "table-labels", "scene3d-timeline", "gradients", "logarithmic", "math-alphabets", "complex-solve", "complex-plane", "polar-plane", "point-morph", "sparse-solve", "boolean", "mesh-morph", "fractions", "bar-chart", "formula-colors", "sparse-product", "quaternion", "oriented-camera", "point-lights", "platonic-solids", "bicgstab", "multiline-math", "font-fallback", "text-markup", "reference"}
}

// BuildSource includes precompiled DT integration as well as authored scenes.
func BuildSource(name string) (render.FrameSource, error) {
	if name == "checkerboard-surface" {
		return CheckerboardSurface()
	}
	if name == "segments3d" {
		return Segments3D()
	}
	if name == "replay" {
		return ReplayOscillator()
	}
	if name == "streamline-flow" {
		return StreamlineFlow()
	}
	if name == "platonic-solids" {
		return PlatonicSolids()
	}
	if name == "scene3d-timeline" {
		return Scene3DTimeline()
	}
	if name == "quaternion" {
		return QuaternionTimeline()
	}
	if name == "oriented-camera" {
		return OrientedCameraTimeline()
	}
	if name == "point-lights" {
		return PointLightsTimeline()
	}
	if name == "mesh-morph" {
		return MeshMorph()
	}
	if name == "point-morph" {
		return PointMorph()
	}
	return Build(name)
}

// Dynamics combines sampled tracker state, redraw, a moving camera and actual
// replacement identity. All callback geometry is derived from private frames.
func Dynamics() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Trackers / redraw / moving camera / replacement")
	if err != nil {
		return nil, err
	}
	label.MoveTo(geom.V(0, 2.5)).Scale(.8)
	axes, err := graph.NewAxes(graph.Range{Min: -4, Max: 4, Step: 1}, graph.Range{Min: -1.5, Max: 1.5, Step: .5}, 10, 3)
	if err != nil {
		return nil, err
	}
	curve, err := axes.Plot(math.Sin, 200)
	if err != nil {
		return nil, err
	}
	curve.SetStroke(gomath.Blue, .04)
	value, err := gomath.NewValueTracker(-4)
	if err != nil {
		return nil, err
	}
	dot := gomath.Dot(axes.Point(-4, math.Sin(-4)), .12).SetFill(gomath.Yellow)
	if err = s.Add(label, axes.Object, curve, value.Object, dot); err != nil {
		return nil, err
	}
	id := value.ID
	if err = s.AddRedraw(dot, func(frame *gomath.Frame, time float64) (*gomath.Object, error) {
		if time >= 3 {
			return frame.Find(dot.ID).Copy(), nil
		}
		x, err := frame.Value(id)
		if err != nil {
			return nil, err
		}
		return gomath.Dot(axes.Point(x, math.Sin(x)), .12).SetFill(gomath.Yellow), nil
	}); err != nil {
		return nil, err
	}
	if err = s.SetCamera(gomath.Camera2D{FrameWidth: 14.2222222222}); err != nil {
		return nil, err
	}
	if err = s.AnimateCameraWithRate(3, gomath.Camera2D{FrameWidth: 12, Center: geom.V(.5, 0), Rotation: .08}, gomath.Smooth, gomath.ValueTo(value, 4).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	target := gomath.Square(.32).SetFill(gomath.Pink).MoveTo(axes.Point(4, math.Sin(4)))
	if err = s.Play(.7, gomath.ReplacementTransform(dot, target)); err != nil {
		return nil, err
	}
	if err = s.Play(.6, gomath.Rotate(target, math.Pi/2)); err != nil {
		return nil, err
	}
	if err = s.Wait(.4); err != nil {
		return nil, err
	}
	return s, nil
}

func Networks() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Graph layouts / vector fields / RK4 streamlines")
	if err != nil {
		return nil, err
	}
	vertices := []string{"a", "b", "c", "d", "e", "f"}
	edges := []network.Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "d"}, {From: "d", To: "e"}, {From: "e", To: "f"}, {From: "f", To: "a"}, {From: "a", To: "d"}, {From: "b", To: "e"}}
	options := network.DefaultOptions()
	options.Layout.Scale, options.Layout.Center = 1.65, geom.V(-3.1, -.2)
	g, err := network.NewDiGraph(vertices, edges, options)
	if err != nil {
		return nil, err
	}
	options.Layout.Kind, options.Layout.Seed, options.Layout.Iterations = network.Spring, 42, 150
	target, err := network.NewDiGraph(vertices, edges, options)
	if err != nil {
		return nil, err
	}
	rotation := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	vo := field.DefaultVectorOptions()
	vo.Scale, vo.MaxLength = .22, .5
	vo.ColorFunc, err = field.SpeedColor(gomath.Blue, gomath.Pink, 0, 2.5)
	if err != nil {
		return nil, err
	}
	vectors, err := field.NewVectorField(rotation, field.Range{Min: -2, Max: 2, Step: .65}, field.Range{Min: -2, Max: 2, Step: .65}, vo)
	if err != nil {
		return nil, err
	}
	so := field.DefaultStreamOptions()
	so.StepSize, so.MaxSteps = .04, 160
	streams, err := field.NewStreamLines(rotation, []geom.Vec3{geom.V(.6, 0), geom.V(1.2, 0), geom.V(1.8, 0)}, so)
	if err != nil {
		return nil, err
	}
	streams.Object.SetStroke(gomath.Yellow, .022)
	flow := gomath.Group(vectors.Object, streams.Object).Shift(geom.V(3.1, -.2))
	if err = s.Add(label); err != nil {
		return nil, err
	}
	if err = s.Play(1.2, gomath.Create(g.Object), gomath.Create(flow)); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Transform(g.Object, target.Object)); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.Rotate(flow, math.Pi/3)); err != nil {
		return nil, err
	}
	if err = s.Wait(.4); err != nil {
		return nil, err
	}
	return s, nil
}

// Three exercises the real CPU depth renderer inside a regular scene. Meshes
// and the renderer are immutable after construction; Render owns frame buffers.
func Three() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Pure Go 3D / clipping, depth and Lambert lighting")
	if err != nil {
		return nil, err
	}
	box, err := three.Box(1.8, 1.8, 1.8)
	if err != nil {
		return nil, err
	}
	sphere, err := three.Sphere(1, 32, 16)
	if err != nil {
		return nil, err
	}
	surface, err := three.ParametricSurface(-3.5, 3.5, -2, 2, 32, 20, func(u, v float64) geom.Vec3 {
		return geom.Vec3{X: u, Y: -1.3 + .2*math.Sin(2*u)*math.Cos(v), Z: -v}
	})
	if err != nil {
		return nil, err
	}
	config := three.DefaultConfig(480, 270)
	config.CullBackfaces = false
	config.Supersampling = 2
	r, err := three.NewRenderer(config)
	if err != nil {
		return nil, err
	}
	draw := func(_ *gomath.Frame, t float64) (*gomath.Object, error) {
		camera := three.Camera{Position: geom.Vec3{X: 5 + .3*math.Sin(t), Y: 3.5, Z: 7}, Up: geom.Vec3{Y: 1}, Projection: three.Perspective, FOVY: math.Pi / 3, Near: .1, Far: 100}
		a, b, c := three.NewInstance(box), three.NewInstance(sphere), three.NewInstance(surface)
		a.Model = geom.Translation(geom.V(-1.5, 0)).Mul(geom.Rotation(geom.Vec3{Y: 1}, t*.9))
		b.Model = geom.Translation(geom.V(1.5, .2*math.Sin(t*2)))
		a.Material.Color, b.Material.Color, c.Material.Color = color.NRGBA{R: 70, G: 160, B: 245, A: 255}, color.NRGBA{R: 245, G: 100, B: 155, A: 255}, color.NRGBA{R: 80, G: 170, B: 150, A: 255}
		pixels, err := r.Render(camera, c, a, b)
		if err != nil {
			return nil, err
		}
		o, err := rasterimage.New(pixels, 11.8)
		if err != nil {
			return nil, err
		}
		return o.MoveTo(geom.V(0, -.5)), nil
	}
	object, err := draw(nil, 0)
	if err != nil {
		return nil, err
	}
	if err = s.Add(object, label); err != nil {
		return nil, err
	}
	if err = s.AddRedraw(object, draw); err != nil {
		return nil, err
	}
	if err = s.Wait(4); err != nil {
		return nil, err
	}
	return s, nil
}

// ReplayOscillator integrates x”=-x with symplectic Euler. The integrated
// scalar state is separate from display geometry; no frame history is shared.
func ReplayOscillator() (*gomath.Replay, error) {
	s := gomath.NewScene()
	label, err := title("DT integration / fixed steps and checkpoint replay")
	if err != nil {
		return nil, err
	}
	velocity, err := gomath.NewValueTracker(0)
	if err != nil {
		return nil, err
	}
	state := gomath.Group(velocity.Object)
	state.Scalar = 2.4
	dot := gomath.Dot(geom.V(2.4, 0), .25).SetFill(gomath.Pink)
	var builder path.Builder
	builder.MoveTo(geom.V(-4, 0))
	builder.LineTo(geom.V(2.4, 0))
	spring := gomath.NewObject(builder.Path()).SetStroke(gomath.Blue, .045)
	formula, err := mathtex.New(`\frac{d^{2}x}{dt^{2}}=-x`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.MoveTo(geom.V(0, -2))
	if err = s.Add(label, state, spring, dot, formula); err != nil {
		return nil, err
	}
	id := state.ID
	if err = s.AddRedraw(dot, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		x, err := f.Value(id)
		if err != nil {
			return nil, err
		}
		return gomath.Dot(geom.V(x, 0), .25).SetFill(gomath.Pink), nil
	}); err != nil {
		return nil, err
	}
	if err = s.AddRedraw(spring, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		x, err := f.Value(id)
		if err != nil {
			return nil, err
		}
		var b path.Builder
		b.MoveTo(geom.V(-4, 0))
		for i := 1; i <= 32; i++ {
			y := .18
			if i%2 == 0 {
				y = -y
			}
			if i == 32 {
				y = 0
			}
			b.LineTo(geom.V(-4+(x+4)*float64(i)/32, y))
		}
		return gomath.NewObject(b.Path()).SetStroke(gomath.Blue, .045), nil
	}); err != nil {
		return nil, err
	}
	if err = s.Wait(6); err != nil {
		return nil, err
	}
	return gomath.CompileReplay(s, gomath.DefaultReplayOptions(), gomath.Integration{Object: state, Update: func(o *gomath.Object, dt, _ float64) error {
		if len(o.Children) != 1 {
			return fmt.Errorf("oscillator state topology changed")
		}
		o.Children[0].Scalar -= o.Scalar * dt
		o.Scalar += o.Children[0].Scalar * dt
		return nil
	}})
}
