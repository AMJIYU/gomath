package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func MultilineMath() (*gomath.Scene, error) {
	aligned, err := mathtex.New(`\begin{aligned}\textcolor[HTML]{62D8FF}{x+y}&=3 & u+v&=7\\2x-y&=0 & u-v&=1\end{aligned}`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	gathered, err := mathtex.New(`\begin{gathered}\textcolor[HTML]{EFA4DC}{\frac{1}{2}+\frac{1}{3}}\\=\frac{5}{6}\\\left(\frac{5}{6}\right)^2=\frac{25}{36}\end{gathered}`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	aligned.Scale(.85).MoveTo(geom.V(-3.3, -.1))
	gathered.Scale(.9).MoveTo(geom.V(3.2, -.1))
	heading, err := title("Multiline math / aligned column pairs / centered gathered rows")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	options := text.DefaultOptions()
	options.Size = .3
	left, err := text.New("aligned / independent right-left pairs", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	right, err := text.New("gathered / shared center and baseline boxes", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	left.MoveTo(geom.V(-3.3, -2.4))
	right.MoveTo(geom.V(3.2, -2.4))
	s := gomath.NewScene()
	if err = s.Add(heading, left, right); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Write(aligned), gomath.Write(gathered)); err != nil {
		return nil, err
	}
	if err = s.Wait(1.5); err != nil {
		return nil, err
	}
	return s, nil
}
