package mathtex

import (
	"gomath"
	"gomath/geom"
	"math"
)

// Forced commands restore nominal em inside scripts/annotations; unforced
// binom follows the current local size and display declaration. Legacy frac
// stays independent, preserving its exact pre-extension geometry.
func (p *parser) generalFraction(command string, size float64) (box, error) {
	em, display := size, p.display
	if p.fractionEm != 0 {
		em = p.fractionEm
	}
	switch command {
	case "dfrac", "dbinom":
		em, display = p.options.Size, true
	case "tfrac", "tbinom":
		em, display = p.options.Size, false
	}
	ratio, gap := .7, .14*em
	if display {
		ratio, gap = 1, .24*em
	}
	thickness := .045 * em
	if !finite(em) || em <= 0 || thickness == 0 || gap == 0 || em*ratio == 0 {
		return box{}, p.error("fraction dimensions underflow or overflow")
	}
	numerator, err := p.textStyleGroup(em * ratio)
	if err != nil {
		return box{}, err
	}
	denominator, err := p.textStyleGroup(em * ratio)
	if err != nil {
		return box{}, err
	}
	binomial := command == "binom" || command == "dbinom" || command == "tbinom"
	axis := .16 * em
	clearance := gap
	if !binomial {
		clearance += thickness / 2
	}
	width := math.Max(numerator.width, denominator.width) + em*.25
	ny := axis + clearance + numerator.depth
	dy := axis - clearance - denominator.height
	numerator.object.Shift(geom.V((width-numerator.width)/2, ny))
	denominator.object.Shift(geom.V((width-denominator.width)/2, dy))
	numerator.object.Name = "FractionNumerator"
	denominator.object.Name = "FractionDenominator"
	children := []*gomath.Object{numerator.object, denominator.object}
	height := math.Max(0, ny+numerator.height)
	depth := math.Max(0, -dy+denominator.depth)
	if !binomial {
		line := gomath.Line(geom.V(0, axis), geom.V(width, axis)).SetStroke(p.options.Color, thickness)
		line.Name = "FractionRule"
		if err := p.accentCurves(line); err != nil {
			return box{}, err
		}
		children = append(children, line)
		height = math.Max(height, axis+thickness/2)
	}
	b := box{object: gomath.Group(children...), width: width, height: height, depth: depth, content: true}
	b.object.Name = "Fraction " + command
	if !finite(width) || !finite(height) || !finite(depth) || width <= 0 {
		return box{}, p.error("fraction dimensions underflow or overflow")
	}
	if binomial {
		// Reuse real delimiter paths, never magnify ordinary '(' glyphs.
		b = p.enclose(b, "(", ")", em)
		b.object.Name = "Binomial " + command
		for _, delimiter := range []*gomath.Object{b.object.Children[0], b.object.Children[2]} {
			if err := p.accentCurves(delimiter); err != nil {
				return box{}, err
			}
		}
	}
	return b, nil
}
