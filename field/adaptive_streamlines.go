package field

import (
	"context"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
)

// AdaptiveStreamOptions separates numerical options from rendering style.
// Integration.Limits are shared by the entire constructor, not reset per seed.
// ColorFunc colors an entire line from its valid seed sample, exactly once;
// zero-duration lines make no field/color call and retain Color.
type AdaptiveStreamOptions struct {
	Integration AdaptiveOptions
	StrokeWidth float64
	Color       gomath.Color
	ColorFunc   ColorFunc
}

func DefaultAdaptiveStreamOptions() AdaptiveStreamOptions {
	return AdaptiveStreamOptions{Integration: DefaultAdaptiveOptions(), StrokeWidth: .025, Color: gomath.Blue}
}

// AdaptiveLine contains an immutable numerical result and a borrowed child of
// AdaptiveStreamLines.Object. Add only the owning root to a scene, or Copy it.
type AdaptiveLine struct {
	Result *AdaptiveResult
	Object *gomath.Object
}
type AdaptiveStreamLines struct {
	Object *gomath.Object
	lines  []AdaptiveLine
	stats  AdaptiveStats
}

func (s *AdaptiveStreamLines) Lines() []AdaptiveLine {
	if s == nil {
		return nil
	}
	return append([]AdaptiveLine(nil), s.lines...)
}
func (s *AdaptiveStreamLines) Stats() AdaptiveStats {
	if s == nil {
		return AdaptiveStats{}
	}
	return s.stats
}

// NewAdaptiveStreamLines integrates seeds in input order, then builds actual
// polyline children without resampling the nonuniform accepted points. All
// callbacks run synchronously here, never during rendering; none is retained.
// Each line's explicit physical termination is retained. Any configuration,
// resource, context or geometry error returns nil, never an incomplete set.
// A conservative 1024*(MaxAcceptedSteps+len(seeds))+64KiB modeled-storage reserve
// includes numerical arrays, geometry copies/records and fixed numeric scratch;
// this is not an RSS/cumulative-allocation cap (see ADAPTIVE_FIELDS.md).
func NewAdaptiveStreamLines(ctx context.Context, fn Function, seeds []geom.Vec3, o AdaptiveStreamOptions) (*AdaptiveStreamLines, error) {
	if !positive(o.StrokeWidth) {
		return nil, fmt.Errorf("%w: stroke width", ErrAdaptiveOptions)
	}
	if e := colorValid(o.Color); e != nil {
		return nil, fmt.Errorf("%w: color: %v", ErrAdaptiveOptions, e)
	}
	b, e := adaptivePrepare(ctx, fn, seeds, o.Integration, true)
	if e != nil {
		return nil, e
	}
	// Snapshot before any field or color callback. A synchronous caller callback
	// may mutate its original seed slice; that must not change later validated
	// seeds or cause an out-of-domain seed evaluation.
	ownedSeeds := append([]geom.Vec3(nil), seeds...)
	s := &AdaptiveStreamLines{lines: make([]AdaptiveLine, 0, len(ownedSeeds))}
	objects := make([]*gomath.Object, 0, len(ownedSeeds))
	for i, seed := range ownedSeeds {
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		r, e := integrateAdaptive(fn, seed, o.Integration, b)
		if e != nil {
			return nil, fmt.Errorf("seed %d: %w", i, e)
		}
		color := o.Color
		if o.ColorFunc != nil && r.seedValid {
			color = o.ColorFunc(seed, r.seedVector, r.seedSpeed)
			if e = ctx.Err(); e != nil {
				return nil, e
			}
			if e = colorValid(color); e != nil {
				return nil, fmt.Errorf("%w: seed %d color: %v", ErrAdaptiveOptions, i, e)
			}
		}
		curves := make([]path.Cubic, len(r.points)-1)
		for j := range curves {
			if j&255 == 0 {
				if e = ctx.Err(); e != nil {
					return nil, e
				}
			}
			curves[j] = path.Line(r.points[j], r.points[j+1])
		}
		object := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: seed, Curves: curves}}}).SetStroke(color, o.StrokeWidth)
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		if e = object.Validate(); e != nil {
			return nil, e
		}
		s.lines = append(s.lines, AdaptiveLine{r, object})
		objects = append(objects, object)
	}
	s.Object = gomath.Group(objects...)
	s.stats = b.stats
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return s, nil
}
