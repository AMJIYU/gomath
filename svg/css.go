package svg

import (
	"fmt"
	"strings"
)

// This is a deliberately small SVG author-style grammar, not a CSS browser
// engine. Limits are per Read, shared by inline declarations and all sheets.
const (
	maxCSSBytes        = 256 * 1024
	maxCSSRules        = 512
	maxCSSSelectors    = 1024
	maxCSSDeclarations = 4096
	maxCSSParts        = 16
	maxCSSTerms        = 16
	maxCSSWork         = 2000000
)

type cssDeclaration struct{ property, value string }
type cssCompound struct {
	tag          string
	ids, classes []string
}
type cssPart struct {
	compound   cssCompound
	combinator byte // relation to previous part: ' ' or '>'
}
type cssSelector struct {
	parts       []cssPart
	specificity [3]int // ID, class, type counts; compared lexicographically
}
type cssRule struct {
	selector     cssSelector
	declarations []cssDeclaration
}
type cssCascade struct {
	rules                           []cssRule
	bytes, blocks, selectors, decls int
	work                            int
}

// Values stay in the existing SVG style evaluator. Unsupported visual features
// may specify their supported neutral value (e.g. filter:none), but never an
// effect that the renderer cannot represent.
func cssProperty(property string) bool {
	switch property {
	case "color", "stop-color", "stop-opacity", "color-interpolation", "fill", "stroke", "fill-opacity", "stroke-opacity", "opacity",
		"stroke-width", "stroke-linecap", "stroke-linejoin", "stroke-miterlimit",
		"display", "visibility", "overflow", "fill-rule", "clip-rule", "stroke-dasharray",
		"stroke-dashoffset", "clip-path", "mask", "filter", "vector-effect",
		"font-family", "font-size", "font-weight", "font-style", "text-anchor", "text-rendering":
		return true
	}
	return false
}

func (c *cssCascade) consume(n int) error {
	c.work += n
	if c.work > maxCSSWork {
		return fmt.Errorf("SVG CSS matching/cascade work exceeds %d steps", maxCSSWork)
	}
	return nil
}

func (c *cssCascade) text(value string) (string, error) {
	c.bytes += len(value)
	if c.bytes > maxCSSBytes {
		return "", fmt.Errorf("SVG CSS text exceeds %d bytes", maxCSSBytes)
	}
	var out strings.Builder
	for {
		start := strings.Index(value, "/*")
		if start < 0 {
			out.WriteString(value)
			return out.String(), nil
		}
		end := strings.Index(value[start+2:], "*/")
		if end < 0 {
			return "", fmt.Errorf("unclosed SVG CSS comment")
		}
		end += start + 4
		if start > 0 && end < len(value) && cssNameByte(value[start-1]) && cssNameByte(value[end]) {
			return "", fmt.Errorf("SVG CSS comments within identifiers are not supported")
		}
		out.WriteString(value[:start])
		value = value[end:]
	}
}

func (c *cssCascade) declarations(value string) ([]cssDeclaration, error) {
	var declarations []cssDeclaration
	for _, item := range strings.Split(value, ";") {
		if trimCSS(item) == "" {
			continue
		}
		property, value, ok := strings.Cut(item, ":")
		property, value = strings.ToLower(trimCSS(property)), trimCSS(value)
		if !ok || property == "" || value == "" {
			return nil, fmt.Errorf("invalid SVG CSS declaration %q", item)
		}
		if strings.Contains(value, "!") {
			return nil, fmt.Errorf("SVG CSS !important/priority syntax is not supported")
		}
		if !cssProperty(property) {
			return nil, fmt.Errorf("unsupported SVG CSS property %q", property)
		}
		// Validate even unmatched or overridden declarations, so an unsupported
		// feature cannot accidentally become a silent fallback.
		if _, _, err := parseStyle(initialSVGStyle(), map[string]string{property: value}); err != nil {
			return nil, fmt.Errorf("SVG CSS %s: %w", property, err)
		}
		c.decls++
		if c.decls > maxCSSDeclarations {
			return nil, fmt.Errorf("SVG CSS declaration count exceeds %d", maxCSSDeclarations)
		}
		declarations = append(declarations, cssDeclaration{property, value})
	}
	return declarations, nil
}

