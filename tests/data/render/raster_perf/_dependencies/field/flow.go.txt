package field

import (
	"errors"
	"fmt"
	"gomath"
	"gomath/path"
	"math"
	"math/bits"
	"sort"
	"strings"
)

var (
	ErrFlowOptions     = errors.New("field: invalid flow input")
	ErrFlowUnsupported = errors.New("field: unsupported flow content")
	ErrFlowLimit       = errors.New("field: flow budget exceeded")
	ErrFlowRange       = errors.New("field: flow numerical range exceeded")
)

// FlowLine maps each cubic to a physical time interval. Times has one more
// element than the single open contour's curves, and is strictly monotone in
// either direction. Times and geometry are copied; Object is only borrowed
// during construction. One time and no curves describes an invisible seed.
type FlowLine struct {
	Times  []float64
	Object *gomath.Object
}

// FlowLimits bounds aggregate construction and individual Sample calls. Bytes
// are a conservative modeled storage reserve, not an RSS/allocation cap.
// Work counts line visits, binary-search comparison reserves and output curves.
type FlowLimits struct {
	MaxLines, MaxSegments            int
	MaxStorageBytes                  uint64
	MaxSampleSegments, MaxSampleWork int
	MaxSampleBytes                   uint64
}

func DefaultFlowLimits() FlowLimits {
	return FlowLimits{100000, 1000000, 512 << 20, 1000000, 10000000, 512 << 20}
}

type FlowOptions struct {
	Duration       float64    // Display seconds; zero is allowed.
	FlowSpeed      float64    // Signed physical seconds per display second, nonzero.
	TraceTimeWidth float64    // Positive physical seconds, not a curve fraction.
	StartDelays    []float64  // Empty means zero; otherwise one nonnegative display delay per line.
	Loop           bool       // Restart after (line span + TraceTimeWidth) physical seconds.
	Limits         FlowLimits // Explicit; start with DefaultFlowOptions.
}

func DefaultFlowOptions() FlowOptions {
	return FlowOptions{Duration: 5, FlowSpeed: 1, TraceTimeWidth: .3, Limits: DefaultFlowLimits()}
}

type flowLine struct {
	elapsed             []float64
	object              gomath.Object
	delay, span, period float64
}

// Flow is an immutable render.FrameSource. Each Sample has independent mutable
// objects/paths with identities stable within this Flow, distinct from sources.
// Authoring inputs must not be mutated concurrently with construction.
type Flow struct {
	options    FlowOptions
	lines      []flowLine
	root       gomath.Object
	baseBytes  uint64
	searchWork int
}

func (f *Flow) Duration() float64 {
	if f == nil {
		return 0
	}
	return f.options.Duration
}

// NewFlow snapshots explicitly timed cubic paths. It never calls a field,
// color function or integrator. Only open, continuous, solid unfilled strokes
// with Reveal=1 are supported; bitmap, clip, children, dash and fill are errors.
func NewFlow(lines []FlowLine, o FlowOptions) (*Flow, error) { return newFlow(lines, o, nil) }

func flowPrepare(o FlowOptions, n int) error {
	l := o.Limits
	if !finite(o.Duration) || o.Duration < 0 || !finite(o.FlowSpeed) || o.FlowSpeed == 0 || !positive(o.TraceTimeWidth) ||
		l.MaxLines < 0 || l.MaxLines > 100000 || l.MaxSegments < 0 || l.MaxSegments > 1000000 || l.MaxStorageBytes < 4096 || l.MaxStorageBytes > 1<<30 ||
		l.MaxSampleSegments < 0 || l.MaxSampleSegments > 1000000 || l.MaxSampleWork < 0 || l.MaxSampleWork > 100000000 || l.MaxSampleBytes < 4096 || l.MaxSampleBytes > 1<<30 ||
		(len(o.StartDelays) != 0 && len(o.StartDelays) != n) {
		return ErrFlowOptions
	}
	if n > l.MaxLines {
		return ErrFlowLimit
	}
	if p := o.Duration * math.Abs(o.FlowSpeed); !finite(p) || (o.Duration > 0 && p == 0) {
		return ErrFlowRange
	}
	for _, d := range o.StartDelays {
		if !finite(d) || d < 0 {
			return ErrFlowOptions
		}
	}
	return nil
}

func flowStyle(o *gomath.Object) error {
	if o == nil {
		return ErrFlowOptions
	}
	if o.Bitmap != nil || o.Clip != nil || len(o.Children) != 0 || o.Style.Fill.A != 0 || o.Style.FillGradient != nil || o.Style.Dash != nil || o.Style.Reveal != 1 {
		return ErrFlowUnsupported
	}
	if err := o.Style.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrFlowOptions, err)
	}
	if !finite(o.Scalar) {
		return ErrFlowOptions
	}
	return nil
}

