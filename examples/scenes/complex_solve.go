package scenes

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/linalg"
	"gomath/mathtex"
	"gomath/matrix"
	"gomath/text"
)

// ComplexSolve displays an actual non-Hermitian complex solve and its checked
// original-system residual; no numerical answer is copied into the scene.
func ComplexSolve() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Complex matrices / partial-pivot LU / checked solve")
	if err != nil {
		return nil, err
	}
	data := []complex128{2 + 1i, .5 - .2i, -1 + .3i, 3 - 2i}
	a, err := linalg.NewComplexDense(2, 2, data)
	if err != nil {
		return nil, err
	}
	factor, err := linalg.FactorizeComplex(a)
	if err != nil {
		return nil, err
	}
	b := []complex128{1 + .5i, -2 + 1i}
	x, residual, err := factor.Solve(b)
	if err != nil {
		return nil, err
	}
	format := func(z complex128) string { return fmt.Sprintf("%.2f%+.2fi", real(z), imag(z)) }
	opt := matrix.DefaultOptions()
	opt.Math.Size = .37
	left, err := matrix.New([][]string{{format(data[0]), format(data[1])}, {format(data[2]), format(data[3])}}, opt)
	if err != nil {
		return nil, err
	}
	left.MoveTo(geom.V(-3, 1))
	right, err := matrix.New([][]string{{format(b[0])}, {format(b[1])}}, opt)
	if err != nil {
		return nil, err
	}
	right.MoveTo(geom.V(3, 1))
	formula, err := mathtex.New(`Az=b`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.MoveTo(geom.V(0, 2.2))
	solution, err := mathtex.New(fmt.Sprintf(`z=(%s,%s)`, format(x[0]), format(x[1])), mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	solution.Scale(.7).MoveTo(geom.V(0, -.8)).SetColor(gomath.Blue)
	phase, logAbs, err := factor.LogDet()
	if err != nil {
		return nil, err
	}
	textOpt := text.DefaultOptions()
	textOpt.Size = .29
	diagnostic, err := text.New(fmt.Sprintf("relative residual %.2e\ndet phase %s   /   log(abs(det)) %.5f", residual, format(phase), logAbs), text.DefaultFont(), textOpt)
	if err != nil {
		return nil, err
	}
	diagnostic.MoveTo(geom.V(0, -2.3))
	if err = s.Add(heading, formula, diagnostic); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Write(left.Object), gomath.Write(right.Object), gomath.Write(solution)); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