func (c *cssCascade) sheet(value string) error {
	text, err := c.text(value)
	if err != nil {
		return err
	}
	for text = trimCSS(text); text != ""; text = trimCSS(text) {
		if strings.HasPrefix(text, "@") {
			return fmt.Errorf("SVG CSS at-rules and external imports are not supported")
		}
		open := strings.IndexByte(text, '{')
		if open < 0 {
			return fmt.Errorf("SVG CSS rule requires a declaration block")
		}
		close := strings.IndexByte(text[open+1:], '}')
		if close < 0 {
			return fmt.Errorf("unclosed SVG CSS declaration block")
		}
		close += open + 1
		body := text[open+1 : close]
		if strings.ContainsAny(body, "{}") {
			return fmt.Errorf("SVG CSS nested rules are not supported")
		}
		c.blocks++
		if c.blocks > maxCSSRules {
			return fmt.Errorf("SVG CSS rule count exceeds %d", maxCSSRules)
		}
		declarations, err := c.declarations(body)
		if err != nil {
			return err
		}
		for _, part := range strings.Split(text[:open], ",") {
			selector, err := parseCSSSelector(trimCSS(part))
			if err != nil {
				return err
			}
			c.selectors++
			if c.selectors > maxCSSSelectors {
				return fmt.Errorf("SVG CSS selector count exceeds %d", maxCSSSelectors)
			}
			c.rules = append(c.rules, cssRule{selector, declarations})
		}
		text = text[close+1:]
	}
	return nil
}

