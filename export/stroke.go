package export

import (
	"fmt"
	"gomath"
	"strconv"
	"strings"
)

// StrokeAttributes returns validated SVG cap/join/miter/dash attributes, with no
// leading or trailing whitespace. Stroke color, width and opacity are emitted
// by the caller. The zero miter limit is written as its effective value, 4.
func StrokeAttributes(style gomath.Style) (string, error) {
	if err := style.Validate(); err != nil {
		return "", fmt.Errorf("SVG stroke style: %w", err)
	}
	caps := [...]string{"round", "butt", "square"}
	joins := [...]string{"miter", "round", "bevel"}
	attrs := `stroke-linecap="` + caps[style.StrokeCap] +
		`" stroke-linejoin="` + joins[style.StrokeJoin] +
		`" stroke-miterlimit="` + strconv.FormatFloat(style.EffectiveMiterLimit(), 'g', -1, 64) + `"`
	if style.Dash != nil {
		var lengths []string
		for _, v := range style.Dash.Lengths() {
			lengths = append(lengths, strconv.FormatFloat(v, 'g', -1, 64))
		}
		attrs += ` stroke-dasharray="` + strings.Join(lengths, " ") + `" stroke-dashoffset="` + strconv.FormatFloat(style.Dash.Offset(), 'g', -1, 64) + `"`
	} else {
		attrs += ` stroke-dasharray="none" stroke-dashoffset="0"`
	}
	return attrs, nil
}
