package scenes

import (
	"context"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/path"
	"gomath/sparse"
	"gomath/text"
)

// SparseSolve computes a Dirichlet one-dimensional Poisson system, then draws
// the returned discrete solution. The graph is not a hard-coded parabola.
func SparseSolve() (*gomath.Scene, error) {
	const n = 31
	h := 1 / float64(n+1)
	rowPtr := make([]int, n+1)
	var columns []int
	var values []float64
	rhs := make([]float64, n)
	for i := range n {
		rowPtr[i] = len(columns)
		if i > 0 {
			columns = append(columns, i-1)
			values = append(values, -1)
		}
		columns = append(columns, i)
		values = append(values, 2)
		if i+1 < n {
			columns = append(columns, i+1)
			values = append(values, -1)
		}
		rhs[i] = h * h
	}
	rowPtr[n] = len(columns)
	a, err := sparse.NewCSR(n, n, rowPtr, columns, values)
	if err != nil {
		return nil, err
	}
	options := sparse.DefaultCGOptions()
	options.Preconditioner = sparse.Jacobi
	result, err := sparse.SolveCG(context.Background(), a, rhs, options)
	if err != nil {
		return nil, err
	}
	solution := result.Solution()
	axes, err := graph.NewAxes(graph.Range{Min: 0, Max: 1, Step: .1}, graph.Range{Min: -.02, Max: .14, Step: .02}, 9, 4)
	if err != nil {
		return nil, err
	}
	var builder path.Builder
	builder.MoveTo(axes.Point(0, 0))
	for i, value := range solution {
		builder.LineTo(axes.Point(float64(i+1)*h, value))
	}
	builder.LineTo(axes.Point(1, 0))
	curve := gomath.NewObject(builder.Path()).SetStroke(gomath.Blue, .05)
	heading, err := title("Sparse CSR / conjugate gradient / computed Poisson solution")
	if err != nil {
		return nil, err
	}
	opt := text.DefaultOptions()
	opt.Size = .3
	caption, err := text.New(fmt.Sprintf("%d unknowns / %d stored coefficients / %d iterations\noriginal-system relative residual %.2e", n, a.NNZ(), result.Iterations(), result.RelativeResidual()), text.DefaultFont(), opt)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.6))
	s := gomath.NewScene()
	if err = s.Add(heading, axes.Object, caption); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.Create(curve)); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
