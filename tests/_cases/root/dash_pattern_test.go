package gomath

import (
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestDashPatternImmutableValidationAndScaling(t *testing.T) {
	input := []float64{2, 0, 3}
	p, err := NewDashPattern(input, -23)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = 100
	if want := []float64{2, 0, 3, 2, 0, 3}; !reflect.DeepEqual(p.Lengths(), want) || p.Offset() != 7 || p.Period() != 10 {
		t.Fatalf("canonical pattern %v offset=%g period=%g", p.Lengths(), p.Offset(), p.Period())
	}
	output := p.Lengths()
	output[0] = 200
	if p.Lengths()[0] != 2 {
		t.Fatal("Lengths exposed mutable storage")
	}
	scaled, err := p.Scaled(2)
	if err != nil {
		t.Fatal(err)
	}
	if scaled.Offset() != 14 || scaled.Period() != 20 || scaled.Lengths()[0] != 4 || p.Lengths()[0] != 2 {
		t.Fatal("immutable scale failed")
	}
	for _, tc := range []struct {
		v      []float64
		offset float64
	}{
		{nil, 0}, {[]float64{0, 0}, 0}, {[]float64{-1, 1}, 0}, {[]float64{math.NaN(), 1}, 0}, {[]float64{math.Inf(1), 1}, 0},
		{[]float64{math.MaxFloat64, math.MaxFloat64}, 0}, {[]float64{1, 2}, math.NaN()}, {[]float64{1, 2}, math.Inf(1)},
		{make([]float64, MaxDashEntries+1), 0},
	} {
		if _, err := NewDashPattern(tc.v, tc.offset); err == nil {
			t.Fatalf("accepted %v offset %g", tc.v, tc.offset)
		}
	}
	for _, s := range []float64{0, -1, math.NaN(), math.Inf(1), math.MaxFloat64} {
		if _, err := p.Scaled(s); err == nil {
			t.Fatalf("accepted scale %g", s)
		}
	}
	style := DefaultStyle()
	style.Dash = &DashPattern{}
	if style.Validate() == nil {
		t.Fatal("accepted unconstructed pattern")
	}
	style.Dash = p
	if err := style.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{-30, -20, -10, 0, 10, 20, 30} {
		q, e := NewDashPattern([]float64{2, 3}, v)
		if e != nil || q.Offset() != 0 {
			t.Fatalf("offset modulus %g -> %v %v", v, q, e)
		}
	}
}

func TestDashPatternCopyLerpAndConcurrentOwnership(t *testing.T) {
	a, _ := NewDashPattern([]float64{2, 3}, 1)
	b, _ := NewDashPattern([]float64{4, 5}, 2)
	source := DefaultStyle()
	source.Dash = a
	target := source
	target.Dash = b
	for _, alpha := range []float64{0, .25, .5, .999, 1} {
		got := source.lerp(target, alpha)
		want := a
		if alpha == 1 {
			want = b
		}
		if got.Dash != want {
			t.Fatalf("lerp at %g changed discrete dash", alpha)
		}
	}
	object := Square(2)
	object.Style = source
	copy := object.Copy()
	if copy.ID == object.ID || copy.Style.Dash != a {
		t.Fatal("copy must share only immutable pattern")
	}
	copy.Style.Dash = b
	if object.Style.Dash != a {
		t.Fatal("copy changed authoring style")
	}
	// Sampling and style replacement use copies; immutable getters remain safe
	// even when independent workers alter their returned length slices.
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				snapshot := object.Copy()
				v := snapshot.Style.Dash.Lengths()
				v[0] = 99
				if snapshot.Style.Dash.Offset() != 1 || object.Style.Dash.Lengths()[0] != 2 {
					t.Error("shared dash was mutated")
				}
			}
		}()
	}
	workers.Wait()
}

func TestDashPatternScaleUnderflow(t *testing.T) {
	p, err := NewDashPattern([]float64{math.SmallestNonzeroFloat64, 1}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Scaled(.5); err == nil {
		t.Fatal("accepted positive dash underflowing to zero")
	}
	tiny, err := NewDashPattern([]float64{2, 3}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tiny.Scaled(math.SmallestNonzeroFloat64); err != nil {
		t.Fatalf("representable positive subnormal lengths rejected: %v", err)
	}
}

func TestDashPatternSceneSamplingOwnership(t *testing.T) {
	source, _ := NewDashPattern([]float64{.2, .1}, .05)
	target, _ := NewDashPattern([]float64{.4, .2}, .1)
	o := Square(2)
	o.Style.Dash = source
	destination := o.Copy()
	destination.Style.Dash = target
	scene := NewScene()
	if err := scene.Add(o); err != nil {
		t.Fatal(err)
	}
	if err := scene.Play(1, Transform(o, destination).WithRate(Linear)); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for _, time := range []float64{0, .25, .5, .75, 1} {
		workers.Add(1)
		go func(time float64) {
			defer workers.Done()
			frame, err := scene.Sample(time)
			if err != nil {
				t.Error(err)
				return
			}
			got := frame.Find(o.ID)
			want := source
			if time == 1 {
				want = target
			}
			if got.Style.Dash != want {
				t.Errorf("frame %g selected wrong discrete dash", time)
			}
			got.Style.Dash = nil
			repeated, err := scene.Sample(time)
			if err != nil {
				t.Error(err)
				return
			}
			if repeated.Find(o.ID).Style.Dash != want {
				t.Error("sample style replacement affected replay")
			}
		}(time)
	}
	workers.Wait()
}
