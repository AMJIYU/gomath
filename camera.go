package gomath

import (
	"fmt"
	"gomath/geom"
	"math"
	"sort"
)

// Camera2D is an orthographic view. Center is in world units (Z must be zero),
// FrameWidth is the visible world width, and Rotation is counterclockwise in
// radians. The output aspect ratio determines the visible height. Rotations are
// interpolated as written, including full turns; no shortest-angle wrapping occurs.
// Its zero value is invalid because a renderer's default width is not known here.
type Camera2D struct {
	Center     geom.Vec3
	FrameWidth float64
	Rotation   float64
}

func (c Camera2D) Validate() error {
	if !c.Center.Finite() || c.Center.Z != 0 {
		return fmt.Errorf("camera center must be finite and have Z=0")
	}
	if !finite(c.FrameWidth) || c.FrameWidth <= 0 || !finite(c.Rotation) {
		return fmt.Errorf("camera width must be finite and positive and rotation finite")
	}
	return nil
}

type cameraSegment struct {
	start, end   float64
	from, target Camera2D
	first, last  Camera2D
	rate         RateFunc
}

type cameraTimeline struct{ segments []cameraSegment }

// SetCamera captures an instantaneous view at the current authoring cursor.
// Before the first SetCamera, frames have no camera and use renderer defaults.
// At a shared boundary the last authored camera operation takes precedence.
func (s *Scene) SetCamera(camera Camera2D) error {
	if err := camera.Validate(); err != nil {
		return err
	}
	if !finite(s.duration) || s.duration < 0 {
		return fmt.Errorf("invalid camera timeline cursor")
	}
	s.camera.segments = append(s.camera.segments, cameraSegment{
		start: s.duration, end: s.duration, from: camera, target: camera, first: camera, last: camera,
	})
	return nil
}

// AnimateCamera advances the cursor by duration, simultaneously playing the
// supplied object animations. An initial SetCamera is required. The default
// camera rate is Smooth; object rates remain those of their own animations.
func (s *Scene) AnimateCamera(duration float64, target Camera2D, animations ...Animation) error {
	return s.AnimateCameraWithRate(duration, target, Smooth, animations...)
}

// AnimateCameraWithRate uses a custom camera rate (nil means Linear). Rates must
// be pure and safe for concurrent calls, as for object animations. Finite rate
// results are clamped to [0,1]; non-finite endpoints reject authoring atomically,
// and non-finite interior results return a sampling error. Endpoint results are
// captured, so ThereAndBack holds its actual final view on subsequent waits.
func (s *Scene) AnimateCameraWithRate(duration float64, target Camera2D, rate RateFunc, animations ...Animation) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if !finite(duration) || duration <= 0 || !finite(s.duration+duration) || s.duration+duration <= s.duration {
		return fmt.Errorf("camera duration must be finite, positive and advance the cursor")
	}
	from, err := s.camera.sample(s.duration)
	if err != nil {
		return err
	}
	if from == nil {
		return fmt.Errorf("AnimateCamera requires an initial SetCamera")
	}
	if rate == nil {
		rate = Linear
	}
	seg := cameraSegment{start: s.duration, end: s.duration + duration, from: *from, target: target, rate: rate}
	if seg.first, err = seg.at(0); err != nil {
		return err
	}
	if seg.last, err = seg.at(1); err != nil {
		return err
	}
	// Play validates and stages all object tracks before changing authoring state.
	// All camera validation precedes it, so failure leaves both timelines untouched.
	if err := s.Play(duration, animations...); err != nil {
		return err
	}
	s.camera.segments = append(s.camera.segments, seg)
	return nil
}

func (s cameraSegment) at(alpha float64) (Camera2D, error) {
	a := s.rate(alpha)
	if !finite(a) {
		return Camera2D{}, fmt.Errorf("camera rate returned a non-finite value")
	}
	a = clamp(a)
	if a == 0 {
		return s.from, nil
	}
	if a == 1 {
		return s.target, nil
	}
	// Convex sums avoid overflow from subtracting opposite finite extremes.
	// Same-sign endpoints use differences, preserving equal subnormal widths.
	lerp := func(from, to float64) float64 {
		if math.Signbit(from) == math.Signbit(to) {
			return from + (to-from)*a
		}
		return (1-a)*from + a*to
	}
	c := Camera2D{
		Center: geom.Vec3{
			X: lerp(s.from.Center.X, s.target.Center.X),
			Y: lerp(s.from.Center.Y, s.target.Center.Y),
		},
		FrameWidth: lerp(s.from.FrameWidth, s.target.FrameWidth),
		Rotation:   lerp(s.from.Rotation, s.target.Rotation),
	}
	if err := c.Validate(); err != nil {
		return Camera2D{}, fmt.Errorf("interpolated camera: %w", err)
	}
	return c, nil
}

