package path

import (
	"fmt"
	"gomath/geom"
	"math"
)

// AnchorMode selects how handles are constructed between stored anchors.
type AnchorMode string

const (
	AnchorJagged AnchorMode = "jagged"
	AnchorSmooth AnchorMode = "smooth"

	curveEditMaxCurves   = 1 << 16
	curveEditMaxContours = 1 << 12
	curveEditMaxWork     = 1 << 20
	curveEditMaxStorage  = 12 << 20
	curveEditMaxCoord    = 1e9
)

// WithCurveCount returns an independent contour containing exactly total
// explicit cubics. It never removes curves. Curve i receives
// ceil((i+1)*total/n)-ceil(i*total/n) equal-parameter subdivisions, matching
// Manim 0.22.1 insert_n_curves(total-n). It does not use arc length.
// A point-only contour expands to total constant cubics. Closed is preserved;
// an implicit closing edge is neither counted nor materialized.
// Inputs must be exactly connected, finite and bounded; see docs/CURVE_EDIT.md.
func (c Contour) WithCurveCount(total int) (Contour, error) {
	n := len(c.Curves)
	if total < n || total < 0 || total > curveEditMaxCurves || n > curveEditMaxCurves {
		return Contour{}, fmt.Errorf("curve edit: total must be between current count and %d", curveEditMaxCurves)
	}
	if err := curveEditBudget(1, n, total, 0, 4*n+8*total+1); err != nil {
		return Contour{}, err
	}
	if err := validateEditContour(c); err != nil {
		return Contour{}, err
	}
	out := Contour{Start: c.Start, Closed: c.Closed, Curves: make([]Cubic, total)}
	if n == 0 {
		for i := range out.Curves {
			out.Curves[i] = Cubic{c.Start, c.Start, c.Start, c.Start}
		}
		return out, nil
	}
	index := 0
	for i, cubic := range c.Curves {
		// int64 avoids intermediate overflow even on 32-bit architectures.
		end := int((int64(i+1)*int64(total) + int64(n-1)) / int64(n))
		parts := end - index
		if parts == 1 {
			out.Curves[index] = cubic
		} else {
			for j := range parts {
				a, b := float64(j)/float64(parts), float64(j+1)/float64(parts)
				out.Curves[index+j] = Cubic{
					cubic.At(a), editBlossom(cubic, a, a, b),
					editBlossom(cubic, a, b, b), cubic.At(b),
				}
			}
			out.Curves[index].P0 = cubic.P0
			out.Curves[end-1].P3 = cubic.P3
		}
		index = end
	}
	if err := validateEditContour(out); err != nil {
		return Contour{}, err
	}
	return out, nil
}

// editBlossom evaluates the cubic polar form, so each output is an exact
// polynomial restriction up to floating-point rounding, without flattening or
// repeatedly splitting an ever-shorter remainder.
func editBlossom(c Cubic, a, b, d float64) geom.Vec3 {
	p, q, r := c.P0.Lerp(c.P1, a), c.P1.Lerp(c.P2, a), c.P2.Lerp(c.P3, a)
	return p.Lerp(q, b).Lerp(q.Lerp(r, b), d)
}

// FromAnchors builds a contour through consecutive anchors with jagged or
// smooth handles. One anchor creates a point-only contour; zero is an error.
// Repeating the first anchor exactly at the end sets Closed. Smooth handles
// use Manim's approximate closure test independently of the Closed metadata.
// No extra closing anchor is appended. Inputs and returned storage never alias.
func FromAnchors(anchors []geom.Vec3, mode AnchorMode) (Contour, error) {
	if err := checkAnchorMode(mode); err != nil {
		return Contour{}, err
	}
	if len(anchors) == 0 || len(anchors) > curveEditMaxCurves+1 {
		return Contour{}, fmt.Errorf("curve edit: need 1..%d anchors", curveEditMaxCurves+1)
	}
	n := len(anchors) - 1
	if err := curveEditBudget(1, 0, n, n, anchorEditWork(n, mode)+len(anchors)); err != nil {
		return Contour{}, err
	}
	for i, a := range anchors {
		if !validEditPoint(a) {
			return Contour{}, fmt.Errorf("curve edit: anchor %d is non-finite or outside +/-1e9", i)
		}
	}
	out := anchorContour(anchors, mode)
	if err := validateEditContour(out); err != nil {
		return Contour{}, err
	}
	return out, nil
}

