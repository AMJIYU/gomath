package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
)

func FormulaColors() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Formula colors / scoped declarations / real vector components")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	for i, source := range []string{
		`{\color[HTML]{58C4DD}\dfrac{1+x^{2}}{1-x^{2}}}+{\color[HTML]{C850B0}\sqrt{1+x^{2}}}`,
		`\textcolor[HTML]{F5CD50}{\displaystyle\sum_{\substack{0\leq k\leq n\\k\neq 2}}^{n}\dbinom{n}{k}x^{k}}`,
		`\textcolor{cyan}{\mathbb{R}}\quad\textcolor{magenta}{\vec{v}}\quad\textcolor{orange}{\begin{pmatrix}a&b\\c&d\end{pmatrix}}`,
	} {
		options := mathtex.DefaultOptions()
		options.Size = .5
		formula, err := mathtex.New(source, options)
		if err != nil {
			return nil, err
		}
		formula.MoveTo(geom.V(0, 1.65-float64(i)*1.85))
		if err = s.Play(.6, gomath.Write(formula)); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
