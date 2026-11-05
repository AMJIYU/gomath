package gomath

// ShowPassingFlash shows a moving parameter-space sliver of vector paths and
// removes the object at the Play endpoint. It introduces absent objects. Cleanup
// restores the author's full geometry for reuse. The default width is .1 and
// the default rate is Smooth. Groups still clean up together at Play's endpoint.
func ShowPassingFlash(object *Object) Animation { return PassingFlashWithWidth(object, .1) }

// PassingFlashWithWidth selects the sliver width relative to each object's
// explicit cubic parameter span. This follows ShowPartial parameter timing,
// not constant arc-length speed. Width must be finite and in (0,1e6].
func PassingFlashWithWidth(object *Object, timeWidth float64) Animation {
	return Animation{object: object, mode: passingFlash, rate: Smooth, end: 1, pose: &poseTrack{kind: passingFlash, width: timeWidth}}
}

// Uncreate progressively erases an existing vector object and removes it at
// the Play endpoint. Its authoring Reveal ends at zero; set Reveal back to 1
// before reusing the stored full path with Create. Unwrite is an alias.
func Uncreate(object *Object) Animation {
	return Animation{object: object, mode: uncreate, rate: Smooth, end: 1}
}
func Unwrite(object *Object) Animation { return Uncreate(object) }