func trimCSS(value string) string { return strings.Trim(value, " \t\n\r\f") }
func cssSpace(ch byte) bool       { return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' }
func cssNameStart(ch byte) bool   { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' }
func cssNameByte(ch byte) bool    { return cssNameStart(ch) || ch >= '0' && ch <= '9' || ch == '-' }

func cssIdentifier(value string, pos *int) (string, error) {
	start := *pos
	if *pos < len(value) && value[*pos] == '-' {
		*pos += 1
	}
	if *pos == len(value) || !cssNameStart(value[*pos]) {
		return "", fmt.Errorf("SVG CSS requires an unescaped ASCII identifier in %q", value)
	}
	for *pos < len(value) && cssNameByte(value[*pos]) {
		*pos += 1
	}
	return value[start:*pos], nil
}

func parseCSSSelector(value string) (cssSelector, error) {
	var selector cssSelector
	pos, relation := 0, byte(0)
	for pos < len(value) {
		var compound cssCompound
		terms := 0
		if value[pos] == '*' {
			pos++
			terms++
		} else if cssNameStart(value[pos]) || value[pos] == '-' {
			tag, err := cssIdentifier(value, &pos)
			if err != nil {
				return selector, err
			}
			compound.tag = tag
			selector.specificity[2]++
			terms++
		}
		for pos < len(value) && (value[pos] == '.' || value[pos] == '#') {
			kind := value[pos]
			pos++
			name, err := cssIdentifier(value, &pos)
			if err != nil {
				return selector, err
			}
			if kind == '#' {
				compound.ids = append(compound.ids, name)
				selector.specificity[0]++
			} else {
				compound.classes = append(compound.classes, name)
				selector.specificity[1]++
			}
			terms++
		}
		if terms == 0 || terms > maxCSSTerms {
			return selector, fmt.Errorf("SVG CSS compound selector requires 1..%d terms: %q", maxCSSTerms, value)
		}
		selector.parts = append(selector.parts, cssPart{compound, relation})
		if len(selector.parts) > maxCSSParts {
			return selector, fmt.Errorf("SVG CSS selector exceeds %d compound parts", maxCSSParts)
		}
		hadSpace := false
		for pos < len(value) && cssSpace(value[pos]) {
			hadSpace = true
			pos++
		}
		if pos == len(value) {
			break
		}
		if value[pos] == '>' {
			relation = '>'
			pos++
			for pos < len(value) && cssSpace(value[pos]) {
				pos++
			}
			if pos == len(value) {
				return selector, fmt.Errorf("incomplete SVG CSS child selector %q", value)
			}
		} else if hadSpace {
			relation = ' '
		} else {
			return selector, fmt.Errorf("unsupported SVG CSS selector syntax in %q", value)
		}
	}
	if len(selector.parts) == 0 {
		return selector, fmt.Errorf("empty SVG CSS selector")
	}
	return selector, nil
}

func (s cssCompound) matches(e *element) bool {
	if s.tag != "" && s.tag != e.name.Local {
		return false
	}
	for _, id := range s.ids {
		if e.attrs["id"] != id {
			return false
		}
	}
	for _, class := range s.classes {
		if !e.classes[class] {
			return false
		}
	}
	return true
}

func (c *cssCascade) matches(selector cssSelector, chain []*element) (bool, error) {
	if err := c.consume(1); err != nil {
		return false, err
	}
	if !selector.parts[len(selector.parts)-1].compound.matches(chain[len(chain)-1]) {
		return false, nil
	}
	if len(selector.parts) == 1 {
		return true, nil
	}
	// Dynamic programming over the bounded ancestor chain avoids exponential
	// backtracking for ambiguous descendant selectors such as "g g g ...".
	var previous, next [maxSVGDepth]bool
	for partIndex, part := range selector.parts {
		ancestorMatched := false
		for i, e := range chain {
			if err := c.consume(1); err != nil {
				return false, err
			}
			eligible := partIndex == 0
			if partIndex != 0 && i > 0 {
				ancestorMatched = ancestorMatched || previous[i-1]
				if part.combinator == '>' {
					eligible = previous[i-1]
				} else {
					eligible = ancestorMatched
				}
			}
			next[i] = eligible && part.compound.matches(e)
		}
		previous, next = next, previous
	}
	return previous[len(chain)-1], nil
}

func cssAtLeast(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func (c *cssCascade) attributes(e *element, chain []*element) (map[string]string, error) {
	if len(c.rules) == 0 && len(e.inline) == 0 && e.name.Local != "symbol" {
		return e.attrs, nil
	}
	attrs := make(map[string]string, len(e.attrs))
	for key, value := range e.attrs {
		if key != "style" {
			attrs[key] = value
		}
	}
	// The symbol UA default precedes presentation attributes and author CSS.
	if e.name.Local == "symbol" {
		if _, exists := attrs["overflow"]; !exists {
			attrs["overflow"] = "hidden"
		}
	}
	var winners map[string][3]int
	for _, rule := range c.rules {
		matched, err := c.matches(rule.selector, chain)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		if winners == nil {
			winners = make(map[string][3]int)
		}
		for _, declaration := range rule.declarations {
			if err := c.consume(1); err != nil {
				return nil, err
			}
			prior, exists := winners[declaration.property]
			if !exists || cssAtLeast(rule.selector.specificity, prior) {
				attrs[declaration.property] = declaration.value
				winners[declaration.property] = rule.selector.specificity
			}
		}
	}
	for _, declaration := range e.inline {
		if err := c.consume(1); err != nil {
			return nil, err
		}
		attrs[declaration.property] = declaration.value
	}
	return attrs, nil
}

func (doc *document) prepareCSS() error {
	for _, e := range doc.elements {
		if e.name.Space != "" && e.name.Space != svgNamespace {
			continue
		}
		if e.attrs["class"] != "" {
			e.classes = make(map[string]bool)
			for _, class := range strings.FieldsFunc(e.attrs["class"], func(r rune) bool { return r < 128 && cssSpace(byte(r)) }) {
				e.classes[class] = true
			}
		}
		if value, exists := e.attrs["style"]; exists {
			text, err := doc.css.text(value)
			if err != nil {
				return err
			}
			e.inline, err = doc.css.declarations(text)
			if err != nil {
				return err
			}
		}
		if e.name.Local != "style" || (e.name.Space != "" && e.name.Space != svgNamespace) {
			continue
		}
		if len(e.children) != 0 {
			return fmt.Errorf("SVG style element must contain CSS text only")
		}
		if mime := strings.ToLower(strings.TrimSpace(e.attrs["type"])); mime != "" && mime != "text/css" {
			return fmt.Errorf("unsupported SVG stylesheet type %q", mime)
		}
		if media := strings.ToLower(strings.TrimSpace(e.attrs["media"])); media != "" && media != "all" {
			return fmt.Errorf("SVG CSS media conditions are not supported")
		}
		if _, scoped := e.attrs["scoped"]; scoped {
			return fmt.Errorf("SVG CSS scoped stylesheets are not supported")
		}
		if err := doc.css.sheet(e.cssText.String()); err != nil {
			return err
		}
	}
	return nil
}
