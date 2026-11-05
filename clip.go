package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
)

const (
	MaxClipContours = 4096
	MaxClipCurves   = 65536
	// MaxClipDepth bounds intersecting clip ancestors for a single drawable.
	MaxClipDepth = 64
	// Alignment repeatedly splits the longest curve; bound its scan work before
	// allocating or iterating, including badly mismatched contour resolutions.
	MaxClipAlignmentWork = 16 << 20
)

// Clip owns an immutable vector clipping path and fill rule in world space.
// Contours are implicitly closed for clipping, including open source contours.
// An empty constructed Clip hides all content. A nil Clip imposes no clipping;
// the zero value is invalid. Copies and snapshots share immutable clip storage.
type Clip struct {
	outline path.Path
	rule    path.FillRule
	ready   bool
	err     error
}

// NewClip constructs a nonzero-winding clip, preserving the original default.
func NewClip(outline path.Path) (*Clip, error) {
	return NewClipWithRule(outline, path.NonZero)
}

// NewClipWithRule copies outline and captures NonZero or EvenOdd immutably.
// The rule applies to the entire compound outline, including self-intersections.
func NewClipWithRule(outline path.Path, rule path.FillRule) (*Clip, error) {
	if rule != path.NonZero && rule != path.EvenOdd {
		return nil, fmt.Errorf("invalid clip fill rule %d", rule)
	}
	if err := validateClipOutline(outline); err != nil {
		return nil, err
	}
	return &Clip{outline: outline.Clone(), rule: rule, ready: true}, nil
}

func validateClipOutline(outline path.Path) error {
	if len(outline.Contours) > MaxClipContours {
		return fmt.Errorf("clip exceeds %d contours", MaxClipContours)
	}
	curves := 0
	for _, contour := range outline.Contours {
		if len(contour.Curves) > MaxClipCurves-curves {
			return fmt.Errorf("clip exceeds %d cubic curves", MaxClipCurves)
		}
		curves += len(contour.Curves)
	}
	if err := outline.Validate(); err != nil {
		return fmt.Errorf("clip: %w", err)
	}
	return nil
}

func (c *Clip) Validate() error {
	if c == nil || !c.ready {
		return fmt.Errorf("uninitialized clip")
	}
	if c.rule != path.NonZero && c.rule != path.EvenOdd {
		return fmt.Errorf("invalid clip fill rule %d", c.rule)
	}
	return c.err
}

// FillRule reports the immutable compound-path rule. Nil reports NonZero, but
// remains absence of clipping rather than a constructed clip.
func (c *Clip) FillRule() path.FillRule {
	if c == nil {
		return path.NonZero
	}
	return c.rule
}

// Path returns an independent outline; changing it cannot alter the Clip.
func (c *Clip) Path() path.Path {
	if c == nil {
		return path.Path{}
	}
	return c.outline.Clone()
}

func (c *Clip) Bounds() geom.Bounds {
	if c == nil || !c.ready || c.err != nil {
		return geom.Bounds{}
	}
	return c.outline.Bounds()
}

// Transform returns an independently owned, validated affine transformation.
func (c *Clip) Transform(matrix geom.Mat4) (*Clip, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	for _, value := range matrix {
		if !finite(value) {
			return nil, fmt.Errorf("clip transform must be finite")
		}
	}
	if matrix[12] != 0 || matrix[13] != 0 || matrix[14] != 0 || matrix[15] != 1 {
		return nil, fmt.Errorf("clip transform must be affine")
	}
	q := c.transformed(matrix)
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return q, nil
}

// Object's established mutation API defers invalid geometry errors to scene or
// rendering validation. Preserve that contract for clips without hiding errors.
func (c *Clip) transformed(matrix geom.Mat4) *Clip {
	if err := c.Validate(); err != nil {
		return &Clip{rule: c.FillRule(), ready: true, err: err}
	}
	q := &Clip{outline: c.outline.Transform(matrix), rule: c.rule, ready: true}
	q.err = q.outline.Validate()
	return q
}

// SetClip installs a world-space clip on this node and its descendants. Child
// clips intersect with ancestor clips. Nil removes this node's own clip only.
// It does not change layout bounds, IDs, child pointers or authored geometry.
func (o *Object) SetClip(clip *Clip) *Object { o.Clip = clip; return o }

func alignClip(a, b *Object) error {
	if a.Clip == nil || b.Clip == nil || a.Clip == b.Clip {
		return nil
	}
	for _, clip := range []*Clip{a.Clip, b.Clip} {
		if err := clip.Validate(); err != nil {
			return err
		}
	}
	if a.Clip.rule != b.Clip.rule {
		return fmt.Errorf("Transform requires matching clip fill rules; use FadeTransform for independently clipped layers")
	}
	work := 0
	for i := range max(len(a.Clip.outline.Contours), len(b.Clip.outline.Contours)) {
		na, nb := 0, 0
		if i < len(a.Clip.outline.Contours) {
			na = len(a.Clip.outline.Contours[i].Curves)
		}
		if i < len(b.Clip.outline.Contours) {
			nb = len(b.Clip.outline.Contours[i].Curves)
		}
		n := max(na, nb)
		gap := n - min(na, nb)
		if n > MaxClipAlignmentWork-work || (n != 0 && gap > (MaxClipAlignmentWork-work-n)/n) {
			return fmt.Errorf("clip alignment exceeds bounded subdivision work")
		}
		work += n + gap*n
	}
	x, y := path.Align(a.Clip.outline, b.Clip.outline)
	if err := validateClipOutline(x); err != nil {
		return err
	}
	if err := validateClipOutline(y); err != nil {
		return err
	}
	a.Clip, b.Clip = &Clip{outline: x, rule: a.Clip.rule, ready: true}, &Clip{outline: y, rule: b.Clip.rule, ready: true}
	return nil
}

func interpolateClip(a, b *Clip, progress float64) *Clip {
	if a == b || a == nil || b == nil {
		return a // Presence changes are rejected while preparing the animation.
	}
	if progress == 0 {
		return a
	}
	if progress == 1 {
		return b
	}
	if a.rule != b.rule {
		return &Clip{rule: a.rule, ready: true, err: fmt.Errorf("cannot interpolate mismatched clip fill rules")}
	}
	out := &Clip{rule: a.rule, ready: true}
	interpolatePrivatePath(&out.outline, a.outline, b.outline, progress)
	// Most coordinates lie between prevalidated endpoints. Extrapolation needs
	// its own finite check, just as the object's animated path does.
	if progress < 0 || progress > 1 {
		out.err = out.outline.Validate()
	}
	return out
}