func newFlow(lines []FlowLine, o FlowOptions, group *gomath.Object) (*Flow, error) {
	if err := flowPrepare(o, len(lines)); err != nil {
		return nil, err
	}
	// Preflight sizes before any input-dependent copy or geometry traversal.
	total := 0
	bytes := uint64(4096) + uint64(len(lines))*1024
	work := 2 * len(lines)
	if group != nil {
		bytes += uint64(len(group.Name))
	}
	if bytes > o.Limits.MaxStorageBytes {
		return nil, ErrFlowLimit
	}
	for i, line := range lines {
		if line.Object == nil {
			return nil, fmt.Errorf("%w: line %d nil object", ErrFlowOptions, i)
		}
		p := line.Object.Path
		if len(p.Contours) != 1 || p.Contours[0].Closed {
			return nil, ErrFlowUnsupported
		}
		n := len(p.Contours[0].Curves)
		if n > o.Limits.MaxSegments-total {
			return nil, ErrFlowLimit
		}
		total += n
		if len(line.Times) != n+1 {
			return nil, fmt.Errorf("%w: line %d time/curve count", ErrFlowOptions, i)
		}
		bytes += uint64(n)*256 + uint64(len(line.Object.Name))
		if bytes > o.Limits.MaxStorageBytes {
			return nil, ErrFlowLimit
		}
		work += 4 * bits.Len(uint(n+1))
	}
	if bytes > o.Limits.MaxStorageBytes {
		return nil, ErrFlowLimit
	}
	if group != nil {
		q := *group
		q.Children = nil
		if err := flowStyle(&q); err != nil {
			return nil, err
		}
		if len(q.Path.Contours) != 0 {
			return nil, ErrFlowUnsupported
		}
	}
	// Names are retained in every frame; fixed records/reserves are conservative.
	base := uint64(4096) + uint64(len(lines))*1024
	if group != nil {
		base += uint64(len(group.Name))
	}
	f := &Flow{options: o, lines: make([]flowLine, len(lines)), baseBytes: base, searchWork: work}
	f.options.StartDelays = nil // Per-line values below are owned; retain no caller slice.
	for i, line := range lines {
		src := line.Object
		if err := flowStyle(src); err != nil {
			return nil, fmt.Errorf("line %d: %w", i, err)
		}
		if err := src.Path.Validate(); err != nil {
			return nil, fmt.Errorf("%w: line %d path: %v", ErrFlowOptions, i, err)
		}
		c := src.Path.Contours[0]
		prev := c.Start
		for _, v := range c.Curves {
			// Require exact continuity rather than silently join a near-disconnected curve.
			if v.P0 != prev {
				return nil, ErrFlowUnsupported
			}
			prev = v.P3
		}
		times := line.Times
		if !finite(times[0]) {
			return nil, ErrFlowOptions
		}
		direction := 1.0
		if len(times) > 1 && times[1] < times[0] {
			direction = -1
		}
		elapsed := make([]float64, len(times))
		for j := 1; j < len(times); j++ {
			if !finite(times[j]) || direction*(times[j]-times[j-1]) <= 0 {
				return nil, ErrFlowOptions
			}
			elapsed[j] = direction * (times[j] - times[0])
			if !finite(elapsed[j]) {
				return nil, ErrFlowRange
			}
			if elapsed[j] <= elapsed[j-1] {
				return nil, ErrFlowRange
			}
		}
		span := elapsed[len(elapsed)-1]
		period := span + o.TraceTimeWidth
		if !finite(period) || period <= span {
			return nil, ErrFlowRange
		}
		delay := 0.0
		if len(o.StartDelays) != 0 {
			delay = o.StartDelays[i]
		}
		// Beyond 2^48 cycles binary64 no longer provides useful subcycle resolution.
		if span > 0 && o.Loop && math.Max(0, o.Duration-delay)*math.Abs(o.FlowSpeed)/period > 1<<48 {
			return nil, ErrFlowRange
		}
		owned := src.Copy()
		owned.Name = strings.Clone(src.Name)
		f.lines[i] = flowLine{elapsed: elapsed, object: *owned, delay: delay, span: span, period: period}
		f.baseBytes += uint64(len(src.Name))
	}
	f.root = *gomath.Group()
	if group != nil {
		f.root.Name = strings.Clone(group.Name)
		f.root.Style = group.Style
		f.root.Scalar = group.Scalar
		f.root.ZIndex = group.ZIndex
	}
	return f, nil
}

// NewFlow captures current authored geometry/style, retaining physical times.
// The source root must still contain exactly its original line children.
func (s *StreamLines) NewFlow(o FlowOptions) (*Flow, error) {
	if s == nil || s.Object == nil {
		return nil, ErrFlowOptions
	}
	if err := flowPrepare(o, len(s.lines)); err != nil {
		return nil, err
	}
	if len(s.Object.Children) != len(s.lines) {
		return nil, ErrFlowUnsupported
	}
	n := 0
	for _, l := range s.lines {
		if l.Steps < 0 || l.Steps > o.Limits.MaxSegments-n {
			return nil, ErrFlowLimit
		}
		n += l.Steps
	}
	if uint64(n)*32+uint64(len(s.lines))*1024+4096 > o.Limits.MaxStorageBytes {
		return nil, ErrFlowLimit
	}
	inputs := make([]FlowLine, len(s.lines))
	for i, l := range s.lines {
		if l.Object != s.Object.Children[i] || l.Steps != len(l.Points)-1 || !finite(l.Time) || (l.Steps > 0 && l.Time == 0) {
			return nil, ErrFlowOptions
		}
		times := make([]float64, l.Steps+1)
		for j := 1; j < len(times); j++ {
			times[j] = float64(j) * (l.Time / float64(l.Steps))
		}
		times[len(times)-1] = l.Time
		inputs[i] = FlowLine{times, l.Object}
	}
	return newFlow(inputs, o, s.Object)
}

