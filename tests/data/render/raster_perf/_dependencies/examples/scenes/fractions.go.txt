package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
)

func Fractions() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Native formula layout / fractions / binomials / stacked limits")
	if err != nil {
		return nil, err
	}
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	formulas := []string{
		`\dfrac{1+x^{2}}{1-x^{2}}\qquad\tfrac{1+x^{2}}{1-x^{2}}`,
		`\dbinom{n}{k}\qquad\tbinom{n}{k}\qquad\binom{n+1}{k+1}`,
		`\displaystyle\sum_{\substack{0\leq k\leq n\\k\neq 2}}\binom{n}{k}x^{k}`,
	}
	for i, source := range formulas {
		options := mathtex.DefaultOptions()
		options.Size = .55
		options.Color = gomath.Blue.Lerp(gomath.Pink, float64(i)/2)
		formula, err := mathtex.New(source, options)
		if err != nil {
			return nil, err
		}
		formula.MoveTo(geom.V(0, 1.7-float64(i)*1.8))
		if err = s.Play(.7, gomath.Write(formula)); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
