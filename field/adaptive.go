package field

import (
	"context"
	"errors"
	"fmt"
	"gomath/geom"
	"math"
)

var (
	ErrAdaptiveOptions = errors.New("field: invalid adaptive options or seed")
	ErrAdaptiveLimit   = errors.New("field: adaptive resource budget exceeded")
)

const (
	TimeComplete Termination = "time_complete"
	MinimumStep  Termination = "minimum_step"
)

// AdaptiveLimits are literal per-call budgets, shared across all seeds in a
// constructor. Zero is a zero budget, not a default. Hard caps: 1M accepted,
// 1M rejected, 8M field evaluations, 512MiB modeled storage. No budget is reset
// between seeds. Exhaustion returns nil/error, never an unreported partial set.
type AdaptiveLimits struct {
	MaxAcceptedSteps, MaxRejectedSteps, MaxEvaluations int
	MaxStorageBytes                                    uint64
}

// AdaptiveOptions integrates autonomous planar x'=fn(x). Step sizes are positive
// magnitudes; EndTime-StartTime determines direction. Tolerances are finite and
// nonnegative with at least one positive, applied to RMS normalized embedded
// component errors. MinSpeed is a finite nonnegative stage stopping threshold.
// Start with DefaultAdaptiveOptions; no zero field is implicitly defaulted.
type AdaptiveOptions struct {
	Domain                               Domain
	StartTime, EndTime                   float64
	InitialStep, MinStep, MaxStep        float64
	AbsoluteTolerance, RelativeTolerance float64
	MinSpeed                             float64
	Limits                               AdaptiveLimits
}

func DefaultAdaptiveOptions() AdaptiveOptions {
	return AdaptiveOptions{Domain: Domain{geom.V(-4, -3), geom.V(4, 3)}, EndTime: 10, InitialStep: .05, MinStep: 1e-10, MaxStep: .5, AbsoluteTolerance: 1e-9, RelativeTolerance: 1e-7, MinSpeed: 1e-10, Limits: AdaptiveLimits{10000, 10000, 100000, 64 << 20}}
}
func (o AdaptiveOptions) validate() error {
	if e := o.Domain.validate(); e != nil {
		return fmt.Errorf("%w: %v", ErrAdaptiveOptions, e)
	}
	l := o.Limits
	if !finite(o.StartTime) || !finite(o.EndTime) || !finite(o.EndTime-o.StartTime) || !positive(o.MinStep) || !positive(o.InitialStep) || !positive(o.MaxStep) || o.MinStep > o.InitialStep || o.InitialStep > o.MaxStep || !finite(o.AbsoluteTolerance) || !finite(o.RelativeTolerance) || o.AbsoluteTolerance < 0 || o.RelativeTolerance < 0 || (o.AbsoluteTolerance == 0 && o.RelativeTolerance == 0) || !finite(o.MinSpeed) || o.MinSpeed < 0 || l.MaxAcceptedSteps < 0 || l.MaxAcceptedSteps > 1000000 || l.MaxRejectedSteps < 0 || l.MaxRejectedSteps > 1000000 || l.MaxEvaluations < 0 || l.MaxEvaluations > 8000000 || l.MaxStorageBytes > 512<<20 {
		return ErrAdaptiveOptions
	}
	return nil
}

// AdaptiveStats counts actual accepted steps, completed error-rejected trials
// and field calls (including invalid values). An interrupted/outside stage is
// not an error-rejected trial. FSAL reuses the accepted endpoint derivative.
type AdaptiveStats struct{ AcceptedSteps, RejectedSteps, Evaluations int }

// AdaptiveResult is immutable and contains only accepted points/times, starting
// with the seed/StartTime. Time is the last accepted absolute time, not a step
// count times a nominal step. Stop reports the terminating stage (possibly an
// invalid/outside point); it is not inserted into the accepted slices.
type AdaptiveResult struct {
	times      []float64
	points     []geom.Vec3
	stats      AdaptiveStats
	reason     Termination
	stopTime   float64
	stopPoint  geom.Vec3
	seedVector geom.Vec3
	seedSpeed  float64
	seedValid  bool
}

