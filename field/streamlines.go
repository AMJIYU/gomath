package field

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

// Domain is a closed rectangular XY domain. Min.Z and Max.Z must be zero.
// All RK4 stages must lie inside it; no evaluation outside it is attempted.
type Domain struct{ Min, Max geom.Vec3 }

func (d Domain) validate() error {
	if !planar(d.Min) || !planar(d.Max) || d.Min.X >= d.Max.X || d.Min.Y >= d.Max.Y || !d.Max.Sub(d.Min).Finite() {
		return fmt.Errorf("domain requires finite planar Min < Max on both axes")
	}
	return nil
}

func (d Domain) Contains(p geom.Vec3) bool {
	return planar(p) && p.X >= d.Min.X && p.X <= d.Max.X && p.Y >= d.Min.Y && p.Y <= d.Max.Y
}

type StreamOptions struct {
	Domain      Domain
	StepSize    float64 // Signed time step; negative integrates backwards.
	MaxSteps    int     // 1..100000, with at most 1000000 steps across all seeds.
	MinSpeed    float64 // A speed <= MinSpeed terminates at any RK4 stage.
	StrokeWidth float64
	Color       gomath.Color
	// ColorFunc, if provided, colors each entire line using its seed sample.
	// An invalid seed vector retains Color and an InvalidVector termination.
	ColorFunc ColorFunc
}

func DefaultStreamOptions() StreamOptions {
	return StreamOptions{Domain: Domain{geom.V(-4, -3), geom.V(4, 3)}, StepSize: .05, MaxSteps: 200, MinSpeed: 1e-10, StrokeWidth: .025, Color: gomath.Blue}
}

func (o StreamOptions) validate() error {
	if err := o.Domain.validate(); err != nil {
		return err
	}
	if !finite(o.StepSize) || o.StepSize == 0 || o.MaxSteps < 1 || o.MaxSteps > 100_000 || !finite(o.StepSize*float64(o.MaxSteps)) || !finite(o.MinSpeed) || o.MinSpeed < 0 || !positive(o.StrokeWidth) {
		return fmt.Errorf("invalid RK4 time step, step budget, minimum speed or stroke width")
	}
	return colorValid(o.Color)
}

type Termination string

const (
	StepLimit     Termination = "step_limit"
	DomainExit    Termination = "domain_exit"
	Stagnation    Termination = "stagnation"
	InvalidVector Termination = "invalid_vector"
	InvalidPoint  Termination = "invalid_point"
	NoProgress    Termination = "no_progress"
)

// Streamline includes only accepted RK4 points, beginning with the seed. Steps
// counts accepted steps and Time == Steps*StepSize. StopPoint identifies the
// failed stage (or last point for StepLimit). Reason reports why integration
// stopped, including singularities, unsupported Z and numerical overflow.
// Object is a borrowed child of StreamLines.Object. A line with no accepted
// steps has only its seed and intentionally has no visible segment.
type Streamline struct {
	Points    []geom.Vec3
	Steps     int
	Time      float64
	Reason    Termination
	StopPoint geom.Vec3
	Object    *gomath.Object
}

type StreamLines struct {
	Object *gomath.Object
	lines  []Streamline
}

// Lines copies both records and numerical point slices. Object references are
// borrowed; use Object.Copy() to add an independent tree to a scene.
func (s *StreamLines) Lines() []Streamline {
	out := append([]Streamline(nil), s.lines...)
	for i := range out {
		out[i].Points = append([]geom.Vec3(nil), out[i].Points...)
	}
	return out
}

