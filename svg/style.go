package svg

import (
	"fmt"
	"gomath"
	"gomath/path"
	"strconv"
	"strings"
)

func initialSVGStyle() inheritedStyle {
	return inheritedStyle{
		fill: gomath.Black, stroke: gomath.Transparent, color: gomath.Black,
		fillOpacity: 1, strokeOpacity: 1, opacity: 1, width: 1,
		cap: gomath.CapButt, overflow: "visible",
		fontFamily: "Go", fontSize: 16, textAnchor: "start", clipRule: "nonzero",
		stopColor: gomath.Black, stopOpacity: 1, colorInterpolation: "sRGB",
	}
}

func initialCSSValue(key string) string {
	switch key {
	case "font-family":
		return "Go"
	case "font-size":
		return "16"
	case "font-weight", "font-style":
		return "normal"
	case "text-anchor":
		return "start"
	case "color-interpolation":
		return "sRGB"
	case "text-rendering":
		return "auto"
	case "fill", "color", "stop-color":
		return "black"
	case "stroke", "stroke-dasharray", "clip-path", "mask", "filter", "vector-effect":
		return "none"
	case "opacity", "fill-opacity", "stroke-opacity", "stroke-width", "stop-opacity":
		return "1"
	case "stroke-dashoffset":
		return "0"
	case "stroke-linecap":
		return "butt"
	case "stroke-linejoin":
		return "miter"
	case "stroke-miterlimit":
		return "4"
	case "fill-rule", "clip-rule":
		return "nonzero"
	case "display":
		return "inline"
	case "visibility", "overflow":
		return "visible"
	}
	return ""
}

func cssInherited(key string) bool {
	switch key {
	case "opacity", "display", "overflow", "clip-path", "mask", "filter", "vector-effect", "stop-color", "stop-opacity":
		return false
	}
	return true
}

// Resolve CSS-wide keywords without serializing inherited colors or lengths:
// inherited values retain their original precision. Unknown presentation
// attributes retain the pre-existing SVG checks (e.g. unsupported stroke-*).
func styleProperties(parent inheritedStyle, attrs map[string]string) map[string]string {
	properties := make(map[string]string, len(attrs))
	for key, value := range attrs {
		if cssProperty(key) {
			value = trimCSS(value)
			switch strings.ToLower(value) {
			case "inherit":
				if cssInherited(key) {
					continue
				}
				switch key {
				case "stop-color":
					continue // parseStyle retains the exact parent color for explicit inherit
				case "stop-opacity":
					value = strconv.FormatFloat(parent.stopOpacity, 'g', -1, 64)
				case "opacity":
					value = strconv.FormatFloat(parent.opacity, 'g', -1, 64)
				case "display":
					value = "inline"
					if parent.displayNone {
						value = "none"
					}
				case "clip-path":
					value = "none"
					if parent.clipPath != "" {
						value = "url(#" + parent.clipPath + ")"
					}
				case "overflow":
					value = parent.overflow
				default:
					value = initialCSSValue(key)
				}
			case "initial":
				value = initialCSSValue(key)
			case "unset":
				if cssInherited(key) {
					continue
				}
				value = initialCSSValue(key)
			}
		}
		properties[key] = value
	}
	return properties
}

