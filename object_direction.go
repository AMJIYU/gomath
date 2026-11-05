package gomath

// Family versus receiver-only reversal follows Manim Community 0.22.1 (MIT).
// Upstream license notices: LICENSES/manim-LICENSE*.txt.

import (
	"fmt"
	"gomath/path"
)

// ReversePoints atomically reverses the stored vector paths of the complete
// bounded Object tree. Children keep their order, pointers, IDs and styles.
// Inputs use Path.Reversed's strict geometry/implicit-closure rules. Nil,
// repeated/cyclic nodes, duplicate/zero IDs and bitmap nodes reject the edit.
// Authoring must not overlap sampling or other writes. Atomicity means no
// paths are published on error; this is not a concurrent-write transaction.
func (o *Object) ReversePoints() error { return o.reversePathDirections(true) }

// ReverseDirection reverses only the receiver's path, matching VMobject's
// nonrecursive reverse_direction. The complete tree is still structurally
// checked and charged to a shared resource budget, but descendant geometry
// is neither edited nor validated. Use ReversePoints to reverse descendants.
func (o *Object) ReverseDirection() error { return o.reversePathDirections(false) }

func (o *Object) reversePathDirections(family bool) error {
	type visit struct {
		o     *Object
		depth int
	}
	stack := []visit{{o, 1}}
	nodes := make([]*Object, 0)
	seen := make(map[*Object]bool)
	ids := make(map[uint64]bool)
	contours, curves := 0, 0
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		v := current.o
		if v == nil || seen[v] {
			return fmt.Errorf("object direction: nil, shared or cyclic node")
		}
		if len(nodes) >= MaxAnnotationObjects || current.depth > MaxAnnotationDepth || len(v.Children) > MaxAnnotationObjects-len(nodes)-1-len(stack) {
			return fmt.Errorf("object direction: object/depth budget exceeded")
		}
		if v.ID == 0 || ids[v.ID] {
			return fmt.Errorf("object direction: zero or duplicate object ID")
		}
		seen[v], ids[v.ID] = true, true
		nodes = append(nodes, v)
		if len(v.Path.Contours) > path.MaxDirectionContours-contours {
			return fmt.Errorf("object direction: shared contour budget exceeded")
		}
		contours += len(v.Path.Contours)
		for _, c := range v.Path.Contours {
			if len(c.Curves) > path.MaxDirectionCurves-curves {
				return fmt.Errorf("object direction: shared curve budget exceeded")
			}
			curves += len(c.Curves)
		}
		for _, child := range v.Children {
			stack = append(stack, visit{child, current.depth + 1})
		}
	}
	// Count all stored paths, even for receiver-only edits. Conservatively
	// charge a duplicate of all geometry and 64 bytes/node of staging, so a
	// large family cannot multiply the per-path output allowance.
	if 12*curves+3*contours+2*len(nodes) > path.MaxDirectionWork || 192*curves+112*contours+64*len(nodes) > path.MaxDirectionStorage {
		return fmt.Errorf("object direction: shared work/storage budget exceeded")
	}
	if !family {
		nodes = nodes[:1]
	}
	changes := make([]path.Path, len(nodes))
	for i, v := range nodes {
		if v.Bitmap != nil {
			return fmt.Errorf("object direction: edited node %d is a bitmap", v.ID)
		}
		if err := v.ValidateVisual(); err != nil {
			return fmt.Errorf("object direction: node %d: %w", v.ID, err)
		}
		q, err := v.Path.Reversed()
		if err != nil {
			return fmt.Errorf("object direction: node %d: %w", v.ID, err)
		}
		changes[i] = q
	}
	for i, v := range nodes {
		v.Path = changes[i]
	}
	return nil
}
