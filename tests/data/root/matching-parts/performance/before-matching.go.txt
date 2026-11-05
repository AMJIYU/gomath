package gomath

// Exact-key bucket matching follows the default policy of Manim Community's
// MIT-licensed animation/transform_matching_parts.py, commit
// 16ea443e2a5f1d79db0a721161411ce916d6e733. Upstream notices are in LICENSES/.

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxMatchingParts    = 256
	MaxMatchingKeyBytes = 65536
)

type matchingKeys struct {
	source, target       []string
	sourceIDs, targetIDs []uint64
}

// TransformMatchingParts matches the direct children of two plain groups by
// exact string keys. Repeated keys form ordered buckets; unequal bucket counts
// repeat existing children with transparent copies. Unmatched source buckets
// fade towards the unmatched target's geometric center; unmatched target
// buckets fade in at their own positions. At cleanup the actual target replaces
// source in its scene slot; the removed source remains unchanged and reusable.
//
// Keys and direct child identities are captured now, geometry at Play. No TeX
// parsing or geometric shape hashing is performed. Plain roots have no path,
// bitmap, clip or gradient and have opacity/reveal 1. Both trees are bounded.
// See docs/MATCHING_PARTS.md for alignment, painting and timing boundaries.
func TransformMatchingParts(source, target *Object, sourceKeys, targetKeys []string) (Animation, error) {
	k := &matchingKeys{}
	for side, keys := range [][]string{sourceKeys, targetKeys} {
		if len(keys) > MaxMatchingParts {
			return Animation{}, fmt.Errorf("matching requires at most %d direct parts per side", MaxMatchingParts)
		}
		copy := make([]string, len(keys))
		bytes := 0
		for i, key := range keys {
			if len(key) > MaxMatchingKeyBytes-bytes || !utf8.ValidString(key) {
				return Animation{}, fmt.Errorf("matching keys exceed byte budget or are invalid UTF-8")
			}
			bytes += len(key)
			copy[i] = strings.Clone(key)
		}
		if side == 0 {
			k.source = copy
		} else {
			k.target = copy
		}
	}
	if err := matchingInputs(source, target, k, false); err != nil {
		return Animation{}, err
	}
	for _, c := range source.Children {
		k.sourceIDs = append(k.sourceIDs, c.ID)
	}
	for _, c := range target.Children {
		k.targetIDs = append(k.targetIDs, c.ID)
	}
	a := ReplacementTransform(source, target)
	a.matching = k
	return a, nil
}

type matchingSize struct{ nodes, contours, curves int }

func (s *matchingSize) add(b matchingSize) error {
	s.nodes += b.nodes
	s.contours += b.contours
	s.curves += b.curves
	// Reserve two aligned trees before cloning a repeated family. Alignment
	// can materialize the union of both sides in each tree.
	if s.nodes > 8192 || s.contours > 16384 || s.curves > 65536 {
		return fmt.Errorf("matching exceeds aggregate node/contour/curve budget")
	}
	return nil
}

// Iterative inspection precedes recursive validation, snapshots and .Copy.
// A direct-child stack is checked before it can grow from caller input.
func matchingTree(root *Object) (matchingSize, error) {
	type item struct {
		o *Object
		d int
	}
	stack := []item{{root, 1}}
	seen := map[*Object]bool{}
	size := matchingSize{}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x.o == nil || seen[x.o] || x.d > 60 {
			return size, fmt.Errorf("matching has nil/shared/cyclic nodes or excessive depth")
		}
		seen[x.o] = true
		if err := size.add(matchingSize{nodes: 1, contours: len(x.o.Path.Contours)}); err != nil {
			return size, err
		}
		if len(x.o.Children) > 8192-size.nodes-len(stack) {
			return size, fmt.Errorf("matching exceeds pending-child budget")
		}
		for _, c := range x.o.Path.Contours {
			if err := size.add(matchingSize{curves: len(c.Curves)}); err != nil {
				return size, err
			}
			if !transformPathPointBound(c.Start) {
				return size, fmt.Errorf("matching coordinates exceed +/-1e9")
			}
			for _, q := range c.Curves {
				if !transformPathPointBound(q.P0) || !transformPathPointBound(q.P1) || !transformPathPointBound(q.P2) || !transformPathPointBound(q.P3) {
					return size, fmt.Errorf("matching controls exceed +/-1e9")
				}
			}
		}
		if x.o.Bitmap != nil || x.o.Clip != nil {
			return size, fmt.Errorf("matching currently requires unclipped vector parts")
		}
		for _, child := range x.o.Children {
			stack = append(stack, item{child, x.d + 1})
		}
	}
	return size, nil
}