// parseStyle evaluates already-cascaded specified declarations. Only the SVG
// importer resolves inheritance before assigning native stroke styles.
func parseStyle(base inheritedStyle, attrs map[string]string) (inheritedStyle, float64, error) {
	properties := styleProperties(base, attrs)
	if !strings.EqualFold(trimCSS(attrs["stop-color"]), "inherit") {
		base.stopColor, base.stopCurrent = gomath.Black, false
	}
	base.stopOpacity = 1
	if value, ok := properties["color-interpolation"]; ok {
		if !strings.EqualFold(value, "sRGB") {
			return base, 0, fmt.Errorf("SVG color-interpolation %q is not supported; requires sRGB", value)
		}
		base.colorInterpolation = "sRGB"
	}
	for key := range properties {
		if !cssProperty(key) && (strings.HasPrefix(key, "font-") || strings.HasPrefix(key, "text-") || key == "font" || key == "letter-spacing" || key == "word-spacing" || key == "direction" || key == "unicode-bidi" || key == "writing-mode" || key == "dominant-baseline" || key == "alignment-baseline" || key == "baseline-shift" || key == "white-space" || key == "kerning") {
			return base, 0, fmt.Errorf("unsupported SVG text property %q", key)
		}
	}
	var textErr error
	base, textErr = parseTextStyle(base, properties)
	if textErr != nil {
		return base, 0, textErr
	}
	base.displayNone, base.overflow, base.clipPath = false, "visible", ""
	opacity := 1.0
	if value, exists := properties["color"]; exists {
		if strings.EqualFold(value, "currentcolor") {
			// color:currentColor means the inherited color property.
		} else {
			c, err := paint(value)
			if err != nil || strings.EqualFold(value, "none") {
				return base, 0, fmt.Errorf("unsupported SVG color %q", value)
			}
			base.color = c
		}
	}
	for _, key := range []string{"fill", "stroke"} {
		if value, ok := properties[key]; ok {
			var c gomath.Color
			var err error
			current := strings.EqualFold(value, "currentcolor")
			if key == "fill" && strings.HasPrefix(strings.ToLower(value), "url(") {
				id, refErr := gradientReference(value)
				if refErr != nil {
					return base, 0, refErr
				}
				base.fillServer, base.fillCurrent, base.fill = id, false, gomath.Black
				continue
			}
			if current {
				c = base.color
			} else {
				c, err = paint(value)
			}
			if err != nil {
				return base, 0, err
			}
			if key == "fill" {
				base.fill, base.fillCurrent, base.fillServer = c, current, ""
			} else {
				base.stroke, base.strokeCurrent = c, current
			}
		}
	}
	if base.fillCurrent {
		base.fill = base.color
	}
	if base.strokeCurrent {
		base.stroke = base.color
	}
	if value, ok := properties["stop-color"]; ok {
		base.stopCurrent = strings.EqualFold(value, "currentcolor")
		if base.stopCurrent {
			base.stopColor = base.color
		} else {
			c, err := paint(value)
			if err != nil || strings.EqualFold(value, "none") {
				return base, 0, fmt.Errorf("unsupported SVG stop-color %q", value)
			}
			base.stopColor = c
		}
	}
	if base.stopCurrent {
		base.stopColor = base.color
	}
	for _, key := range []string{"opacity", "fill-opacity", "stroke-opacity", "stop-opacity"} {
		if value, ok := properties[key]; ok {
			v, err := strconv.ParseFloat(value, 64)
			if err != nil || !valid(v) || v < 0 || v > 1 {
				return base, 0, fmt.Errorf("invalid SVG %s", key)
			}
			switch key {
			case "opacity":
				opacity = v
			case "fill-opacity":
				base.fillOpacity = v
			case "stroke-opacity":
				base.strokeOpacity = v
			case "stop-opacity":
				base.stopOpacity = v
			}
		}
	}
	if _, ok := properties["stroke-width"]; ok {
		v, err := length(properties, "stroke-width", 1)
		if err != nil || v < 0 {
			return base, 0, fmt.Errorf("invalid SVG stroke width")
		}
		base.width = v
	}
	for _, key := range []string{"stroke-linecap", "stroke-linejoin", "stroke-miterlimit"} {
		value, ok := properties[key]
		if !ok {
			continue
		}
		switch key {
		case "stroke-linecap":
			switch strings.ToLower(value) {
			case "butt":
				base.cap = gomath.CapButt
			case "round":
				base.cap = gomath.CapRound
			case "square":
				base.cap = gomath.CapSquare
			default:
				return base, 0, fmt.Errorf("unsupported SVG stroke-linecap %q", value)
			}
		case "stroke-linejoin":
			switch strings.ToLower(value) {
			case "miter":
				base.join = gomath.JoinMiter
			case "round":
				base.join = gomath.JoinRound
			case "bevel":
				base.join = gomath.JoinBevel
			default:
				return base, 0, fmt.Errorf("unsupported SVG stroke-linejoin %q", value)
			}
		case "stroke-miterlimit":
			limit, err := strconv.ParseFloat(value, 64)
			if err != nil || !valid(limit) || limit < 1 {
				return base, 0, fmt.Errorf("SVG stroke-miterlimit must be finite and at least 1")
			}
			base.miterLimit = limit
		}
	}
	for key, value := range properties {
		if !strings.HasPrefix(key, "stroke-") {
			continue
		}
		switch key {
		case "stroke-width", "stroke-opacity", "stroke-linecap", "stroke-linejoin", "stroke-miterlimit":
		case "stroke-dasharray":
			pattern, err := parseDashArray(value)
			if err != nil {
				return base, 0, err
			}
			base.dash = pattern
		case "stroke-dashoffset":
			v, err := length(properties, key, 0)
			if err != nil {
				return base, 0, fmt.Errorf("invalid SVG stroke-dashoffset: %w", err)
			}
			base.dashOffset = v
		default:
			return base, 0, fmt.Errorf("unsupported SVG stroke property %q", key)
		}
	}
	if value, exists := properties["display"]; exists {
		switch strings.ToLower(value) {
		case "none":
			base.displayNone = true
		case "inline":
		default:
			return base, 0, fmt.Errorf("unsupported SVG display %q", value)
		}
	}
	if value, exists := properties["visibility"]; exists {
		switch strings.ToLower(value) {
		case "hidden", "collapse":
			base.visibilityHidden = true
		case "visible":
			base.visibilityHidden = false
		default:
			return base, 0, fmt.Errorf("unsupported SVG visibility %q", value)
		}
	}
	if value, exists := properties["overflow"]; exists {
		switch strings.ToLower(value) {
		case "visible", "hidden", "auto", "scroll":
			base.overflow = strings.ToLower(value)
		default:
			return base, 0, fmt.Errorf("unsupported SVG overflow %q", value)
		}
	}
	if value, exists := properties["clip-path"]; exists {
		id, err := clipReference(value)
		if err != nil {
			return base, 0, err
		}
		base.clipPath = id
	}
	if value, exists := properties["clip-rule"]; exists {
		base.clipRule = strings.ToLower(value)
		if base.clipRule != "nonzero" && base.clipRule != "evenodd" {
			return base, 0, fmt.Errorf("unsupported SVG clip-rule %q", value)
		}
	}
	for _, key := range []string{"mask", "filter", "vector-effect"} {
		if value, exists := properties[key]; exists && !strings.EqualFold(value, "none") {
			return base, 0, fmt.Errorf("SVG %s is not yet supported", key)
		}
	}
	if value, exists := properties["fill-rule"]; exists {
		switch strings.ToLower(value) {
		case "nonzero":
			base.fillRule = path.NonZero
		case "evenodd":
			base.fillRule = path.EvenOdd
		default:
			return base, 0, fmt.Errorf("unsupported SVG fill-rule %q", value)
		}
	}
	base.opacity = opacity
	return base, opacity, nil
}

