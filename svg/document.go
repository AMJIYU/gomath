package svg

import (
	"encoding/xml"
	"fmt"
	"gomath"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	maxSVGDepth    = 128
	maxSVGElements = 100000
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
	xmlNamespace   = "http://www.w3.org/XML/1998/namespace"
)

// Pointers keep builders stationary while the mixed-content slice grows.
type textContent struct {
	value strings.Builder
	child *element
}

type element struct {
	name         xml.Name
	attrs        map[string]string
	children     []*element
	parent       *element
	foreignAttrs []xml.Name
	target       *element
	hasText      bool
	cssText      strings.Builder
	classes      map[string]bool
	inline       []cssDeclaration
	bitmap       *gomath.Bitmap
	content      []*textContent // ordered character data and tspan children
}

type document struct {
	root           *element
	ids            map[string]*element
	elements       []*element
	css            cssCascade
	clipPlans      map[*element][]*gomath.Clip
	clipBudget     clipBudget
	gradients      map[*element]*gradientPlan
	gradientBudget gradientBudget
}

// Index the entire document before expanding it: forward references and IDs
// inside nested definitions must work without instantiating those definitions.
func readDocument(reader io.Reader) (*document, error) {
	doc := &document{ids: make(map[string]*element)}
	// Bound XML token allocations too, before parsing a potentially huge href.
	input := &io.LimitedReader{R: reader, N: maxSVGInputBytes + 1}
	decoder := xml.NewDecoder(input)
	var stack []*element
	cssBytes := 0
	textBytes, textRunes := 0, 0
	for {
		token, err := decoder.Token()
		if input.N == 0 {
			return nil, fmt.Errorf("SVG input exceeds %d bytes", maxSVGInputBytes)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode SVG: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if len(stack) >= maxSVGDepth {
				return nil, fmt.Errorf("SVG XML depth exceeds %d", maxSVGDepth)
			}
			if len(doc.elements) >= maxSVGElements {
				return nil, fmt.Errorf("SVG XML element count exceeds %d", maxSVGElements)
			}
			e := &element{name: t.Name, attrs: make(map[string]string)}
			seen := make(map[xml.Name]bool)
			var xlink string
			var hasXlink bool
			for _, attr := range t.Attr {
				if seen[attr.Name] {
					return nil, fmt.Errorf("duplicate SVG attribute %s", attr.Name.Local)
				}
				seen[attr.Name] = true
				switch {
				case attr.Name.Space == "":
					e.attrs[attr.Name.Local] = attr.Value
				case attr.Name.Space == xlinkNamespace && attr.Name.Local == "href":
					xlink, hasXlink = attr.Value, true
				case attr.Name.Space == xmlNamespace && attr.Name.Local == "space":
					e.attrs["xml:space"] = attr.Value
				default:
					if attr.Name.Space != "xmlns" {
						e.foreignAttrs = append(e.foreignAttrs, attr.Name)
					}
					if textElement(e) && attr.Name.Space != "xmlns" {
						return nil, fmt.Errorf("unsupported SVG text attribute namespace %q", attr.Name.Space)
					}
				}
			}
			if _, hasHref := e.attrs["href"]; !hasHref && hasXlink {
				e.attrs["href"] = xlink
			}
			if len(stack) == 0 {
				if doc.root != nil {
					return nil, fmt.Errorf("multiple SVG root elements")
				}
				if e.name.Local != "svg" || (e.name.Space != "" && e.name.Space != svgNamespace) {
					return nil, fmt.Errorf("root element must be svg")
				}
				doc.root = e
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, e)
				e.parent = parent
				if textElement(parent) {
					parent.content = append(parent.content, &textContent{child: e})
				}
			}
			if id := e.attrs["id"]; id != "" {
				if _, exists := doc.ids[id]; exists {
					return nil, fmt.Errorf("duplicate SVG id %q", id)
				}
				doc.ids[id] = e
			}
			doc.elements = append(doc.elements, e)
			stack = append(stack, e)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				e := stack[len(stack)-1]
				if textElement(e) {
					textBytes += len(t)
					textRunes += utf8.RuneCount(t)
					if textBytes > maxTextBytes || textRunes > maxTextRunes {
						return nil, fmt.Errorf("SVG text source budget exceeds %d bytes or %d runes", maxTextBytes, maxTextRunes)
					}
					if len(e.content) == 0 || e.content[len(e.content)-1].child != nil {
						e.content = append(e.content, &textContent{})
					}
					e.content[len(e.content)-1].value.Write(t)
				}
				if e.name.Local == "style" && (e.name.Space == "" || e.name.Space == svgNamespace) {
					cssBytes += len(t)
					if cssBytes > maxCSSBytes {
						return nil, fmt.Errorf("SVG CSS text exceeds %d bytes", maxCSSBytes)
					}
					e.cssText.Write(t)
				}
			}
			if strings.TrimSpace(string(t)) != "" {
				if len(stack) == 0 {
					return nil, fmt.Errorf("text outside SVG root")
				}
				stack[len(stack)-1].hasText = true
			}
		case xml.ProcInst:
			if strings.EqualFold(t.Target, "xml-stylesheet") {
				return nil, fmt.Errorf("SVG external stylesheets are not supported")
			}
		}
	}
	if doc.root == nil {
		return nil, fmt.Errorf("empty SVG")
	}
	return doc, nil
}

