package gomath

import (
	"fmt"
	"gomath/geom"
)

// PointwiseOptions selects the contraction of cubic handles toward anchors
// before mapping, then reciprocal expansion afterwards. This approximates the
// mapped tangents without pretending nonlinear images are still exact cubics.
type PointwiseOptions struct{ HandleScale float64 }

func DefaultPointwiseOptions() PointwiseOptions { return PointwiseOptions{HandleScale: .01} }

// ApplyPointwise atomically updates vector paths, retaining all object pointers,
// IDs, topology, styles and scalar metadata. The default handle policy follows
// Manim VMobject.apply_function (MIT; see LICENSES). Mapping is about the origin.
// Gradients, clipping and bitmaps reject before invoking the callback because
// their nonlinear images cannot be represented by their existing affine frames.
//
// The bounded source-tree policy is the same as annotations: 4096 objects,
// depth64,16384 contours,65536 cubics and coordinates within +/-1e12. The mapping
// runs synchronously in tree/contour/curve order, four calls per cubic, or once
// for an empty contour's Start. Repeated anchors are not deduplicated. It must
// be deterministic and return; its external side effects cannot be rolled back.
// Errors and panics publish no geometry. No callback is retained for sampling.
func (o *Object) ApplyPointwise(mapping func(geom.Vec3) (geom.Vec3, error), options PointwiseOptions) error {
	if mapping == nil || !finite(options.HandleScale) || options.HandleScale < 1e-6 || options.HandleScale > 1 {
		return fmt.Errorf("pointwise mapping requires a callback and handle scale [1e-6,1]")
	}
	if _, err := annotationBounds(o); err != nil {
		return err
	}
	unsupported := false
	o.Walk(func(v *Object) {
		unsupported = unsupported || v.Bitmap != nil || v.Clip != nil || v.Style.FillGradient != nil
	})
	if unsupported {
		return fmt.Errorf("pointwise mapping requires solid vector paths without bitmap, clip or gradient frames")
	}
	private := o.snapshot()
	index := 0
	mapPoint := func(p geom.Vec3) (geom.Vec3, error) {
		v, err := pointwiseCall(mapping, p, index)
		index++
		if err != nil {
			return geom.Vec3{}, err
		}
		if !boundedPrimitivePoint(v) {
			return geom.Vec3{}, fmt.Errorf("pointwise result%d outside finite coordinate bounds", index-1)
		}
		return v, nil
	}
	var failure error
	private.Walk(func(v *Object) {
		if failure != nil {
			return
		}
		for ci := range v.Path.Contours {
			contour := &v.Path.Contours[ci]
			if len(contour.Curves) == 0 {
				contour.Start, failure = mapPoint(contour.Start)
				if failure != nil {
					return
				}
				continue
			}
			for j, c := range contour.Curves {
				h1 := c.P0.Add(c.P1.Sub(c.P0).Mul(options.HandleScale))
				h2 := c.P3.Add(c.P2.Sub(c.P3).Mul(options.HandleScale))
				var q [4]geom.Vec3
				for k, p := range [4]geom.Vec3{c.P0, h1, h2, c.P3} {
					q[k], failure = mapPoint(p)
					if failure != nil {
						return
					}
				}
				c.P0, c.P1 = q[0], q[0].Add(q[1].Sub(q[0]).Mul(1/options.HandleScale))
				c.P2, c.P3 = q[3].Add(q[2].Sub(q[3]).Mul(1/options.HandleScale)), q[3]
				contour.Curves[j] = c
			}
			contour.Start = contour.Curves[0].P0
		}
	})
	if failure != nil {
		return failure
	}
	if _, err := annotationBounds(private); err != nil {
		return fmt.Errorf("mapped vector paths: %w", err)
	}
	// Topology never changes; commit only after every callback and validation.
	var commit func(*Object, *Object)
	commit = func(dst, src *Object) {
		dst.Path = src.Path
		for i, child := range dst.Children {
			commit(child, src.Children[i])
		}
	}
	commit(o, private)
	return nil
}

// ApplyComplex maps XY as a complex number and preserves each input Z. The
// callback and handle/resource/error policy is identical to ApplyPointwise.
func (o *Object) ApplyComplex(mapping func(complex128) (complex128, error), options PointwiseOptions) error {
	if mapping == nil {
		return fmt.Errorf("complex mapping requires a callback")
	}
	return o.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) {
		v, err := mapping(complex(p.X, p.Y))
		return geom.Vec3{X: real(v), Y: imag(v), Z: p.Z}, err
	}, options)
}

func pointwiseCall(mapping func(geom.Vec3) (geom.Vec3, error), p geom.Vec3, index int) (point geom.Vec3, err error) {
	completed := false
	defer func() {
		if !completed {
			_ = recover()
			point = geom.Vec3{}
			err = fmt.Errorf("pointwise callback panicked at point%d", index)
		}
	}()
	point, err = mapping(p)
	completed = true
	if err != nil {
		return geom.Vec3{}, fmt.Errorf("pointwise callback at point%d: %w", index, err)
	}
	return point, nil
}