// Dash lengths support SVG user units and existing absolute units. Percentages
// (viewport-diagonal semantics) and pathLength calibration are explicit errors.
func parseDashArray(value string) (*gomath.DashPattern, error) {
	value = trimCSS(value)
	if strings.EqualFold(value, "none") {
		return nil, nil
	}
	var values []float64
	for value != "" {
		end := 0
		for end < len(value) && !cssSpace(value[end]) && value[end] != ',' {
			end++
		}
		if end == 0 {
			return nil, fmt.Errorf("invalid SVG stroke-dasharray separator")
		}
		v, err := length(map[string]string{"dash": value[:end]}, "dash", 0)
		if err != nil {
			return nil, fmt.Errorf("invalid SVG stroke-dasharray: %w", err)
		}
		values = append(values, v)
		if len(values) > gomath.MaxDashEntries {
			return nil, fmt.Errorf("SVG stroke-dasharray exceeds %d entries", gomath.MaxDashEntries)
		}
		value = trimCSS(value[end:])
		if strings.HasPrefix(value, ",") {
			value = trimCSS(value[1:])
			if value == "" || value[0] == ',' {
				return nil, fmt.Errorf("invalid SVG stroke-dasharray separator")
			}
		}
	}
	return gomath.NewDashPattern(values, 0)
}
