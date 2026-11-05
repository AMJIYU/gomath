package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"sync"
	"testing"
)

func TestValueTrackerAndRedrawRandomAccess(t *testing.T) {
	v, err := gomath.NewValueTracker(-2)
	if err != nil {
		t.Fatal(err)
	}
	dot := gomath.Dot(geom.V(-2, 4), .1)
	s := gomath.NewScene()
	if err := s.Add(v.Object, dot); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRedraw(dot, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		x, err := f.Value(v.ID)
		if err != nil {
			return nil, err
		}
		return gomath.Dot(geom.V(x, x*x), .1), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(2, gomath.ValueTo(v, 2).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if v.Value() != 2 {
		t.Fatal("authoring tracker not committed")
	}
	var wg sync.WaitGroup
	for _, time := range []float64{1.5, 0, 1, .5, 2, 1.5} {
		wg.Add(1)
		go func(time float64) {
			defer wg.Done()
			f, err := s.Sample(time)
			if err != nil {
				t.Error(err)
				return
			}
			x, err := f.Value(v.ID)
			if err != nil {
				t.Error(err)
				return
			}
			want := -2 + 2*time
			if math.Abs(x-want) > 1e-10 || f.Find(dot.ID).Center().Sub(geom.V(want, want*want)).Norm() > 1e-10 {
				t.Error("sampled value or redraw differs", time, x)
			}
			f.Find(dot.ID).Shift(geom.V(100, 100))
		}(time)
	}
	wg.Wait()
	if dot.Center().Sub(geom.V(-2, 4)).Norm() > 1e-10 {
		t.Fatal("redraw mutated authoring object")
	}
}

func TestTrackerAndRedrawErrors(t *testing.T) {
	if _, err := gomath.NewValueTracker(math.NaN()); err == nil {
		t.Fatal("NaN tracker accepted")
	}
	v, _ := gomath.NewValueTracker(1)
	if err := v.SetValue(math.Inf(1)); err == nil || v.Value() != 1 {
		t.Fatal("invalid set mutated value")
	}
	s := gomath.NewScene()
	if err := s.Add(v.Object); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.ValueTo(v, math.NaN())); err == nil || s.Duration() != 0 || v.Value() != 1 {
		t.Fatal("invalid animation committed")
	}
	if err := s.AddRedraw(v.Object, func(*gomath.Frame, float64) (*gomath.Object, error) { return gomath.Group(gomath.Square(1)), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sample(0); err == nil {
		t.Fatal("changed redraw topology accepted")
	}
	if err := s.AddRedraw(gomath.Square(1), func(*gomath.Frame, float64) (*gomath.Object, error) { return nil, nil }); err == nil {
		t.Fatal("unowned redraw object accepted")
	}
}
