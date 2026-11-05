package sparse

import "testing"

func BenchmarkSpGEMM(b *testing.B) {
	for _, n := range []int{256, 1024} {
		name := "band_n256"
		if n == 1024 {
			name = "band_n1024"
		}
		b.Run(name, func(b *testing.B) {
			a := tridiagonal(b, n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				out, e := a.Mul(a)
				if e != nil {
					b.Fatal(e)
				}
				b.ReportMetric(float64(out.NNZ()), "output-nnz/op")
			}
		})
	}
	b.Run("fan_in64_columns128", func(b *testing.B) {
		p, ci, v := []int{0, 64}, make([]int, 64), make([]float64, 64)
		for i := range ci {
			ci[i] = i
			v[i] = 1
		}
		a := must(b, 1, 64, p, ci, v)
		p, ci, v = make([]int, 65), make([]int, 64*128), make([]float64, 64*128)
		for i := 0; i < 64; i++ {
			p[i+1] = (i + 1) * 128
			for j := 0; j < 128; j++ {
				ci[i*128+j] = j
				v[i*128+j] = 1
			}
		}
		right := must(b, 64, 128, p, ci, v)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, e := a.Mul(right); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("wide_1048576_columns_two_entries", func(b *testing.B) {
		a := diagonal(b, []float64{2})
		right := must(b, 1, 1<<20, []int{0, 2}, []int{0, 1<<20 - 1}, []float64{3, 4})
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, e := a.Mul(right); e != nil {
				b.Fatal(e)
			}
		}
	})
}
