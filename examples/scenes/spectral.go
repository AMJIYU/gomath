package scenes

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/linalg"
	"gomath/mathtex"
	"gomath/matrix"
	"math"
)

// Spectral draws real computed eigenvectors and solves a wide system with the
// SVD minimum-norm solver. Displayed values are computed, not copied constants.
func Spectral() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Symmetric eigenvectors / wide SVD / minimum-norm solve")
	if err != nil {
		return nil, err
	}
	a, err := linalg.NewDense(2, 2, []float64{2, .7, .7, 1})
	if err != nil {
		return nil, err
	}
	eigen, err := linalg.FactorizeEigenSym(a)
	if err != nil {
		return nil, err
	}
	vectors, err := eigen.Vectors()
	if err != nil {
		return nil, err
	}
	values, v := eigen.Values(), vectors.Data()
	center := geom.V(-3.2, -.3)
	circle := gomath.Circle(1).MoveTo(center).SetStroke(gomath.Blue, .035)
	m := geom.Identity()
	m[0], m[1], m[4], m[5] = 2, .7, .7, 1
	ellipse := circle.Copy().ApplyMatrix(geom.About(center, m)).SetStroke(gomath.Pink, .045)
	colors := []gomath.Color{gomath.Yellow, gomath.Green}
	var arrows []*gomath.Object
	for j, lambda := range values {
		endpoint := center.Add(geom.V(v[j]*lambda, v[2+j]*lambda))
		arrows = append(arrows, gomath.Arrow(center, endpoint, .16).SetColor(colors[j]))
	}
	wide, err := linalg.NewDense(2, 3, []float64{1, 0, 1, 0, 1, 1})
	if err != nil {
		return nil, err
	}
	svd, err := linalg.FactorizeSVD(wide)
	if err != nil {
		return nil, err
	}
	x, residual, err := svd.Solve([]float64{1, 1})
	if err != nil {
		return nil, err
	}
	if residual > 1e-12 || math.Abs(x[0]+x[1]-x[2]) > 1e-12 {
		return nil, fmt.Errorf("wide solve failed residual/nullspace check")
	}
	data := [][]string{{"1", "0", "1"}, {"0", "1", "1"}}
	display, err := matrix.New(data, matrix.DefaultOptions())
	if err != nil {
		return nil, err
	}
	display.Object.Scale(.8).MoveTo(geom.V(3.1, 1.15))
	formula, err := mathtex.New(fmt.Sprintf(`x=(%.3f,%.3f,%.3f)`, x[0], x[1], x[2]), mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.Scale(.65).MoveTo(geom.V(3.1, -.5))
	sig := svd.Values()
	info, err := mathtex.New(fmt.Sprintf(`\sigma=(%.3f,%.3f)\quad rank=%d`, sig[0], sig[1], svd.Rank()), mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	info.Scale(.6).MoveTo(geom.V(3.1, -1.5))
	if err = s.Add(label, circle, display.Object, formula, info); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Transform(circle, ellipse), gomath.GrowArrow(arrows[0]), gomath.GrowArrow(arrows[1])); err != nil {
		return nil, err
	}
	if err = s.Play(1.2, gomath.Indicate(formula)); err != nil {
		return nil, err
	}
	if err = s.Wait(.8); err != nil {
		return nil, err
	}
	return s, nil
}
