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

// Schur displays computed real factors and complex eigenvalues of a nonnormal
// real matrix. It does not label Schur vectors as general eigenvectors.
func Schur() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Real Schur / complex eigenvalues / checked reconstruction")
	if err != nil {
		return nil, err
	}
	base, err := linalg.NewDense(3, 3, []float64{0, -2, 1, 2, 0, 3, 0, 0, 5})
	if err != nil {
		return nil, err
	}
	rotation := geom.Rotation(geom.Vec3{X: 1, Y: 2, Z: 3}, .7)
	qData := []float64{rotation[0], rotation[1], rotation[2], rotation[4], rotation[5], rotation[6], rotation[8], rotation[9], rotation[10]}
	q0, err := linalg.NewDense(3, 3, qData)
	if err != nil {
		return nil, err
	}
	qt, err := q0.Transpose()
	if err != nil {
		return nil, err
	}
	a, err := q0.Mul(base)
	if err != nil {
		return nil, err
	}
	a, err = a.Mul(qt)
	if err != nil {
		return nil, err
	}
	factors, err := linalg.FactorizeSchur(a)
	if err != nil {
		return nil, err
	}
	_, triangular, err := factors.Factors()
	if err != nil {
		return nil, err
	}
	makeDisplay := func(dense *linalg.Dense, x float64) (*gomath.Object, error) {
		data := dense.Data()
		rows := [][]float64{data[:3], data[3:6], data[6:]}
		opt := matrix.DefaultOptions()
		opt.Math.Size = .38
		opt.ColumnGap = .22
		opt.RowGap = .3
		display, err := matrix.NewDecimals(rows, 2, opt)
		if err != nil {
			return nil, err
		}
		return display.Object.MoveTo(geom.V(x, .4)), nil
	}
	left, err := makeDisplay(a, -3.5)
	if err != nil {
		return nil, err
	}
	right, err := makeDisplay(triangular, 3.5)
	if err != nil {
		return nil, err
	}
	formula, err := mathtex.New(`A=QTQ^T`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.MoveTo(geom.V(0, 2.1))
	values := factors.Values()
	valueLabel, err := mathtex.New(fmt.Sprintf(`\lambda=(%.2f%+.2fi,%.2f%+.2fi,%.2f%+.2fi)`, real(values[0]), imag(values[0]), real(values[1]), imag(values[1]), real(values[2]), imag(values[2])), mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	valueLabel.Scale(.65).MoveTo(geom.V(0, -1.6)).SetColor(gomath.Blue)
	opt := text.DefaultOptions()
	opt.Size = .28
	residual, err := text.New(fmt.Sprintf("relative reconstruction %.2e   /   orthogonality %.2e", factors.RelativeReconstructionError(), factors.OrthogonalityError()), text.DefaultFont(), opt)
	if err != nil {
		return nil, err
	}
	residual.MoveTo(geom.V(0, -2.6))
	if err = s.Add(heading, formula, residual); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.Write(left), gomath.Write(right), gomath.Write(valueLabel)); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
