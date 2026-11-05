package field_test

import (
	"context"
	"errors"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"math"
	"sync/atomic"
	"testing"
)

type adaptiveCountContext struct {
	context.Context
	cancel context.CancelFunc
	count  atomic.Int64
	after  int64
}

func (c *adaptiveCountContext) Err() error {
	if c.count.Add(1) >= c.after {
		c.cancel()
	}
	return c.Context.Err()
}
func TestAdaptiveCancellationEveryCheckpoint(t *testing.T) {
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	o := adaptiveOptions()
	o.EndTime = .1
	ctx, cancel := context.WithCancel(context.Background())
	counter := &adaptiveCountContext{Context: ctx, cancel: cancel, after: math.MaxInt64}
	_, e := field.IntegrateAdaptive(counter, fn, geom.V(1, 0), o)
	if e != nil {
		t.Fatal(e)
	}
	total := counter.count.Load()
	cancel()
	for k := int64(1); k <= total; k++ {
		ctx, cancel := context.WithCancel(context.Background())
		counter := &adaptiveCountContext{Context: ctx, cancel: cancel, after: k}
		adaptiveFailure(t, counter, fn, geom.V(1, 0), o, context.Canceled)
		cancel()
	}
	// Callback cancellation is observed before accepting its result.
	ctx, cancel = context.WithCancel(context.Background())
	adaptiveFailure(t, ctx, func(p geom.Vec3) geom.Vec3 { cancel(); return p }, geom.V(1, 0), o, context.Canceled)
	q := field.DefaultAdaptiveStreamOptions()
	q.Integration = o
	ctx, cancel = context.WithCancel(context.Background())
	q.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color { cancel(); return gomath.Red }
	r, e := field.NewAdaptiveStreamLines(ctx, fn, []geom.Vec3{geom.V(1, 0)}, q)
	if r != nil || !errors.Is(e, context.Canceled) {
		t.Fatalf("color cancellation %v %v", r, e)
	}
	// Constructor geometry and completion checkpoints are covered independently.
	q.ColorFunc = nil
	ctx, cancel = context.WithCancel(context.Background())
	counter = &adaptiveCountContext{Context: ctx, cancel: cancel, after: math.MaxInt64}
	_, e = field.NewAdaptiveStreamLines(counter, fn, []geom.Vec3{geom.V(1, 0), geom.V(2, 0)}, q)
	if e != nil {
		t.Fatal(e)
	}
	total = counter.count.Load()
	cancel()
	for k := int64(1); k <= total; k++ {
		ctx, cancel := context.WithCancel(context.Background())
		counter = &adaptiveCountContext{Context: ctx, cancel: cancel, after: k}
		r, e := field.NewAdaptiveStreamLines(counter, fn, []geom.Vec3{geom.V(1, 0), geom.V(2, 0)}, q)
		if r != nil || !errors.Is(e, context.Canceled) {
			t.Fatalf("constructor checkpoint %d %v %v", k, r, e)
		}
		cancel()
	}
}
