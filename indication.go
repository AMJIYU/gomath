// Growing, indicate, wiggle and wave formulas are derived from Manim Community
// 0.22.1 animation/growing.py and animation/indication.py. MIT notices: LICENSES/.
package gomath

import (
	"fmt"
	"gomath/geom"
	"math"
)

type poseTrack struct {
	kind                         uint8
	origin, edge, direction      geom.Vec3
	center, fromEdge, fromArrow  bool
	explicitRotation             bool
	scale, angle, wiggles, width float64
	ripples                      int
	wave                         RateFunc
	xMin, xMax                   float64
}

func GrowFromPoint(object *Object, point geom.Vec3) Animation {
	return Animation{object: object, mode: growing, rate: Smooth, end: 1, pose: &poseTrack{kind: growing, origin: point}}
}
func GrowFromCenter(object *Object) Animation {
	a := GrowFromPoint(object, geom.Vec3{})
	a.pose.center = true
	return a
}

// GrowFromEdge uses the sign of each direction component to select a bounding
// box face/corner; a zero component selects the coordinate at its center.
func GrowFromEdge(object *Object, edge geom.Vec3) Animation {
	a := GrowFromPoint(object, geom.Vec3{})
	a.pose.fromEdge, a.pose.edge = true, edge
	return a
}

// GrowArrow grows from the first path's start, including child paths in a
// standard GoMath Arrow. It scales the entire vector arrow, including its tip.
func GrowArrow(object *Object) Animation {
	a := GrowFromPoint(object, geom.Vec3{})
	a.pose.fromArrow = true
	return a
}

// SpinInFromNothing grows from center along spiral paths with a pi/2 rotation.
func SpinInFromNothing(object *Object) Animation { return SpinIn(object, math.Pi/2) }
func SpinIn(object *Object, angle float64) Animation {
	a := GrowFromCenter(object)
	a.pose.angle = angle
	return a
}

// WithPointColor colors the initial collapsed vector shape of a growing
// animation. Invalid colors or application to other animations reject Play.
func (a Animation) WithPointColor(color Color) Animation { a.pointColor = &color; return a }

func Indicate(object *Object) Animation { return IndicateWith(object, 1.2, RGB(1, 1, 0)) }
func IndicateWith(object *Object, scale float64, color Color) Animation {
	if object == nil {
		return Transform(nil, nil)
	}
	return Transform(object, object.Copy().Scale(scale).SetColor(color)).WithRate(ThereAndBack)
}

type WiggleOptions struct {
	Scale, Angle, Wiggles float64
	// Nil centers use the original object's bounding-box center.
	ScaleCenter, RotationCenter *geom.Vec3
}

func DefaultWiggleOptions() WiggleOptions {
	return WiggleOptions{Scale: 1.1, Angle: .02 * math.Pi, Wiggles: 6}
}

// WiggleObject temporarily scales and rotates around the captured center.
// It is named separately from Wiggle, the exported scalar rate function.
func WiggleObject(object *Object) Animation { return WiggleObjectWith(object, DefaultWiggleOptions()) }
func WiggleObjectWith(object *Object, opt WiggleOptions) Animation {
	p := &poseTrack{kind: wiggleObject, scale: opt.Scale, angle: opt.Angle, wiggles: opt.Wiggles, center: true}
	// Store supplied values, never the caller-owned center pointers.
	if opt.ScaleCenter != nil {
		p.origin = *opt.ScaleCenter
		p.center = false
	}
	if opt.RotationCenter != nil {
		p.direction = *opt.RotationCenter
		p.explicitRotation = true
	}
	return Animation{object: object, mode: wiggleObject, rate: Smooth, end: 1, pose: p}.WithDuration(2)
}

type WaveOptions struct {
	Direction            geom.Vec3
	Amplitude, TimeWidth float64
	Ripples              int
	WaveFunc             RateFunc
}

func DefaultWaveOptions() WaveOptions {
	return WaveOptions{Direction: geom.V(0, 1), Amplitude: .2, TimeWidth: 1, Ripples: 1, WaveFunc: Smooth}
}
func ApplyWave(object *Object) Animation { return ApplyWaveWith(object, DefaultWaveOptions()) }
func ApplyWaveWith(object *Object, opt WaveOptions) Animation {
	return Animation{object: object, mode: waving, rate: Smooth, end: 1, pose: &poseTrack{kind: waving, direction: opt.Direction, scale: opt.Amplitude, width: opt.TimeWidth, ripples: opt.Ripples, wave: opt.WaveFunc}}.WithDuration(2)
}

