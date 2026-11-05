package gomath_test

import (
	"gomath"
	"math"
	"reflect"
	"testing"
)

func TestStrokeStyleValidationAndExtent(t *testing.T) {
	if gomath.CapRound != 0 || gomath.JoinMiter != 0 {
		t.Fatal("stroke zero values changed")
	}
	s := gomath.DefaultStyle()
	s.StrokeWidth = 2
	if err := s.Validate(); err != nil || s.EffectiveMiterLimit() != 4 || s.StrokeExtent() != 4 {
		t.Fatal("invalid default stroke", s, err)
	}
	for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
		for _, cap := range []gomath.StrokeCap{gomath.CapRound, gomath.CapButt, gomath.CapSquare} {
			s.StrokeCap, s.StrokeJoin, s.MiterLimit = cap, join, 1
			want := 1.0
			if cap == gomath.CapSquare {
				want = math.Sqrt2
			}
			if err := s.Validate(); err != nil || s.StrokeExtent() != want {
				t.Fatalf("cap=%v join=%v: extent=%g want=%g err=%v", cap, join, s.StrokeExtent(), want, err)
			}
		}
	}
	for _, limit := range []float64{-1, .5, math.NaN(), math.Inf(1), math.Inf(-1)} {
		s.MiterLimit = limit
		if s.Validate() == nil {
			t.Errorf("accepted miter limit %g", limit)
		}
	}
	s = gomath.DefaultStyle()
	s.StrokeCap = gomath.StrokeCap(255)
	if s.Validate() == nil {
		t.Fatal("accepted unknown cap")
	}
	s = gomath.DefaultStyle()
	s.StrokeJoin = gomath.StrokeJoin(255)
	if s.Validate() == nil {
		t.Fatal("accepted unknown join")
	}
	o := gomath.Square(1)
	o.Style = s
	if gomath.NewScene().Add(o) == nil {
		t.Fatal("scene accepted invalid stroke style")
	}
}

func TestStrokeStyleTransformSampling(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(2)
	source := o.Style
	target := o.Copy()
	target.Style.StrokeCap = gomath.CapSquare
	target.Style.StrokeJoin = gomath.JoinBevel
	target.Style.MiterLimit = 8
	must(t, s.Add(o))
	must(t, s.Play(1, gomath.Transform(o, target).WithRate(gomath.Linear)))
	for _, time := range []float64{0, .25, .5, .75, 1} {
		style := sample(t, s, time).Find(o.ID).Style
		must(t, style.Validate())
		if time == 0 && style != source {
			t.Fatal("source endpoint changed", style)
		}
		if time > 0 && time < 1 {
			if style.MiterLimit != 4+4*time || style.StrokeCap != source.StrokeCap || style.StrokeJoin != source.StrokeJoin {
				t.Fatalf("bad intermediate style at %g: %+v", time, style)
			}
		}
		if time == 1 && !reflect.DeepEqual(style, target.Style) {
			t.Fatal("target style lost", style)
		}
	}
	// Sampling style fields must not mutate the source, target or later frames.
	f := sample(t, s, .5)
	f.Find(o.ID).Style.MiterLimit = 100
	if sample(t, s, .5).Find(o.ID).Style.MiterLimit != 6 || target.Style.MiterLimit != 8 {
		t.Fatal("sampled styles alias authoring data")
	}
}
