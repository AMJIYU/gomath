package gomath

import "fmt"

type animationGroup struct {
	children []Animation
	lag      float64
	sequence bool
}

// AnimationGroup schedules children simultaneously. Groups can be nested and
// have their own rate, window and duration. As with Play, writable subtrees must
// be disjoint. This is not Succession's delayed initialization of shared objects.
func AnimationGroup(animations ...Animation) Animation {
	return LaggedGroup(0, animations...)
}

// LaggedGroup starts each child after lag times the preceding child's duration.
// The group span is the maximum child end, including when an earlier child is
// longer than the last. Invalid timing and empty groups are rejected by Play.
func LaggedGroup(lag float64, animations ...Animation) Animation {
	return Animation{group: &animationGroup{children: append([]Animation(nil), animations...), lag: lag}, rate: Linear, end: 1}
}

// WithDuration sets the child's relative runtime inside a group, in seconds.
// Ungrouped leaves default to 1; groups default to their natural child span.
// Play's duration rescales the complete schedule. Zero/negative/nonfinite values
// reject authoring. WithWindow is applied within the allocated child runtime.
func (a Animation) WithDuration(seconds float64) Animation {
	a.duration, a.hasDuration = seconds, true
	return a
}

type animationTiming struct {
	start, end float64
	rate       RateFunc
}

func (t animationTiming) progress(alpha float64) float64 {
	a := clamp((alpha - t.start) / (t.end - t.start))
	if t.rate != nil {
		a = t.rate(a)
	}
	return a
}

func validateAnimationTiming(a Animation) error {
	if !finite(a.start) || !finite(a.end) || a.start < 0 || a.end > 1 || a.start >= a.end {
		return fmt.Errorf("invalid animation window [%g,%g]", a.start, a.end)
	}
	if a.hasDuration && (!finite(a.duration) || a.duration <= 0) {
		return fmt.Errorf("animation duration must be finite and positive")
	}
	return nil
}

type compiledAnimation struct {
	animation      Animation
	duration, span float64
	children       []*compiledAnimation
}

// compileAnimations checks the shared complexity/timing budget before either
// the parallel clock compiler or the sequential transaction stages any objects.
func compileAnimations(input []Animation) ([]*compiledAnimation, error) {
	const maxNodes, maxDepth = 10000, 64
	count := 0
	var compile func(Animation, int) (*compiledAnimation, error)
	compile = func(a Animation, depth int) (*compiledAnimation, error) {
		count++
		if count > maxNodes || depth > maxDepth {
			return nil, fmt.Errorf("animation group complexity limit exceeded")
		}
		if err := validateAnimationTiming(a); err != nil {
			return nil, err
		}
		node := &compiledAnimation{animation: a, span: 1}
		if a.group != nil && a.transformPath != nil {
			return nil, fmt.Errorf("transform path options require a Transform leaf, not a group")
		}
		if a.group != nil {
			if len(a.group.children) == 0 {
				return nil, fmt.Errorf("animation group requires children")
			}
			if !finite(a.group.lag) || a.group.lag < 0 {
				return nil, fmt.Errorf("group lag must be finite and nonnegative")
			}
			start := 0.0
			node.span = 0
			for i, child := range a.group.children {
				c, err := compile(child, depth+1)
				if err != nil {
					return nil, err
				}
				node.children = append(node.children, c)
				node.span = max(node.span, start+c.duration)
				if i+1 < len(a.group.children) {
					start += a.group.lag * c.duration
				}
				if !finite(start) || !finite(node.span) {
					return nil, fmt.Errorf("group timing overflow")
				}
			}
		}
		node.duration = node.span
		if a.hasDuration {
			node.duration = a.duration
		}
		return node, nil
	}
	roots := make([]*compiledAnimation, 0, len(input))
	for _, a := range input {
		node, err := compile(a, 0)
		if err != nil {
			return nil, err
		}
		roots = append(roots, node)
	}
	return roots, nil
}

// flattenAnimations compiles a bounded hierarchy into independent progress
// functions. A group's rate warps its whole clock before resolving child windows.
func flattenAnimations(input []Animation) ([]Animation, error) {
	roots, err := compileAnimations(input)
	if err != nil {
		return nil, err
	}
	var out []Animation
	var emit func(*compiledAnimation, []animationTiming) error
	emit = func(node *compiledAnimation, ancestors []animationTiming) error {
		a := node.animation
		if a.group != nil && a.group.sequence {
			return fmt.Errorf("Succession must be the sole Play argument or a direct child of Succession")
		}
		if a.mode == copyMorph && a.group == nil {
			a.rate = copyMorphRate(a.rate)
		}
		clock := append(append([]animationTiming(nil), ancestors...), animationTiming{a.start, a.end, a.rate})
		if a.group == nil {
			leaf := a
			leaf.start, leaf.end = 0, 1
			leaf.rate = func(alpha float64) float64 {
				// AnimationGroup.finish finishes every leaf, even when a parent
				// has a returning or otherwise nonstandard final clock value.
				if alpha >= 1 {
					if a.rate == nil {
						return 1
					}
					return a.rate(1)
				}
				for _, step := range clock {
					alpha = step.progress(alpha)
					if !finite(alpha) {
						return alpha
					}
				}
				return alpha
			}
			out = append(out, leaf)
			return nil
		}
		start := 0.0
		for i, child := range node.children {
			lo, hi := start/node.span, (start+child.duration)/node.span
			if !finite(lo) || !finite(hi) || lo >= hi {
				return fmt.Errorf("unrepresentable group child window")
			}
			parent := append(append([]animationTiming(nil), clock...), animationTiming{lo, hi, nil})
			if err := emit(child, parent); err != nil {
				return err
			}
			if i+1 < len(node.children) {
				start += a.group.lag * child.duration
			}
		}
		return nil
	}
	for _, node := range roots {
		if node.animation.group == nil {
			a := node.animation
			if a.mode == copyMorph {
				a.rate = copyMorphRate(a.rate)
			}
			out = append(out, a)
			continue
		}
		if err := emit(node, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Source/target roles are swapped by Manim's implementation, and it evaluates
// rate(1-alpha). In our forward source-to-target representation this is the
// reflected rate 1-rate(1-alpha), applied inside the leaf's window/group clock.
func copyMorphRate(rate RateFunc) RateFunc {
	if rate == nil {
		rate = Linear
	}
	return func(alpha float64) float64 { return 1 - rate(1-alpha) }
}