func matchingInputs(source, target *Object, keys *matchingKeys, captured bool) error {
	if source == nil || target == nil || keys == nil {
		return fmt.Errorf("matching requires source, target and keys")
	}
	if len(source.Children) != len(keys.source) || len(target.Children) != len(keys.target) {
		return fmt.Errorf("matching key count differs from direct children")
	}
	combined := matchingSize{}
	for i, root := range []*Object{source, target} {
		if len(root.Path.Contours) != 0 || root.Style.Opacity != 1 || root.Style.Reveal != 1 || root.Style.FillGradient != nil {
			return fmt.Errorf("matching roots must be plain groups with opacity/reveal 1")
		}
		size, err := matchingTree(root)
		if err != nil {
			return err
		}
		if err := combined.add(size); err != nil {
			return err
		}
		if captured {
			ids := keys.sourceIDs
			if i == 1 {
				ids = keys.targetIDs
			}
			if len(ids) != len(root.Children) {
				return fmt.Errorf("matching captured child count changed")
			}
			for j, c := range root.Children {
				if c.ID != ids[j] {
					return fmt.Errorf("matching direct child identities/order changed")
				}
			}
		}
	}
	return validateObjects([]*Object{source, target})
}

// Each bucket reuses its first original and distributes transparent copies
// by the pinned floor(j*old/total) policy, not by appending the other side.
func repeatMatchingBucket(children []*Object, total int, budget *matchingSize) ([]*Object, error) {
	out := make([]*Object, 0, total)
	for i, child := range children {
		n := len(children)
		parts := ((i+1)*total+n-1)/n - (i*total+n-1)/n
		size, err := matchingTree(child)
		if err != nil {
			return nil, err
		}
		if err := budget.add(matchingSize{nodes: size.nodes * parts, contours: size.contours * parts, curves: size.curves * parts}); err != nil {
			return nil, err
		}
		out = append(out, child)
		for j := 1; j < parts; j++ {
			out = append(out, child.Copy().SetOpacity(0))
		}
	}
	return out, nil
}

func prepareMatching(a Animation) (*Object, *Object, error) {
	if err := matchingInputs(a.object, a.target, a.matching, true); err != nil {
		return nil, nil, err
	}
	f, t := a.object.snapshot(), a.target.snapshot()
	type bucket struct{ from, to []*Object }
	buckets := map[string]*bucket{}
	var order []string
	for side, keys := range [][]string{a.matching.source, a.matching.target} {
		root := f
		if side == 1 {
			root = t
		}
		for i, key := range keys {
			if buckets[key] == nil {
				buckets[key] = new(bucket)
				order = append(order, key)
			}
			b := buckets[key]
			if side == 0 {
				b.from = append(b.from, root.Children[i])
			} else {
				b.to = append(b.to, root.Children[i])
			}
		}
	}
	f.Children, t.Children = nil, nil
	unmatchedFrom, unmatchedTo := Group(), Group()
	budget := matchingSize{nodes: 4 + 2*len(order)}
	for _, key := range order {
		b := buckets[key]
		if len(b.from) == 0 {
			unmatchedTo.Children = append(unmatchedTo.Children, b.to...)
			continue
		}
		if len(b.to) == 0 {
			unmatchedFrom.Children = append(unmatchedFrom.Children, b.from...)
			continue
		}
		n := max(len(b.from), len(b.to))
		x, err := repeatMatchingBucket(b.from, n, &budget)
		if err != nil {
			return nil, nil, err
		}
		y, err := repeatMatchingBucket(b.to, n, &budget)
		if err != nil {
			return nil, nil, err
		}
		f.Children = append(f.Children, Group(x...))
		t.Children = append(t.Children, Group(y...))
	}
	for _, root := range []*Object{unmatchedFrom, unmatchedTo} {
		size, err := matchingTree(root)
		if err != nil {
			return nil, nil, err
		}
		if err := budget.add(matchingSize{2 * size.nodes, 2 * size.contours, 2 * size.curves}); err != nil {
			return nil, nil, err
		}
	}
	if len(unmatchedFrom.Children) > 0 {
		out := unmatchedFrom.snapshot().Shift(unmatchedTo.Center().Sub(unmatchedFrom.Center())).SetOpacity(0)
		f.Children = append(f.Children, unmatchedFrom)
		t.Children = append(t.Children, out)
	}
	if len(unmatchedTo.Children) > 0 {
		in := unmatchedTo.Copy().SetOpacity(0)
		f.Children = append(f.Children, in)
		t.Children = append(t.Children, unmatchedTo)
	}
	// A bounded dry traversal accounts for longest-curve subdivision work
	// before path.Align allocates; this applies even on straight trajectories.
	if err := validateTransformArcInputs(f, t); err != nil {
		return nil, nil, err
	}
	if err := validateBitmapMorph(f, t); err != nil {
		return nil, nil, err
	}
	return f, t, nil
}