func definitionOnly(e *element) bool {
	if e.name.Space != "" && e.name.Space != svgNamespace {
		return false
	}
	switch e.name.Local {
	case "defs", "symbol", "clipPath", "linearGradient", "radialGradient", "stop", "title", "desc", "metadata", "style":
		return true
	}
	return false
}

func (doc *document) resolveReferences() error {
	for _, e := range doc.elements {
		if e.name.Local != "use" {
			continue
		}
		href := strings.TrimSpace(e.attrs["href"])
		if !strings.HasPrefix(href, "#") || len(href) == 1 {
			return fmt.Errorf("SVG use requires a nonempty local fragment reference, got %q", href)
		}
		e.target = doc.ids[href[1:]]
		if e.target == nil {
			return fmt.Errorf("unknown SVG use reference %q", href)
		}
		switch e.target.name.Local {
		case "g", "symbol", "use", "image", "text", "path", "rect", "circle", "ellipse", "line", "polygon", "polyline":
		default:
			return fmt.Errorf("unsupported SVG use target <%s> for %q", e.target.name.Local, href)
		}
	}
	// Check even unused definitions. Memoized heights bound graph work to the
	// input size; expansion has a separate budget for exponential fan-out.
	state := make(map[*element]uint8)
	heights := make(map[*element]int)
	var visit func(*element, int) (int, error)
	visit = func(e *element, depth int) (int, error) {
		if state[e] == 1 {
			return 0, fmt.Errorf("cyclic SVG use reference at id %q", e.attrs["id"])
		}
		if depth > maxSVGDepth {
			return 0, fmt.Errorf("SVG reference depth exceeds %d", maxSVGDepth)
		}
		if state[e] == 2 {
			return heights[e], nil
		}
		state[e] = 1
		height := 1
		edges := e.children
		if e.target != nil {
			edges = []*element{e.target}
		}
		for _, child := range edges {
			if e.target == nil && definitionOnly(child) {
				continue
			}
			h, err := visit(child, depth+1)
			if err != nil {
				return 0, err
			}
			height = max(height, h+1)
		}
		if height > maxSVGDepth {
			return 0, fmt.Errorf("SVG reference depth exceeds %d", maxSVGDepth)
		}
		state[e], heights[e] = 2, height
		return height, nil
	}
	for _, e := range doc.elements {
		if _, err := visit(e, 1); err != nil {
			return err
		}
	}
	return nil
}