func (s *AdaptiveStreamLines) NewFlow(o FlowOptions) (*Flow, error) {
	if s == nil || s.Object == nil {
		return nil, ErrFlowOptions
	}
	if err := flowPrepare(o, len(s.lines)); err != nil {
		return nil, err
	}
	if len(s.Object.Children) != len(s.lines) {
		return nil, ErrFlowUnsupported
	}
	if uint64(len(s.lines))*1024+4096 > o.Limits.MaxStorageBytes {
		return nil, ErrFlowLimit
	}
	inputs := make([]FlowLine, len(s.lines))
	for i, l := range s.lines {
		if l.Result == nil || l.Object != s.Object.Children[i] {
			return nil, ErrFlowOptions
		}
		// newFlow copies these immutable internal times after preflight.
		inputs[i] = FlowLine{l.Result.times, l.Object}
	}
	return newFlow(inputs, o, s.Object)
}

// bounds returns an elapsed-time interval and its half-open curve-index span.
func (f *Flow) bounds(l *flowLine, t float64) (lo, hi float64, first, last int, err error) {
	if l.span == 0 || t <= l.delay {
		return
	}
	h := (t - l.delay) * math.Abs(f.options.FlowSpeed)
	if h == 0 {
		return 0, 0, 0, 0, ErrFlowRange
	}
	if f.options.Loop {
		h = math.Mod(h, l.period)
	}
	lo = math.Max(0, h-f.options.TraceTimeWidth)
	hi = math.Min(l.span, h)
	if hi <= lo {
		return 0, 0, 0, 0, nil
	}
	if f.options.FlowSpeed < 0 {
		lo, hi = l.span-hi, l.span-lo
		if hi <= lo {
			return 0, 0, 0, 0, ErrFlowRange
		}
	}
	first = sort.Search(len(l.elapsed)-1, func(i int) bool { return l.elapsed[i+1] > lo })
	last = sort.Search(len(l.elapsed), func(i int) bool { return l.elapsed[i] >= hi })
	return
}

// Sample accepts only finite display times in [0,Duration]. Before start and
// after a nonlooping pulse drains, it returns stable empty-path line objects.
// Loop endpoints reset to the empty seed state; tails never bridge end to start.
// Configuration, resource and numeric failures return nil with an error.
func (f *Flow) Sample(t float64) (*gomath.Frame, error) {
	if f == nil || f.root.ID == 0 || !finite(t) || t < 0 || t > f.options.Duration {
		return nil, ErrFlowOptions
	}
	if f.searchWork > f.options.Limits.MaxSampleWork {
		return nil, ErrFlowLimit
	}
	n := 0
	for i := range f.lines {
		_, _, a, b, err := f.bounds(&f.lines[i], t)
		if err != nil {
			return nil, err
		}
		if b-a > f.options.Limits.MaxSampleSegments-n {
			return nil, ErrFlowLimit
		}
		n += b - a
	}
	if f.searchWork+n > f.options.Limits.MaxSampleWork || f.baseBytes+uint64(n)*128 > f.options.Limits.MaxSampleBytes {
		return nil, ErrFlowLimit
	}
	root := f.root
	root.Children = make([]*gomath.Object, len(f.lines))
	for i := range f.lines {
		l := &f.lines[i]
		q := l.object
		q.Path = path.Path{}
		lo, hi, a, b, err := f.bounds(l, t)
		if err != nil {
			return nil, err
		}
		if b > a {
			curves := make([]path.Cubic, b-a)
			for j := a; j < b; j++ {
				v := l.object.Path.Contours[0].Curves[j]
				dt := l.elapsed[j+1] - l.elapsed[j]
				start := (math.Max(lo, l.elapsed[j]) - l.elapsed[j]) / dt
				end := (math.Min(hi, l.elapsed[j+1]) - l.elapsed[j]) / dt
				if end <= start {
					return nil, ErrFlowRange
				}
				if end < 1 {
					v, _ = v.Split(end)
				}
				if start > 0 {
					_, v = v.Split(start / end)
				}
				if !v.P0.Finite() || !v.P1.Finite() || !v.P2.Finite() || !v.P3.Finite() {
					return nil, ErrFlowRange
				}
				curves[j-a] = v
			}
			q.Path = path.Path{Contours: []path.Contour{{Start: curves[0].P0, Curves: curves}}}
		}
		root.Children[i] = &q
	}
	return &gomath.Frame{Time: t, Objects: []*gomath.Object{&root}}, nil
}