// NewStreamLines integrates one independent line per seed in input order using
// x'=fn(x), without normalizing velocities. Configuration and invalid/outside
// seeds return errors. Invalid field values stop that line with InvalidVector
// and remain visible in Lines(), rather than being silently discarded. Domain
// exits stop at the last complete step, without boundary clipping. At most
// 100000 seeds and 1000000 total requested steps are supported.
func NewStreamLines(fn Function, seeds []geom.Vec3, o StreamOptions) (*StreamLines, error) {
	if fn == nil {
		return nil, fmt.Errorf("nil streamline function")
	}
	if err := o.validate(); err != nil {
		return nil, err
	}
	if len(seeds) > maxSamples || len(seeds) > 1_000_000/o.MaxSteps {
		return nil, fmt.Errorf("streamline seed/step budget exceeded")
	}
	for i, seed := range seeds {
		if !o.Domain.Contains(seed) {
			return nil, fmt.Errorf("seed %d is non-planar, non-finite or outside domain", i)
		}
	}
	s := &StreamLines{lines: make([]Streamline, 0, len(seeds))}
	objects := make([]*gomath.Object, 0, len(seeds))
	for i, seed := range seeds {
		line, vector, speed := integrate(fn, seed, o)
		color := o.Color
		// There is no valid seed sample to color for InvalidVector at step 0.
		if o.ColorFunc != nil && planar(vector) && finite(speed) {
			color = o.ColorFunc(seed, vector, speed)
			if err := colorValid(color); err != nil {
				return nil, fmt.Errorf("seed %d color: %w", i, err)
			}
		}
		line.Object = gomath.Polyline(line.Points...).SetStroke(color, o.StrokeWidth)
		if err := line.Object.Validate(); err != nil {
			return nil, err
		}
		s.lines = append(s.lines, line)
		objects = append(objects, line.Object)
	}
	s.Object = gomath.Group(objects...)
	return s, nil
}

// Integrate computes one numerical streamline without allocating any objects.
// Color/style options are still validated consistently with NewStreamLines;
// ColorFunc is not evaluated because this function produces no rendered line.
func Integrate(fn Function, seed geom.Vec3, o StreamOptions) (Streamline, error) {
	if fn == nil {
		return Streamline{}, fmt.Errorf("nil streamline function")
	}
	if err := o.validate(); err != nil {
		return Streamline{}, err
	}
	if !o.Domain.Contains(seed) {
		return Streamline{}, fmt.Errorf("seed is non-planar, non-finite or outside domain")
	}
	line, _, _ := integrate(fn, seed, o)
	return line, nil
}

func integrate(fn Function, seed geom.Vec3, o StreamOptions) (Streamline, geom.Vec3, float64) {
	line := Streamline{Points: make([]geom.Vec3, 1, min(o.MaxSteps+1, 1024)), Reason: StepLimit, StopPoint: seed}
	line.Points[0] = seed
	var seedVector geom.Vec3
	var seedSpeed float64
	eval := func(p geom.Vec3) (geom.Vec3, Termination) {
		line.StopPoint = p
		if !planar(p) {
			return geom.Vec3{}, InvalidPoint
		}
		if !o.Domain.Contains(p) {
			return geom.Vec3{}, DomainExit
		}
		v := fn(p)
		speed := math.Hypot(v.X, v.Y)
		if len(line.Points) == 1 && p == seed {
			seedVector, seedSpeed = v, speed
		}
		if !planar(v) || !finite(speed) {
			return geom.Vec3{}, InvalidVector
		}
		if speed <= o.MinSpeed {
			return geom.Vec3{}, Stagnation
		}
		return v, ""
	}
	p := seed
	for range o.MaxSteps {
		k1, reason := eval(p)
		if reason != "" {
			line.Reason = reason
			break
		}
		k2, reason := eval(p.Add(k1.Mul(o.StepSize / 2)))
		if reason != "" {
			line.Reason = reason
			break
		}
		k3, reason := eval(p.Add(k2.Mul(o.StepSize / 2)))
		if reason != "" {
			line.Reason = reason
			break
		}
		k4, reason := eval(p.Add(k3.Mul(o.StepSize)))
		if reason != "" {
			line.Reason = reason
			break
		}
		// Weight before adding to avoid an unnecessary 6*velocity overflow.
		velocity := k1.Mul(1.0 / 6).Add(k2.Mul(1.0 / 3)).Add(k3.Mul(1.0 / 3)).Add(k4.Mul(1.0 / 6))
		next := p.Add(velocity.Mul(o.StepSize))
		line.StopPoint = next
		if !planar(next) {
			line.Reason = InvalidPoint
			break
		}
		if !o.Domain.Contains(next) {
			line.Reason = DomainExit
			break
		}
		if next == p {
			line.Reason = NoProgress
			break
		}
		p = next
		line.Points = append(line.Points, p)
		line.Steps++
	}
	line.Time = float64(line.Steps) * o.StepSize
	return line, seedVector, seedSpeed
}
