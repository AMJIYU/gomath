package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func Braces() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Braces / projected cubic bounds, segment normals and vector labels")
	if err != nil {
		return nil, err
	}
	heading.Scale(.84)
	formulaOptions := mathtex.DefaultOptions()
	formulaOptions.Size = .6
	formula, err := mathtex.New(`a^2+b^2=c^2`, formulaOptions)
	if err != nil {
		return nil, err
	}
	formula.MoveTo(geom.V(-3.5, .5))
	brace, err := gomath.NewBrace(formula, gomath.DefaultBraceOptions())
	if err != nil {
		return nil, err
	}
	brace.Object.SetColor(gomath.Blue)
	labelOptions := text.DefaultOptions()
	labelOptions.Size = .3
	label, err := text.New("sum of squares", text.DefaultFont(), labelOptions)
	if err != nil {
		return nil, err
	}
	placed, err := brace.LabelAtTip(label, .15)
	if err != nil {
		return nil, err
	}
	left := gomath.Group(formula, brace.Object, placed)
	start, end := geom.V(1.6, -.5), geom.V(5.2, 1.1)
	segmentBrace, err := gomath.NewBraceBetweenPoints(start, end, gomath.DefaultBraceBetweenPointsOptions())
	if err != nil {
		return nil, err
	}
	segmentBrace.Object.SetColor(gomath.Green)
	segmentLabel, err := text.New("distance", text.DefaultFont(), labelOptions)
	if err != nil {
		return nil, err
	}
	segmentPlaced, err := segmentBrace.LabelAtTip(segmentLabel, .15)
	if err != nil {
		return nil, err
	}
	right := gomath.Group(gomath.Line(start, end).SetStroke(gomath.Green, .04), segmentBrace.Object, segmentPlaced)
	caption, err := text.New("Independent label copies / group once to animate together", text.DefaultFont(), labelOptions)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.3))
	if err = scene.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = scene.Play(1.2, gomath.FadeIn(left), gomath.FadeIn(right)); err != nil {
		return nil, err
	}
	if err = scene.Play(1.8, gomath.Transform(left, left.Copy().Scale(1.08)), gomath.Transform(right, right.Copy().Rotate(.12))); err != nil {
		return nil, err
	}
	if err = scene.Wait(.5); err != nil {
		return nil, err
	}
	return scene, nil
}
