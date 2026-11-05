package gomath

import (
	"gomath/geom"
	"testing"
)

var benchmarkAnimation Animation
var benchmarkFrame *Frame

func benchmarkFamily() *Object {
	children := make([]*Object, 32)
	for i := range children {
		children[i] = Circle(.1).Shift(geom.V(float64(i)/4, 0))
	}
	return Group(children...)
}

// This compares authoring-constructor costs on a static source. It excludes
// Play, deferred preparation, sampling and rendering. The explicit-copy control
// captures geometry immediately, while relative operations defer it to Play.
func BenchmarkRelativeAnimationConstruction(b *testing.B) {
	o, delta := benchmarkFamily(), geom.V(2, 0)
	b.Run("explicit-copy-target", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkAnimation = Transform(o, o.Copy().Shift(delta))
		}
	})
	b.Run("deferred-shift", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			benchmarkAnimation = Shift(o, delta)
		}
	})
}

func BenchmarkSharedSequenceSampling(b *testing.B) {
	for _, sequence := range []bool{false, true} {
		name := "explicit-plays"
		if sequence {
			name = "succession"
		}
		b.Run(name, func(b *testing.B) {
			s, o := NewScene(), benchmarkFamily()
			if err := s.Add(o); err != nil {
				b.Fatal(err)
			}
			if sequence {
				if err := s.Play(4, Succession(Shift(o, geom.V(2, 0)), Rotate(o, 1), Scale(o, 1.2), Shift(o, geom.V(1, 0)))); err != nil {
					b.Fatal(err)
				}
			} else {
				for _, a := range []Animation{Shift(o, geom.V(2, 0)), Rotate(o, 1), Scale(o, 1.2), Shift(o, geom.V(1, 0))} {
					if err := s.Play(1, a); err != nil {
						b.Fatal(err)
					}
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchmarkFrame, err = s.Sample(2.4)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