// WithAnchorMode reconstructs handles from the explicit anchors of each
// contour, preserving anchors, contour boundaries and closure metadata. Closed
// contours must have an explicit closing cubic; implicit closing edges are
// rejected rather than producing a nonsmooth seam. Point-only contours survive.
// It validates every input before allocating output and never mutates the input.
func (p Path) WithAnchorMode(mode AnchorMode) (Path, error) {
	if err := checkAnchorMode(mode); err != nil {
		return Path{}, err
	}
	if len(p.Contours) > curveEditMaxContours {
		return Path{}, fmt.Errorf("curve edit: too many contours (max %d)", curveEditMaxContours)
	}
	total, largest := 0, 0
	for _, c := range p.Contours {
		if len(c.Curves) > curveEditMaxCurves-total {
			return Path{}, fmt.Errorf("curve edit: too many curves (max %d)", curveEditMaxCurves)
		}
		total += len(c.Curves)
		largest = max(largest, len(c.Curves))
	}
	if err := curveEditBudget(len(p.Contours), total, total, largest, 4*total+anchorEditWork(total, mode)+len(p.Contours)); err != nil {
		return Path{}, err
	}
	for i, c := range p.Contours {
		if err := validateEditContour(c); err != nil {
			return Path{}, fmt.Errorf("contour %d: %w", i, err)
		}
		if c.Closed && len(c.Curves) > 0 && c.Curves[len(c.Curves)-1].P3 != c.Start {
			return Path{}, fmt.Errorf("curve edit: contour %d has an implicit closing edge", i)
		}
	}
	out := Path{Contours: make([]Contour, len(p.Contours))}
	for i, c := range p.Contours {
		anchors := make([]geom.Vec3, len(c.Curves)+1)
		anchors[0] = c.Start
		for j, cubic := range c.Curves {
			anchors[j+1] = cubic.P3
		}
		out.Contours[i] = anchorContour(anchors, mode)
		out.Contours[i].Closed = c.Closed
		if err := validateEditContour(out.Contours[i]); err != nil {
			return Path{}, fmt.Errorf("contour %d: %w", i, err)
		}
	}
	return out, nil
}

func checkAnchorMode(mode AnchorMode) error {
	if mode != AnchorJagged && mode != AnchorSmooth {
		return fmt.Errorf("curve edit: anchor mode must be jagged or smooth")
	}
	return nil
}

func anchorEditWork(n int, mode AnchorMode) int {
	if mode == AnchorSmooth {
		return 24 * n
	}
	return 4 * n
}

func curveEditBudget(contours, input, output, scratch, work int) error {
	if contours > curveEditMaxContours || input > curveEditMaxCurves || output > curveEditMaxCurves {
		return fmt.Errorf("curve edit: curve/contour limit exceeded")
	}
	if work > curveEditMaxWork {
		return fmt.Errorf("curve edit: work limit exceeded (%d)", curveEditMaxWork)
	}
	// Peak live output: 96 bytes/cubic, 56 bytes/contour; temporary anchors
	// and the two scalar Thomas arrays require at most 40*(scratch+1).
	// Conservatively also charge all input cubics, although they are borrowed.
	if 96*(input+output)+56*contours+40*(scratch+1) > curveEditMaxStorage {
		return fmt.Errorf("curve edit: storage limit exceeded (%d bytes)", curveEditMaxStorage)
	}
	return nil
}

func validEditPoint(p geom.Vec3) bool {
	return p.Finite() && math.Abs(p.X) <= curveEditMaxCoord && math.Abs(p.Y) <= curveEditMaxCoord && math.Abs(p.Z) <= curveEditMaxCoord
}

func validateEditContour(c Contour) error {
	if !validEditPoint(c.Start) {
		return fmt.Errorf("curve edit: start is non-finite or outside +/-1e9")
	}
	prev := c.Start
	for i, v := range c.Curves {
		if !validEditPoint(v.P0) || !validEditPoint(v.P1) || !validEditPoint(v.P2) || !validEditPoint(v.P3) {
			return fmt.Errorf("curve edit: curve %d is non-finite or outside +/-1e9", i)
		}
		if v.P0 != prev {
			return fmt.Errorf("curve edit: curve %d is not exactly connected", i)
		}
		prev = v.P3
	}
	return nil
}
