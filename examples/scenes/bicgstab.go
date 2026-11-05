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
	"math"
)

// BiCGSTAB solves a nonsymmetric tridiagonal system and draws the returned
// solution. Its RHS is independently formed from a smooth manufactured vector.
func BiCGSTAB() (*gomath.Scene, error) {
	const n = 32
	h := 1 / float64(n+1)
	expected := make([]float64, n)
	for i := range n {
		x := float64(i+1) * h
		expected[i] = math.Sin(math.Pi*x) * (1 + .5*x)
	}
	ptr := make([]int, n+1)
	columns := make([]int, 0, 3*n-2)
	values := make([]float64, 0, 3*n-2)
	rhs := make([]float64, n)
	for i := range n {
		ptr[i] = len(columns)
		if i > 0 {
			columns, values = append(columns, i-1), append(values, -1)
			rhs[i] -= expected[i-1]
		}
		columns, values = append(columns, i), append(values, 3)
		rhs[i] += 3 * expected[i]
		if i+1 < n {
			columns, values = append(columns, i+1), append(values, -.5)
			rhs[i] -= .5 * expected[i+1]
		}
	}
	ptr[n] = len(columns)
	a, err := sparse.NewCSR(n, n, ptr, columns, values)
	if err != nil {
		return nil, err
	}
	options := sparse.DefaultBiCGSTABOptions()
	options.Preconditioner = sparse.Jacobi
	result, err := sparse.SolveBiCGSTAB(context.Background(), a, rhs, options)
	if err != nil {
		return nil, err
	}
	solution := result.Solution()
	actual, err := a.MulVec(solution)
	if err != nil {
		return nil, err
	}
	rnorm, bnorm := 0.0, 0.0
	for i := range actual {
		rnorm, bnorm = math.Hypot(rnorm, actual[i]-rhs[i]), math.Hypot(bnorm, rhs[i])
	}
	if rnorm/bnorm > 1e-9 {
		return nil, fmt.Errorf("BiCGSTAB example failed independent residual check: %g", rnorm/bnorm)
	}
	axes, err := graph.NewAxes(graph.Range{Min: 0, Max: 1, Step: .1}, graph.Range{Min: -.05, Max: 1.5, Step: .25}, 9, 4)
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
	heading, err := title("Nonsymmetric CSR / BiCGSTAB / verified original-system residual")
	if err != nil {
		return nil, err
	}
	heading.Scale(.88)
	optionsText := text.DefaultOptions()
	optionsText.Size = .3
	caption, err := text.New(fmt.Sprintf("%d unknowns / %d coefficients / %d iterations\nrecomputed relative residual %.2e", n, a.NNZ(), result.Iterations(), rnorm/bnorm), text.DefaultFont(), optionsText)
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
