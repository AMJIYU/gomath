package gomath_test

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"testing"
)

func BenchmarkObjectUpdaterFrame(b *testing.B) {
	for _, count := range []int{1, 32, 128} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			scene := gomath.NewScene()
			children := make([]*gomath.Object, count)
			for i := range children {
				children[i] = gomath.Square(.08).MoveTo(geom.V(float64(i)*.1, 0))
			}
			if e := scene.Add(gomath.Group(children...)); e != nil {
				b.Fatal(e)
			}
			for i, child := range children {
				x := float64(i) * .1
				if _, e := scene.AddObjectUpdater(child, func(o *gomath.Object, _ *gomath.Frame, t float64) error { o.MoveTo(geom.V(x, t)); return nil }, gomath.DefaultObjectUpdaterOptions()); e != nil {
					b.Fatal(e)
				}
			}
			if e := scene.Wait(2); e != nil {
				b.Fatal(e)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, e := scene.Sample(.75); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}