func preparePose(a Animation, from, to *Object) (*poseTrack, error) {
	if a.pointColor != nil && (a.mode != growing || !a.pointColor.valid()) {
		return nil, fmt.Errorf("point color requires a growing animation and valid color")
	}
	if a.pose == nil {
		return nil, nil
	}
	p := *a.pose
	if !p.origin.Finite() || !p.edge.Finite() || !p.direction.Finite() || !finite(p.angle) || !finite(p.scale) || !finite(p.wiggles) || !finite(p.width) {
		return nil, fmt.Errorf("animation pose parameters must be finite")
	}
	hasBitmap := false
	from.Walk(func(o *Object) { hasBitmap = hasBitmap || o.Bitmap != nil })
	if hasBitmap {
		return nil, fmt.Errorf("growing/wiggle/wave currently require vector objects")
	}
	bounds := from.Bounds()
	if !bounds.Valid {
		return nil, fmt.Errorf("animation pose requires visible vector geometry")
	}
	if p.center {
		p.origin = bounds.Center()
	}
	if p.kind == growing {
		if p.fromEdge {
			selectAxis := func(lo, hi, center, direction float64) float64 {
				if direction > 0 {
					return hi
				}
				if direction < 0 {
					return lo
				}
				return center
			}
			center := bounds.Center()
			p.origin = geom.Vec3{X: selectAxis(bounds.Min.X, bounds.Max.X, center.X, p.edge.X), Y: selectAxis(bounds.Min.Y, bounds.Max.Y, center.Y, p.edge.Y), Z: selectAxis(bounds.Min.Z, bounds.Max.Z, center.Z, p.edge.Z)}
		}
		if p.fromArrow {
			found := false
			from.Walk(func(o *Object) {
				if !found && len(o.Path.Contours) > 0 {
					p.origin = o.Path.Contours[0].Start
					found = true
				}
			})
		}
		from.ApplyMatrix(geom.Scaling(0, 0, 0)).Shift(p.origin)
		if a.pointColor != nil {
			from.SetColor(*a.pointColor)
		}
	} else if p.kind == wiggleObject {
		if p.scale <= 0 || p.wiggles < 0 || p.wiggles > 1e6 {
			return nil, fmt.Errorf("wiggle scale must be positive and count in [0,1000000]")
		}
		if !p.explicitRotation {
			p.direction = bounds.Center()
		}
	} else if p.kind == waving {
		if p.width <= 0 || p.width > 1e6 || p.ripples < 1 || p.ripples > 1024 || p.wave == nil || p.direction.Norm() == 0 || !finite(p.direction.Norm()) || bounds.Max.X <= bounds.Min.X {
			return nil, fmt.Errorf("wave requires nonzero direction, width in (0,1000000], 1..1024 ripples, a wave function and positive shape width")
		}
		p.direction = p.direction.Unit().Mul(p.scale)
		p.xMin, p.xMax = bounds.Min.X, bounds.Max.X
	} else if p.kind == passingFlash {
		if p.width <= 0 || p.width > 1e6 {
			return nil, fmt.Errorf("passing flash width must be in (0,1000000]")
		}
	}
	if !p.origin.Finite() {
		return nil, fmt.Errorf("animation pose origin exceeds finite range")
	}
	return &p, nil
}

func (p *poseTrack) apply(object *Object, alpha float64) error {
	switch p.kind {
	case passingFlash:
		upper := (1 + p.width) * clamp(alpha)
		lo, hi := clamp(upper-p.width), clamp(upper)
		var failure error
		object.Walk(func(o *Object) {
			if failure != nil {
				return
			}
			o.Path, failure = o.Path.TrimByParameter(lo, hi)
		})
		if failure != nil {
			return failure
		}
	case growing:
		if p.angle != 0 {
			object.ApplyMatrix(geom.About(p.origin, geom.RotationZ((alpha-1)*p.angle)))
		}
	case wiggleObject:
		scale := 1 + (p.scale-1)*ThereAndBack(alpha)
		angle := ThereAndBack(alpha) * math.Sin(p.wiggles*math.Pi*alpha) * p.angle
		object.ApplyMatrix(geom.About(p.origin, geom.Scaling(scale, scale, scale))).ApplyMatrix(geom.About(p.direction, geom.RotationZ(angle)))
	case waving:
		var bad bool
		point := func(v geom.Vec3) geom.Vec3 {
			x := (v.X - p.xMin) / (p.xMax - p.xMin)
			upper := (1 + p.width) * alpha
			t := 1 - (x-(upper-p.width))/p.width
			nudge := 0.0
			if t > 0 && t < 1 {
				phases := 2 * p.ripples
				phase := int(t * float64(phases))
				if phase == 0 {
					nudge = p.wave(t * float64(phases))
				} else if phase == phases-1 {
					t -= float64(phase) / float64(phases)
					nudge = (1 - p.wave(t*float64(phases))) * float64(2*(p.ripples%2)-1)
				} else {
					phase = (phase - 1) / 2
					t -= float64(2*phase+1) / float64(phases)
					nudge = (1 - 2*p.wave(t*float64(p.ripples))) * float64(1-2*(phase%2))
				}
			}
			if !finite(nudge) {
				bad = true
				return v
			}
			return v.Add(p.direction.Mul(nudge))
		}
		object.Walk(func(o *Object) {
			for i := range o.Path.Contours {
				c := &o.Path.Contours[i]
				c.Start = point(c.Start)
				for j := range c.Curves {
					b := &c.Curves[j]
					// Match VMobject.apply_function's default handle mapping.
					h1 := b.P0.Add(b.P1.Sub(b.P0).Mul(.01))
					h2 := b.P3.Add(b.P2.Sub(b.P3).Mul(.01))
					a, z := point(b.P0), point(b.P3)
					b.P0, b.P1, b.P2, b.P3 = a, a.Add(point(h1).Sub(a).Mul(100)), z.Add(point(h2).Sub(z).Mul(100)), z
				}
			}
		})
		if bad {
			return fmt.Errorf("wave function returned nonfinite displacement")
		}
	}
	return object.Validate()
}