func (r *AdaptiveResult) Times() []float64 {
	if r == nil {
		return nil
	}
	return append([]float64(nil), r.times...)
}
func (r *AdaptiveResult) Points() []geom.Vec3 {
	if r == nil {
		return nil
	}
	return append([]geom.Vec3(nil), r.points...)
}
func (r *AdaptiveResult) Stats() AdaptiveStats {
	if r == nil {
		return AdaptiveStats{}
	}
	return r.stats
}
func (r *AdaptiveResult) Reason() Termination {
	if r == nil {
		return ""
	}
	return r.reason
}
func (r *AdaptiveResult) Time() float64 {
	if r == nil || len(r.times) == 0 {
		return 0
	}
	return r.times[len(r.times)-1]
}
func (r *AdaptiveResult) Stop() (float64, geom.Vec3) {
	if r == nil {
		return 0, geom.Vec3{}
	}
	return r.stopTime, r.stopPoint
}

type adaptiveBudget struct {
	ctx    context.Context
	limits AdaptiveLimits
	stats  AdaptiveStats
	times  []float64
	points []geom.Vec3
}

func adaptivePrepare(ctx context.Context, fn Function, seeds []geom.Vec3, o AdaptiveOptions, geometry bool) (*adaptiveBudget, error) {
	if ctx == nil {
		return nil, ErrAdaptiveOptions
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if fn == nil {
		return nil, ErrAdaptiveOptions
	}
	if e := o.validate(); e != nil {
		return nil, e
	}
	if len(seeds) > maxSamples {
		return nil, ErrAdaptiveLimit
	}
	for i, p := range seeds {
		if i&255 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
		}
		if !o.Domain.Contains(p) {
			return nil, fmt.Errorf("%w: seed %d", ErrAdaptiveOptions, i)
		}
	}
	// Validated integer caps make additions/products safe on both 32 and 64 bits.
	count := uint64(o.Limits.MaxAcceptedSteps) + uint64(len(seeds))
	perPoint := uint64(32)
	if geometry {
		perPoint = 1024
	} // includes path copies, objects, records and scratch model
	storage := perPoint*count + adaptiveScratchBytes
	if storage > o.Limits.MaxStorageBytes || storage > uint64(^uint(0)>>1) {
		return nil, ErrAdaptiveLimit
	}
	b := &adaptiveBudget{ctx: ctx, limits: o.Limits, times: make([]float64, 0, int(count)), points: make([]geom.Vec3, 0, int(count))}
	return b, nil
}
func (b *adaptiveBudget) evaluate(fn Function, p geom.Vec3) (geom.Vec3, float64, bool, error) {
	if e := b.ctx.Err(); e != nil {
		return geom.Vec3{}, 0, false, e
	}
	if b.stats.Evaluations >= b.limits.MaxEvaluations {
		return geom.Vec3{}, 0, false, fmt.Errorf("%w: evaluations", ErrAdaptiveLimit)
	}
	b.stats.Evaluations++
	v := fn(p)
	if e := b.ctx.Err(); e != nil {
		return geom.Vec3{}, 0, false, e
	}
	speed := math.Hypot(v.X, v.Y)
	return v, speed, planar(v) && finite(speed), nil
}

