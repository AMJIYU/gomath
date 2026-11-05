package field_test

import (
	"gomath/field"
	"math"
	"testing"
)

func FuzzFlowBoundedWindows(f *testing.F) {
	f.Add([]byte{0, 1, 3, 9, 255}, .5, 1.0, true)
	f.Add([]byte{2, 0, 1}, .1, -2.0, false)
	f.Fuzz(func(t *testing.T, data []byte, at, speed float64, loop bool) {
		if len(data) > 32 {
			return
		}
		times := make([]float64, len(data)+1)
		for i, v := range data {
			times[i+1] = times[i] + float64(v+1)/256
		}
		// byte v+1 intentionally wraps at 255; exercise malformed monotonic clocks.
		o := field.DefaultFlowOptions()
		o.Duration = 4
		o.FlowSpeed = speed
		o.Loop = loop
		o.TraceTimeWidth = .125
		src := flowInput(times...)
		a, e := field.NewFlow([]field.FlowLine{src}, o)
		if e != nil {
			if a != nil {
				t.Fatal("partial constructor result")
			}
			return
		}
		if math.IsNaN(at) || math.IsInf(at, 0) || at < 0 || at > o.Duration {
			if fr, e := a.Sample(at); fr != nil || e == nil {
				t.Fatal("invalid sample accepted")
			}
			return
		}
		fr, e := a.Sample(at)
		if e != nil {
			if fr != nil {
				t.Fatal("partial sampled result")
			}
			return
		}
		for _, obj := range fr.Objects {
			if e := obj.Validate(); e != nil {
				t.Fatal(e)
			}
		}
		src.Object.Style.Opacity = 0
		second, e := a.Sample(at)
		if e != nil {
			t.Fatal(e)
		}
		if second.Objects[0].Children[0].Style.Opacity == 0 {
			t.Fatal("source aliases flow")
		}
	})
}
