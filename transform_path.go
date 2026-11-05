package gomath

// Point-circle geometry derived from Manim Community's MIT-licensed
// manim/utils/paths.py, commit 16ea443e2a5f1d79db0a721161411ce916d6e733.
// Copyright (c) 2018 3Blue1Brown LLC
// Copyright (c) 2024, the Manim Community Developers
// See LICENSES/ and THIRD_PARTY_NOTICES.md for the upstream MIT notice.

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
)

// Transform path budgets apply only to nonstraight, explicitly selected arcs.
const (
	MaxTransformPathArc        = 2 * math.Pi
	MaxTransformPathCoordinate = 1e9
	MaxTransformPathProgress   = 1e6
	MaxTransformPathPoints     = 1 << 20
	MaxTransformPathWork       = 16 << 20
)

type transformPathOptions struct {
	angle float64
	axis  geom.Vec3
}

// WithPathArc selects Manim's per-point circular path in radians. Play checks
// the options atomically. |angle| < .01 retains the established straight path;
// full turns and near-full-turn singular centers are rejected. Options apply
// only to Transform, ReplacementTransform and TransformFromCopy leaves.
func (a Animation) WithPathArc(angle float64) Animation {
	options := transformPathOptions{axis: geom.Vec3{Z: 1}}
	if a.transformPath != nil {
		options = *a.transformPath
	}
	options.angle = angle
	a.transformPath = &options
	return a
}

// WithPathArcAxis captures a finite axis by value. It is normalized robustly at
// Play; a zero axis falls back to OUT, matching Manim. It does not select an
// angle by itself. Tilted axes support vector geometry; planar paint has the
// narrower bounds described in docs/TRANSFORM_PATHS.md.
func (a Animation) WithPathArcAxis(axis geom.Vec3) Animation {
	options := transformPathOptions{}
	if a.transformPath != nil {
		options = *a.transformPath
	}
	options.axis = axis
	a.transformPath = &options
	return a
}

func ClockwiseTransform(object, target *Object) Animation {
	return Transform(object, target).WithPathArc(-math.Pi)
}

func CounterclockwiseTransform(object, target *Object) Animation {
	return Transform(object, target).WithPathArc(math.Pi)
}

// This immutable value is safe to share with frozen replay tracks and frames.
type transformArcTrack struct {
	angle, cotHalf float64
	axis           geom.Vec3
	reversed       bool
}

func prepareTransformArc(a Animation) (*transformArcTrack, error) {
	if a.transformPath == nil {
		return nil, nil
	}
	if a.group != nil || (a.mode != morph && a.mode != replacement && a.mode != copyMorph) || a.relative != nil || a.pose != nil || a.rotation != nil {
		return nil, fmt.Errorf("transform path options require Transform, ReplacementTransform or TransformFromCopy")
	}
	op := *a.transformPath
	if !finite(op.angle) || math.Abs(op.angle) > MaxTransformPathArc || !op.axis.Finite() {
		return nil, fmt.Errorf("transform path needs a finite axis and finite angle within +/-2pi")
	}
	if math.Abs(op.angle) < .01 {
		return nil, nil
	}
	if math.Abs(math.Sin(op.angle/2)) < 1e-6 {
		return nil, fmt.Errorf("transform path angle has singular or ill-conditioned full-turn centers")
	}
	scale := math.Max(math.Abs(op.axis.X), math.Max(math.Abs(op.axis.Y), math.Abs(op.axis.Z)))
	axis := geom.Vec3{Z: 1}
	if scale > 0 {
		// Divide components separately: 1/scale can overflow for subnormals.
		axis = geom.Vec3{X: op.axis.X / scale, Y: op.axis.Y / scale, Z: op.axis.Z / scale}
		axis = axis.Mul(1 / math.Sqrt(axis.Dot(axis)))
	}
	cot := 0.0
	if op.angle != math.Pi { // Preserve upstream's exact positive-pi branch.
		cot = 1 / math.Tan(op.angle/2)
	}
	return &transformArcTrack{angle: op.angle, cotHalf: cot, axis: axis, reversed: a.mode == copyMorph}, nil
}

func transformPathPointBound(p geom.Vec3) bool {
	return p.Finite() && math.Abs(p.X) <= MaxTransformPathCoordinate && math.Abs(p.Y) <= MaxTransformPathCoordinate && math.Abs(p.Z) <= MaxTransformPathCoordinate
}

