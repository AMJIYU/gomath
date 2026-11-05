package gomath

import "gomath/path"

// SetFillRule selects the vector fill rule for this node and descendants.
// Like SetFill, this is an authoring setter: invalid values remain observable
// and are rejected by Validate, scene preparation, rendering and export.
// It does not change clip rules, stroke outlines or bitmap sampling.
func (o *Object) SetFillRule(rule path.FillRule) *Object {
	o.Walk(func(v *Object) { v.Style.FillRule = rule })
	return o
}