func (c cameraTimeline) sample(time float64) (*Camera2D, error) {
	if !finite(time) || time < 0 {
		return nil, fmt.Errorf("invalid camera sample time %g", time)
	}
	i := sort.Search(len(c.segments), func(i int) bool { return c.segments[i].start > time }) - 1
	if i < 0 {
		return nil, nil
	}
	seg := c.segments[i]
	camera := seg.last
	if time == seg.start && seg.end > seg.start {
		camera = seg.first
	} else if time < seg.end {
		var err error
		camera, err = seg.at((time - seg.start) / (seg.end - seg.start))
		if err != nil {
			return nil, err
		}
	}
	return &camera, nil
}

// applyCamera runs after object tracks and before updaters, exposing the sampled
// camera to updaters without sharing mutable camera state across frames.
func (s *Scene) applyCamera(frame *Frame, time float64) error {
	if frame == nil {
		return fmt.Errorf("nil frame")
	}
	camera, err := s.camera.sample(time)
	if err != nil {
		return fmt.Errorf("camera at %g: %w", time, err)
	}
	frame.Camera = camera
	return nil
}

// ProjectCamera returns independent view-space geometry with original IDs.
// It translates by -Center then rotates by -Rotation. Z and world stroke widths
// are preserved; the renderer implements zoom with Camera.FrameWidth. Bitmap
// planes and vector groups use the same affine transformation. Immutable bitmap
// assets are shared. Gradient affine frames follow the view; shared paints remain
// shared on the projected frame. Camera is cleared on the result to prevent double projection.
// A frame without a camera is returned unchanged. No input is modified on error.
func (f *Frame) ProjectCamera() (*Frame, error) {
	if f == nil {
		return nil, fmt.Errorf("nil frame")
	}
	if f.Camera == nil {
		return f, nil
	}
	camera := *f.Camera
	if err := camera.Validate(); err != nil {
		return nil, err
	}
	if err := validateObjects(f.Objects); err != nil {
		return nil, fmt.Errorf("camera input: %w", err)
	}
	q := *f
	q.Objects = snapshots(f.Objects)
	q.Camera = nil
	// Center subtraction is performed before rotation, avoiding large translation
	// terms in a composed matrix when nearby world points are viewed far from zero.
	co, si := math.Cos(camera.Rotation), math.Sin(camera.Rotation)
	project := func(v geom.Vec3) geom.Vec3 {
		x, y := v.X-camera.Center.X, v.Y-camera.Center.Y
		return geom.Vec3{X: co*x + si*y, Y: -si*x + co*y, Z: v.Z}
	}
	var gradients map[*Gradient]*Gradient
	for _, root := range q.Objects {
		root.Walk(func(o *Object) {
			if g := o.Style.FillGradient; g != nil {
				if cached := gradients[g]; cached != nil {
					o.Style.FillGradient = cached
				} else {
					paint := *g
					origin := project(geom.V(g.matrix[4], g.matrix[5]))
					paint.matrix[0], paint.matrix[1] = co*g.matrix[0]+si*g.matrix[1], -si*g.matrix[0]+co*g.matrix[1]
					paint.matrix[2], paint.matrix[3] = co*g.matrix[2]+si*g.matrix[3], -si*g.matrix[2]+co*g.matrix[3]
					paint.matrix[4], paint.matrix[5] = origin.X, origin.Y
					paint.err = paint.prepareFrame()
					if gradients == nil {
						gradients = make(map[*Gradient]*Gradient)
					}
					gradients[g] = &paint
					o.Style.FillGradient = &paint
				}
			}
			if o.Clip != nil {
				clip := &Clip{outline: o.Clip.outline.Clone(), rule: o.Clip.rule, ready: true}
				for i := range clip.outline.Contours {
					contour := &clip.outline.Contours[i]
					contour.Start = project(contour.Start)
					for j := range contour.Curves {
						curve := &contour.Curves[j]
						curve.P0, curve.P1, curve.P2, curve.P3 = project(curve.P0), project(curve.P1), project(curve.P2), project(curve.P3)
					}
				}
				clip.err = clip.outline.Validate()
				o.Clip = clip
			}
			for i := range o.Path.Contours {
				c := &o.Path.Contours[i]
				c.Start = project(c.Start)
				for j := range c.Curves {
					b := &c.Curves[j]
					b.P0, b.P1, b.P2, b.P3 = project(b.P0), project(b.P1), project(b.P2), project(b.P3)
				}
			}
		})
	}
	if err := validateObjects(q.Objects); err != nil {
		return nil, fmt.Errorf("camera projection: %w", err)
	}
	return &q, nil
}