// Bound source alignment work before path.Align allocates/subdivides. Include
// unmatched children, which alignment creates with collapsed vector geometry.
func validateTransformArcInputs(a, b *Object) error {
	points, work := 0, 0
	checkPath := func(a, b path.Path) error {
		for _, p := range []path.Path{a, b} {
			for _, c := range p.Contours {
				points += 1 + 4*len(c.Curves)
				if points > MaxTransformPathPoints || !transformPathPointBound(c.Start) {
					return fmt.Errorf("transform path exceeds point/coordinate budget")
				}
				for _, q := range c.Curves {
					for _, v := range [4]geom.Vec3{q.P0, q.P1, q.P2, q.P3} {
						if !transformPathPointBound(v) {
							return fmt.Errorf("transform path coordinates must be finite and within +/-1e9")
						}
					}
				}
			}
		}
		for i := range max(len(a.Contours), len(b.Contours)) {
			x, y := 0, 0
			if i < len(a.Contours) {
				x = len(a.Contours[i].Curves)
			}
			if i < len(b.Contours) {
				y = len(b.Contours[i].Curves)
			}
			n, gap := max(x, y), max(x, y)-min(x, y)
			if n > MaxTransformPathWork-work || (n > 0 && gap > (MaxTransformPathWork-work-n)/n) {
				return fmt.Errorf("transform path alignment exceeds bounded subdivision work")
			}
			work += n + gap*n
		}
		return nil
	}
	var visit func(*Object, *Object) error
	visit = func(a, b *Object) error {
		if a == nil {
			a = &Object{}
		}
		if b == nil {
			b = &Object{}
		}
		if err := checkPath(a.Path, b.Path); err != nil {
			return err
		}
		if a.Clip != nil && b.Clip != nil {
			if err := checkPath(a.Clip.outline, b.Clip.outline); err != nil {
				return err
			}
		}
		for i := range max(len(a.Children), len(b.Children)) {
			var x, y *Object
			if i < len(a.Children) {
				x = a.Children[i]
			}
			if i < len(b.Children) {
				y = b.Children[i]
			}
			if err := visit(x, y); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(a, b)
}

func validateTransformArcPaint(a, b *Object, arc *transformArcTrack) error {
	if a.Clip != nil || a.Style.FillGradient != nil {
		if arc.axis.X != 0 || arc.axis.Y != 0 {
			return fmt.Errorf("transform path clips/gradients require a planar OUT/IN axis")
		}
	}
	if a.Clip != nil {
		for _, c := range []*Clip{a.Clip, b.Clip} {
			if c == nil {
				return fmt.Errorf("transform path requires matching clip presence")
			}
			for _, contour := range c.outline.Contours {
				if contour.Start.Z != 0 {
					return fmt.Errorf("transform path clips require planar z=0 geometry")
				}
				for _, q := range contour.Curves {
					if q.P0.Z != 0 || q.P1.Z != 0 || q.P2.Z != 0 || q.P3.Z != 0 {
						return fmt.Errorf("transform path clips require planar z=0 geometry")
					}
				}
			}
		}
	}
	if g, h := a.Style.FillGradient, b.Style.FillGradient; g != nil {
		if h == nil {
			return fmt.Errorf("transform path requires matching gradient presence")
		}
		for i := range 4 {
			if g.matrix[i] != h.matrix[i] {
				return fmt.Errorf("transform path gradients support translation frames only; changing affine bases is unsupported")
			}
		}
	}
	for i := range a.Children {
		if err := validateTransformArcPaint(a.Children[i], b.Children[i], arc); err != nil {
			return err
		}
	}
	return nil
}

func (arc *transformArcTrack) point(a, b geom.Vec3, progress float64) geom.Vec3 {
	if arc.reversed {
		a, b, progress = b, a, 1-progress
	}
	center := a.Add(b.Sub(a).Mul(.5)).Add(arc.axis.Cross(b.Sub(a).Mul(.5)).Mul(arc.cotHalf))
	v := a.Sub(center)
	s, c := math.Sincos(progress * arc.angle)
	return center.Add(v.Mul(c)).Add(arc.axis.Cross(v).Mul(s)).Add(arc.axis.Mul(arc.axis.Dot(v) * (1 - c)))
}

func (arc *transformArcTrack) interpolatePath(dst *path.Path, a, b path.Path, progress float64) {
	// First establish owned/aligned buffers and the standard contour flags.
	interpolatePrivatePath(dst, a, b, progress)
	for i := range a.Contours {
		x, y := a.Contours[i], b.Contours[i]
		out := &dst.Contours[i]
		out.Start = arc.point(x.Start, y.Start, progress)
		if progress <= 0 {
			out.Closed = x.Closed
		} else if progress >= 1 {
			out.Closed = y.Closed
		}
		for j := range x.Curves {
			p, q := x.Curves[j], y.Curves[j]
			out.Curves[j] = path.Cubic{P0: arc.point(p.P0, q.P0, progress), P1: arc.point(p.P1, q.P1, progress), P2: arc.point(p.P2, q.P2, progress), P3: arc.point(p.P3, q.P3, progress)}
		}
	}
}

func (arc *transformArcTrack) apply(dst, a, b *Object, progress float64) error {
	if math.Abs(progress) > MaxTransformPathProgress {
		return fmt.Errorf("transform path rate output exceeds +/-1e6")
	}
	// Work on a private tree: even a late bitmap/paint error cannot expose a
	// partially written result to authoring or another concurrently sampled frame.
	state := a.snapshot()
	interpolateTree(state, a, b, progress)
	var visit func(*Object, *Object, *Object) error
	visit = func(dst, a, b *Object) error {
		arc.interpolatePath(&dst.Path, a.Path, b.Path, progress)
		if a.Clip != nil {
			clip := &Clip{rule: a.Clip.rule, ready: true}
			arc.interpolatePath(&clip.outline, a.Clip.outline, b.Clip.outline, progress)
			clip.err = validateClipOutline(clip.outline)
			dst.Clip = clip
		}
		if g := a.Style.FillGradient; g != nil {
			h := b.Style.FillGradient
			q := *g
			p := arc.point(geom.V(g.matrix[4], g.matrix[5]), geom.V(h.matrix[4], h.matrix[5]), progress)
			q.matrix[4], q.matrix[5] = p.X, p.Y
			q.err = q.prepareFrame()
			dst.Style.FillGradient = &q
		}
		if err := dst.ValidateVisual(); err != nil {
			return fmt.Errorf("transform path frame: %w", err)
		}
		for i := range a.Children {
			if err := visit(dst.Children[i], a.Children[i], b.Children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(state, a, b); err != nil {
		return err
	}
	assignTree(dst, state)
	return nil
}