// IntegrateAdaptive computes bounded Dormand-Prince RK45 without objects or
// style/color callbacks. No field call is made outside Domain. The first outside
// trial stage terminates at the accepted prefix; there is no boundary clipping,
// event localization, dense output or RK4 fallback. Invalid vectors/points,
// stage stagnation, no coordinate/time progress and an unsatisfied minimum step
// also return explicit prefix reasons. Configuration/resource/cancellation
// failures return nil/error. Caller panics are not recovered. Callbacks run
// synchronously; shared callbacks must themselves be safe for concurrent calls.
func IntegrateAdaptive(ctx context.Context, fn Function, seed geom.Vec3, o AdaptiveOptions) (*AdaptiveResult, error) {
	b, e := adaptivePrepare(ctx, fn, []geom.Vec3{seed}, o, false)
	if e != nil {
		return nil, e
	}
	return integrateAdaptive(fn, seed, o, b)
}
func integrateAdaptive(fn Function, seed geom.Vec3, o AdaptiveOptions, b *adaptiveBudget) (*AdaptiveResult, error) {
	start := len(b.points)
	b.points = append(b.points, seed)
	b.times = append(b.times, o.StartTime)
	before := b.stats
	r := &AdaptiveResult{reason: TimeComplete, stopTime: o.StartTime, stopPoint: seed}
	finish := func(reason Termination) (*AdaptiveResult, error) {
		if e := b.ctx.Err(); e != nil {
			return nil, e
		}
		r.reason = reason
		end := len(b.points)
		r.points = b.points[start:end:end]
		r.times = b.times[start:end:end]
		r.stats = AdaptiveStats{b.stats.AcceptedSteps - before.AcceptedSteps, b.stats.RejectedSteps - before.RejectedSteps, b.stats.Evaluations - before.Evaluations}
		return r, nil
	}
	if o.StartTime == o.EndTime {
		return finish(TimeComplete)
	}
	if b.stats.AcceptedSteps >= b.limits.MaxAcceptedSteps {
		return nil, fmt.Errorf("%w: accepted steps", ErrAdaptiveLimit)
	}
	var k [7]geom.Vec3
	v, speed, valid, e := b.evaluate(fn, seed)
	if e != nil {
		return nil, e
	}
	r.seedVector, r.seedSpeed, r.seedValid = v, speed, valid
	if !valid {
		return finish(InvalidVector)
	}
	if speed <= o.MinSpeed {
		return finish(Stagnation)
	}
	k[0] = v
	arithmetic := newAdaptiveArithmetic()
	p, t := seed, o.StartTime
	direction := math.Copysign(1, o.EndTime-o.StartTime)
	step := o.InitialStep
	rejectedLast := false
	for {
		if e = b.ctx.Err(); e != nil {
			return nil, e
		}
		if b.stats.AcceptedSteps >= b.limits.MaxAcceptedSteps {
			return nil, fmt.Errorf("%w: accepted steps", ErrAdaptiveLimit)
		}
		remaining := math.Abs(o.EndTime - t)
		step = min(step, o.MaxStep, remaining)
		nextTime := t + direction*step
		if step == remaining {
			nextTime = o.EndTime
		}
		h := nextTime - t
		r.stopTime, r.stopPoint = t, p
		if nextTime == t || math.Abs(h) > o.MaxStep {
			// The time grid cannot represent the proposed step inside the allowed cap.
			return finish(NoProgress)
		}
		if math.Abs(h) < o.MinStep && nextTime != o.EndTime {
			return finish(MinimumStep)
		}
		for j, row := range adaptiveRows {
			if e = b.ctx.Err(); e != nil {
				return nil, e
			}
			stage, ok := arithmetic.point(p, h, &k, row)
			stageTime := t + adaptiveAbscissa[j]*h
			if j >= 4 {
				stageTime = nextTime
			}
			r.stopTime, r.stopPoint = stageTime, stage
			if !ok || !planar(stage) {
				return finish(InvalidPoint)
			}
			if !o.Domain.Contains(stage) {
				return finish(DomainExit)
			}
			v, speed, valid, e = b.evaluate(fn, stage)
			if e != nil {
				return nil, e
			}
			if !valid {
				return finish(InvalidVector)
			}
			if speed <= o.MinSpeed {
				return finish(Stagnation)
			}
			k[j+1] = v
		}
		// The final stage is the fifth-order endpoint, whose derivative is FSAL.
		next := r.stopPoint
		errorNorm := arithmetic.errorNorm(p, next, h, &k, o.AbsoluteTolerance, o.RelativeTolerance)
		if errorNorm > 1 {
			if b.stats.RejectedSteps >= b.limits.MaxRejectedSteps {
				return nil, fmt.Errorf("%w: rejected steps", ErrAdaptiveLimit)
			}
			b.stats.RejectedSteps++
			if math.Abs(h) <= o.MinStep {
				return finish(MinimumStep)
			}
			reduced := max(o.MinStep, math.Abs(h)*max(.2, .9*math.Pow(errorNorm, -.2)))
			if reduced >= math.Abs(h) {
				return finish(MinimumStep)
			}
			step = reduced
			rejectedLast = true
			continue
		}
		if next == p {
			return finish(NoProgress)
		}
		b.stats.AcceptedSteps++
		b.points = append(b.points, next)
		b.times = append(b.times, nextTime)
		p, t = next, nextTime
		k[0] = k[6]
		if t == o.EndTime {
			return finish(TimeComplete)
		}
		factor := 10.0
		if errorNorm > 0 {
			factor = min(10, .9*math.Pow(errorNorm, -.2))
		}
		if rejectedLast {
			factor = min(1, factor)
		}
		// Cap before multiplying, including the case of a huge finite step.
		if factor >= o.MaxStep/math.Abs(h) {
			step = o.MaxStep
		} else {
			step = max(o.MinStep, math.Abs(h)*factor)
		}
		rejectedLast = false
	}
}
