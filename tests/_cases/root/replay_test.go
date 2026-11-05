package gomath_test

import (
	"context"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"image"
	"math"
	"sync"
	"testing"
)

func TestReplayIntegratesThenRedrawsWithRandomAccess(t *testing.T) {
	value, err := gomath.NewValueTracker(0)
	if err != nil {
		t.Fatal(err)
	}
	dot := gomath.Dot(geom.V(0, 0), .1)
	s := gomath.NewScene()
	if err := s.Add(value.Object, dot); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRedraw(dot, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		x, err := f.Value(value.ID)
		if err != nil {
			return nil, err
		}
		return gomath.Dot(geom.V(x, math.Sin(x)), .1), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(1.05); err != nil {
		t.Fatal(err)
	}
	opt := gomath.DefaultReplayOptions()
	opt.Step, opt.CheckpointEvery = .1, 3
	r, err := gomath.CompileReplay(s, opt, gomath.Integration{Object: value.Object, Update: func(state *gomath.Object, dt, time float64) error {
		// Backward rectangular integration of x'=t, so partial steps have a
		// known result and expose wrong dt or shared-checkpoint mutation.
		state.Scalar += dt * time
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Duration() != 1.05 || value.Value() != 0 {
		t.Fatal("compile mutated authoring state")
	}
	if err := value.SetValue(100); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(.2); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, test := range []struct{ time, want float64 }{{1.05, .6025}, {.25, .0425}, {0, 0}, {.3, .06}, {.25, .0425}, {1, .55}} {
		wg.Add(1)
		go func(time, want float64) {
			defer wg.Done()
			frame, err := r.Sample(time)
			if err != nil {
				t.Error(err)
				return
			}
			got, err := frame.Value(value.ID)
			if err != nil || math.Abs(got-want) > 1e-12 {
				t.Error("wrong integrated value", time, got, want, err)
				return
			}
			if frame.Find(dot.ID).Center().Sub(geom.V(want, math.Sin(want))).Norm() > 1e-10 {
				t.Error("redraw did not see integrated value")
			}
			frame.Find(value.ID).Scalar = -999
		}(test.time, test.want)
	}
	wg.Wait()
	if r.Duration() != 1.05 {
		t.Fatal("replay did not freeze scene timeline")
	}
}

func TestReplayRejectsAmbiguousStateAndBadCallbacks(t *testing.T) {
	opt := gomath.DefaultReplayOptions()
	for _, kind := range []string{"animated", "changed", "removed", "overlap", "topology", "identity", "nonfinite"} {
		t.Run(kind, func(t *testing.T) {
			o := gomath.Square(1)
			s := gomath.NewScene()
			if err := s.Add(o); err != nil {
				t.Fatal(err)
			}
			updates := []gomath.Integration{{Object: o, Update: func(*gomath.Object, float64, float64) error { return nil }}}
			switch kind {
			case "animated":
				if err := s.Play(1, gomath.Shift(o, geom.V(1, 0))); err != nil {
					t.Fatal(err)
				}
			case "changed":
				if err := s.Wait(.5); err != nil {
					t.Fatal(err)
				}
				o.Shift(geom.V(1, 0))
				if err := s.Wait(.5); err != nil {
					t.Fatal(err)
				}
			case "removed":
				if err := s.Wait(.5); err != nil {
					t.Fatal(err)
				}
				s.Remove(o)
			case "overlap":
				updates = append(updates, updates[0])
			case "topology":
				updates[0].Update = func(state *gomath.Object, _, _ float64) error {
					state.Children = append(state.Children, gomath.Square(1))
					return nil
				}
			case "identity":
				updates[0].Update = func(state *gomath.Object, _, _ float64) error { state.ID++; return nil }
			case "nonfinite":
				updates[0].Update = func(state *gomath.Object, _, _ float64) error { state.Scalar = math.NaN(); return nil }
			}
			if s.Duration() == 0 {
				if err := s.Wait(.1); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := gomath.CompileReplay(s, opt, updates...); err == nil {
				t.Fatal("ambiguous replay accepted")
			}
		})
	}
	s := gomath.NewScene()
	if err := s.Wait(2); err != nil {
		t.Fatal(err)
	}
	opt.MaxSteps = 2
	if _, err := gomath.CompileReplay(s, opt); err == nil {
		t.Fatal("step budget ignored")
	}
}

type replaySink struct{ count int }

func (s *replaySink) WriteFrame(_ int, _ *image.RGBA) error { s.count++; return nil }
func (*replaySink) Close() error                            { return nil }
func (*replaySink) Abort() error                            { return nil }

func TestReplayRendersThroughBoundedWorkers(t *testing.T) {
	s := gomath.NewScene()
	dot := gomath.Dot(geom.V(-1, 0), .15)
	if err := s.Add(dot); err != nil {
		t.Fatal(err)
	}
	if err := s.Wait(1); err != nil {
		t.Fatal(err)
	}
	r, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions(), gomath.Integration{Object: dot, Update: func(state *gomath.Object, dt, _ float64) error { state.Shift(geom.V(dt, 0)); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FPS, c.Workers = 128, 72, 12, 3
	sink := &replaySink{}
	if _, err := render.RenderScene(context.Background(), r, c, sink); err != nil || sink.count != 12 {
		t.Fatal("replay render failed", err, sink.count)
	}
}
