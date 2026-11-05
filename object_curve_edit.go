package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
)

// SetAnchorMode atomically changes the cubic handles of a bounded vector
// family, preserving anchors, explicit contours, IDs, pointers and styles.
// Stored contours must be exactly connected; closed contours must have an
// explicit closing cubic. Bitmap families reject before any path is changed.
func (o *Object) SetAnchorMode(mode path.AnchorMode) error {
	if mode != path.AnchorJagged && mode != path.AnchorSmooth {
		return fmt.Errorf("anchor mode must be jagged or smooth")
	}
	return o.editCurveFamily(func(p path.Path) (path.Path, error) { return p.WithAnchorMode(mode) }, mode, 0)
}
func (o *Object) MakeSmooth() error { return o.SetAnchorMode(path.AnchorSmooth) }
func (o *Object) MakeJagged() error { return o.SetAnchorMode(path.AnchorJagged) }

// PrepareForNonlinearTransform raises each nonempty node's complete path to
// at least minimum cubics, using Manim's global equal-parameter allocation
// across explicit contours. Existing cubics are never removed. Point-only
// single contours become constant cubics; mixed trailing points stay points.
// Call ApplyPointwise/ApplyComplex afterwards. This is explicit preparation,
// not error-controlled adaptive nonlinear subdivision.
func (o *Object) PrepareForNonlinearTransform(minimum int) error {
	if minimum < 0 || minimum > MaxAnnotationCurves {
		return fmt.Errorf("nonlinear preparation minimum must be in [0,%d]", MaxAnnotationCurves)
	}
	return o.editCurveFamily(func(p path.Path) (path.Path, error) { return prepareCurvePath(p, minimum) }, "", minimum)
}

func (o *Object) editCurveFamily(edit func(path.Path) (path.Path, error), mode path.AnchorMode, minimum int) error {
	if _, err := annotationBounds(o); err != nil {
		return err
	}
	input, output, contours, largest := 0, 0, 0, 0
	unsupported := false
	o.Walk(func(v *Object) {
		unsupported = unsupported || v.Bitmap != nil
		n := 0
		for _, c := range v.Path.Contours {
			n += len(c.Curves)
			largest = max(largest, len(c.Curves))
		}
		input += n
		contours += len(v.Path.Contours)
		if mode != "" {
			output += n
		} else if n > 0 || len(v.Path.Contours) == 1 {
			output += max(n, minimum)
		}
	})
	if unsupported {
		return fmt.Errorf("curve editing requires vector objects without bitmap nodes")
	}
	work := 4*input + 8*output + contours
	if mode == path.AnchorSmooth {
		work = 28*input + contours
	}
	if work > 1<<20 || output > MaxAnnotationCurves || 96*(input+output)+56*contours+40*(largest+1) > 12<<20 {
		return fmt.Errorf("curve family edit exceeds aggregate curve/work/storage budget")
	}
	type change struct {
		o *Object
		p path.Path
	}
	var changes []change
	var failure error
	o.Walk(func(v *Object) {
		if failure != nil {
			return
		}
		q, e := edit(v.Path)
		if e != nil {
			failure = e
			return
		}
		changes = append(changes, change{v, q})
	})
	if failure != nil {
		return failure
	}
	for _, change := range changes {
		change.o.Path = change.p
	}
	return nil
}

func prepareCurvePath(p path.Path, minimum int) (path.Path, error) {
	if len(p.Contours) > 4096 {
		return path.Path{}, fmt.Errorf("nonlinear preparation exceeds per-path contour budget")
	}
	for _, c := range p.Contours {
		check := func(v geom.Vec3) bool {
			return v.Finite() && math.Abs(v.X) <= 1e9 && math.Abs(v.Y) <= 1e9 && math.Abs(v.Z) <= 1e9
		}
		if !check(c.Start) {
			return path.Path{}, fmt.Errorf("curve edit start exceeds coordinate bounds")
		}
		previous := c.Start
		for _, curve := range c.Curves {
			if curve.P0 != previous || !check(curve.P0) || !check(curve.P1) || !check(curve.P2) || !check(curve.P3) {
				return path.Path{}, fmt.Errorf("curve edit requires exact joins and coordinates within +/-1e9")
			}
			previous = curve.P3
		}
	}
	n := 0
	for _, c := range p.Contours {
		n += len(c.Curves)
	}
	if n == 0 {
		if len(p.Contours) > 1 && minimum > 0 {
			return path.Path{}, fmt.Errorf("cannot allocate nonlinear preparation across several point-only contours")
		}
		q := p.Clone()
		if len(q.Contours) == 1 {
			c, e := q.Contours[0].WithCurveCount(minimum)
			if e != nil {
				return path.Path{}, e
			}
			q.Contours[0] = c
		}
		return q, nil
	}
	total := max(n, minimum)
	q := path.Path{Contours: make([]path.Contour, len(p.Contours))}
	previous, allocated := 0, 0
	for i, c := range p.Contours {
		out := path.Contour{Start: c.Start, Closed: c.Closed}
		end := int((int64(previous+len(c.Curves))*int64(total) + int64(n-1)) / int64(n))
		out.Curves = make([]path.Cubic, 0, end-allocated)
		for _, curve := range c.Curves {
			next := int((int64(previous+1)*int64(total) + int64(n-1)) / int64(n))
			parts := next - allocated
			single, e := (path.Contour{Start: curve.P0, Curves: []path.Cubic{curve}}).WithCurveCount(parts)
			if e != nil {
				return path.Path{}, e
			}
			out.Curves = append(out.Curves, single.Curves...)
			previous++
			allocated = next
		}
		q.Contours[i] = out
	}
	return q, nil
}
