package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/table"
	"gomath/text"
)

// TableLabels uses separate header objects and data selections. Header labels
// and background cells remain ordinary editable children of the whole table.
func TableLabels() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Tables / owned row and column labels / stable selections")
	if err != nil {
		return nil, err
	}
	makeLabel := func(value string) (*gomath.Object, error) {
		opt := text.DefaultOptions()
		opt.Size = .4
		opt.Color = gomath.Blue
		return text.New(value, text.DefaultFont(), opt)
	}
	opt := table.DefaultOptions()
	opt.Text.Size = .42
	opt.PaddingX = .55
	opt.PaddingY = .26
	for _, value := range []string{"Vector", "Matrix", "Scalar"} {
		label, err := makeLabel(value)
		if err != nil {
			return nil, err
		}
		opt.RowLabels = append(opt.RowLabels, label)
	}
	for _, value := range []string{"Shape", "Operation", "Result"} {
		label, err := makeLabel(value)
		if err != nil {
			return nil, err
		}
		opt.ColumnLabels = append(opt.ColumnLabels, label)
	}
	opt.Corner, err = makeLabel("Object")
	if err != nil {
		return nil, err
	}
	tab, err := table.New([][]string{{"3 x 1", "dot", "Scalar"}, {"3 x 3", "multiply", "Matrix"}, {"1 x 1", "scale", "Scalar"}}, opt)
	if err != nil {
		return nil, err
	}
	tab.MoveTo(geom.V(0, -.4))
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	if err = s.Play(.8, gomath.FadeIn(tab.Object)); err != nil {
		return nil, err
	}
	background, err := tab.CellBackground(0, 2)
	if err != nil {
		return nil, err
	}
	target := background.Copy().SetFill(gomath.Color{R: .1, G: .5, B: .8, A: .35})
	if err = s.Play(.7, gomath.Transform(background, target)); err != nil {
		return nil, err
	}
	label := tab.RowLabels()[0]
	if err = s.Play(.7, gomath.Transform(label, label.Copy().SetColor(gomath.Yellow))); err != nil {
		return nil, err
	}
	if err = s.Wait(.8); err != nil {
		return nil, err
	}
	return s, nil
}
