package scenes

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/matrix"
	"gomath/sparse"
	"gomath/text"
)

func SparseProduct() (*gomath.Scene, error) {
	a, err := sparse.NewCSR(3, 3, []int{0, 2, 5, 7}, []int{0, 1, 0, 1, 2, 1, 2}, []float64{2, -1, -1, 2, -1, -1, 2})
	if err != nil {
		return nil, err
	}
	b, err := sparse.NewCSR(3, 2, []int{0, 1, 2, 4}, []int{0, 1, 0, 1}, []float64{1, 1, 1, 1})
	if err != nil {
		return nil, err
	}
	product, err := a.Mul(b)
	if err != nil {
		return nil, err
	}
	s := gomath.NewScene()
	heading, err := title("Sparse matrix product / actual CSR multiplication / sorted output")
	if err != nil {
		return nil, err
	}
	heading.Scale(.85)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	for i, data := range []*sparse.CSR{a, b, product} {
		rows, cols := data.Dims()
		values := make([][]int64, rows)
		for r := range rows {
			values[r] = make([]int64, cols)
			for c := range cols {
				value, err := data.At(r, c)
				if err != nil {
					return nil, err
				}
				values[r][c] = int64(value)
			}
		}
		display, err := matrix.NewIntegers(values, matrix.DefaultOptions())
		if err != nil {
			return nil, err
		}
		display.Object.MoveTo(geom.V([]float64{-4.3, 0, 4}[i], 0)).SetColor([]gomath.Color{gomath.Blue, gomath.Pink, gomath.Yellow}[i])
		if err = s.Play(.6, gomath.Write(display.Object)); err != nil {
			return nil, err
		}
	}
	opt := text.DefaultOptions()
	opt.Size = .35
	for i, value := range []string{"x", "="} {
		label, err := text.New(value, text.DefaultFont(), opt)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(float64(i)*4-2, 0))
		if err = s.Add(label); err != nil {
			return nil, err
		}
	}
	caption, err := text.New(fmt.Sprintf("A: %d stored / B: %d stored / A*B: %d stored / no dense workspace", a.NNZ(), b.NNZ(), product.NNZ()), text.DefaultFont(), opt)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.5))
	if err = s.Add(caption); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
