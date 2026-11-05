// Package field samples planar vector fields and integrates deterministic,
// fixed-step RK4 streamlines. Functions supplied by callers must be deterministic
// for deterministic output. Callbacks run synchronously during construction,
// never during rendering; panics from caller functions are not recovered.
package field

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

// Function maps an XY point to an XY velocity. Nonzero Z is unsupported.
type Function func(geom.Vec3) geom.Vec3

// ColorFunc maps a sample and its unscaled speed to a straight sRGB color.
type ColorFunc func(position, vector geom.Vec3, speed float64) gomath.Color

// Range samples Min+i*Step while <= Max, in ascending order. A singleton range
// (Min == Max) is supported. Steps must advance floating-point coordinates.
type Range struct{ Min, Max, Step float64 }

const maxSamples = 100_000

func (r Range) count() (int, error) {
	if !finite(r.Min) || !finite(r.Max) || !finite(r.Step) || r.Max < r.Min || r.Step <= 0 || !finite(r.Max-r.Min) {
		return 0, fmt.Errorf("range requires finite Min <= Max and Step > 0")
	}
	if r.Min < r.Max && (r.Min+r.Step <= r.Min || r.Max-r.Step >= r.Max) {
		return 0, fmt.Errorf("range step does not advance coordinates")
	}
	n := math.Floor((r.Max-r.Min)/r.Step) + 1
	if !finite(n) || n > maxSamples {
		return 0, fmt.Errorf("range exceeds %d samples", maxSamples)
	}
	return int(n), nil
}

type VectorOptions struct {
	Scale, MaxLength, TipSize, StrokeWidth float64
	Color                                  gomath.Color
	// Length, if non-nil, replaces speed*Scale before the MaxLength cap. It
	// must return a finite nonnegative length, including for zero speed.
	Length    func(speed float64) float64
	ColorFunc ColorFunc
}

func DefaultVectorOptions() VectorOptions {
	return VectorOptions{Scale: 1, MaxLength: .8, TipSize: .15, StrokeWidth: .035, Color: gomath.Blue}
}

// Sample records the original mathematical sample, independently of subsequent
// object transforms. Arrow is a borrowed node of VectorField.Object, or nil for
// zero velocity or zero requested length. Zero vectors never acquire a direction.
type Sample struct {
	Position, Vector geom.Vec3
	Speed            float64
	Arrow            *gomath.Object
}

// VectorField owns one tree; add Object to a scene. Sample arrows must not also
// be added as scene roots. Object.Copy() creates an independent renderable tree.
type VectorField struct {
	Object  *gomath.Object
	samples []Sample
}

// Samples returns a copy of the sample records, with borrowed arrow references.
func (v *VectorField) Samples() []Sample { return append([]Sample(nil), v.samples...) }

