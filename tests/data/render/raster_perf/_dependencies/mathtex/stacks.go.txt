package mathtex

// Horizontal visible rules have zero path-bbox height, but their baseline box
// includes stroke thickness. Empty and advance-only boxes have no visible body.
func visibleStackBody(b box) bool {
	ink := b.object.Bounds()
	return ink.Valid && ink.Size().X > 0 && (ink.Size().Y > 0 || b.height+b.depth > 0)
}

func (p *parser) explicitStack(command string, size float64) (box, error) {
	// An annotation enters text style at script size. Its local declarations
	// are restored before the body; body parsing inherits the caller's mode.
	annotation, err := p.textStyleGroup(size * .65)
	if err != nil {
		return box{}, err
	}
	body, err := p.requiredGroup(size)
	if err != nil {
		return box{}, err
	}
	if !visibleStackBody(body) {
		return box{}, p.error(command + " requires a nonempty visible body")
	}
	var sup, sub *box
	if command == "underset" {
		sub = &annotation
	} else {
		sup = &annotation
	}
	b, err := p.operatorLimits(body, sup, sub, size)
	if err != nil {
		return box{}, err
	}
	// A decorated body becomes a compound ordinary atom, even if the body
	// contains an operator. Outer _/^ scripts therefore remain side scripts.
	b.object.Name = "Stack " + command
	return b, nil
}
