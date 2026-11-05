package mathtex

import "gomath"

// TransformMatchingParts matches explicit, separately laid-out fragment keys.
// It installs the actual target Parts root at the segment endpoint, preserving
// all of its live handles, and leaves the removed source Parts reusable.
// This does not infer fragments or semantic keys from a concatenated formula.
func TransformMatchingParts(source, target *Parts) (gomath.Animation, error) {
	if err := source.validate(); err != nil {
		return gomath.Animation{}, err
	}
	if err := target.validate(); err != nil {
		return gomath.Animation{}, err
	}
	return gomath.TransformMatchingParts(source.object, target.object, source.Keys(), target.Keys())
}