// NewVectorField samples in row-major order (Y outer, X inner). Any non-finite
// vector, nonzero Z, invalid callback result or geometry overflow is an error;
// construction never silently drops such samples. At most 100000 samples are
// accepted. Start from DefaultVectorOptions to configure individual options.
func NewVectorField(fn Function, x, y Range, o VectorOptions) (*VectorField, error) {
	if fn == nil {
		return nil, fmt.Errorf("nil vector field function")
	}
	if !positive(o.Scale) || !positive(o.MaxLength) || !positive(o.TipSize) || !positive(o.StrokeWidth) {
		return nil, fmt.Errorf("scale, maximum length, tip size and stroke width must be finite and positive")
	}
	if err := colorValid(o.Color); err != nil {
		return nil, err
	}
	nx, err := x.count()
	if err != nil {
		return nil, fmt.Errorf("X: %w", err)
	}
	ny, err := y.count()
	if err != nil {
		return nil, fmt.Errorf("Y: %w", err)
	}
	if nx > maxSamples/ny {
		return nil, fmt.Errorf("vector field exceeds %d samples", maxSamples)
	}
	v := &VectorField{samples: make([]Sample, 0, nx*ny)}
	objects := make([]*gomath.Object, 0, nx*ny)
	for j := range ny {
		for i := range nx {
			p := geom.V(x.Min+float64(i)*x.Step, y.Min+float64(j)*y.Step)
			// Rounding may place the final index just past Max.
			if p.X > x.Max || p.Y > y.Max {
				continue
			}
			vector := fn(p)
			speed := math.Hypot(vector.X, vector.Y)
			if !planar(vector) || !finite(speed) {
				return nil, fmt.Errorf("invalid planar vector at (%g,%g)", p.X, p.Y)
			}
			length := o.MaxLength
			if speed <= o.MaxLength/o.Scale {
				length = speed * o.Scale
			}
			if o.Length != nil {
				length = o.Length(speed)
			}
			if !finite(length) || length < 0 {
				return nil, fmt.Errorf("invalid arrow length at (%g,%g)", p.X, p.Y)
			}
			length = min(length, o.MaxLength)
			color := o.Color
			if o.ColorFunc != nil {
				color = o.ColorFunc(p, vector, speed)
			}
			if err := colorValid(color); err != nil {
				return nil, fmt.Errorf("color at (%g,%g): %w", p.X, p.Y, err)
			}
			sample := Sample{Position: p, Vector: vector, Speed: speed}
			if speed > 0 && length > 0 {
				// Normalize after scaling components, so subnormal magnitudes
				// do not distort direction or the requested arrow length.
				component := max(math.Abs(vector.X), math.Abs(vector.Y))
				direction := geom.V(vector.X/component, vector.Y/component)
				unitLength := math.Hypot(direction.X, direction.Y)
				direction = geom.V(direction.X/unitLength, direction.Y/unitLength)
				end := p.Add(direction.Mul(length))
				if !end.Finite() || end == p {
					return nil, fmt.Errorf("arrow endpoint overflow or loss of precision at (%g,%g)", p.X, p.Y)
				}
				tip := min(o.TipSize, length*.4)
				base := end.Sub(direction.Mul(tip))
				side := geom.V(-direction.Y, direction.X).Mul(tip * .4)
				left, right := base.Add(side), base.Sub(side)
				if base == end || base == p || left == right || left == end || right == end {
					return nil, fmt.Errorf("arrowhead loses precision at (%g,%g)", p.X, p.Y)
				}
				shaft := gomath.Line(p, base).SetStroke(color, o.StrokeWidth)
				head := gomath.Polygon(end, left, right).SetFill(color).SetStroke(gomath.Transparent, 0)
				sample.Arrow = gomath.Group(shaft, head)
				if err := sample.Arrow.Validate(); err != nil {
					return nil, err
				}
				objects = append(objects, sample.Arrow)
			}
			v.samples = append(v.samples, sample)
		}
	}
	v.Object = gomath.Group(objects...)
	return v, nil
}

// SpeedColor returns a clamped linear speed gradient, after validating its
// finite strictly increasing range and endpoint colors.
func SpeedColor(low, high gomath.Color, minSpeed, maxSpeed float64) (ColorFunc, error) {
	if err := colorValid(low); err != nil {
		return nil, err
	}
	if err := colorValid(high); err != nil {
		return nil, err
	}
	if !finite(minSpeed) || !finite(maxSpeed) || minSpeed < 0 || maxSpeed <= minSpeed || !finite(maxSpeed-minSpeed) {
		return nil, fmt.Errorf("speed color requires 0 <= minimum < maximum, both finite")
	}
	return func(_, _ geom.Vec3, speed float64) gomath.Color {
		t := max(0, min(1, (speed-minSpeed)/(maxSpeed-minSpeed)))
		if t <= 0 {
			return low
		}
		if t >= 1 {
			return high
		}
		return low.Lerp(high, t)
	}, nil
}

func finite(v float64) bool   { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func positive(v float64) bool { return finite(v) && v > 0 }
func planar(v geom.Vec3) bool { return v.Finite() && v.Z == 0 }
func colorValid(c gomath.Color) error {
	s := gomath.DefaultStyle()
	s.Stroke = c
	return s.Validate()
}
