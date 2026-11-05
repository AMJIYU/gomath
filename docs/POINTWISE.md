# Checked vector pointwise and complex maps

`object.ApplyPointwise(mapping, DefaultPointwiseOptions())` maps actual cubic
geometry during authoring. The callback has type
`func(geom.Vec3) (geom.Vec3,error)`. `ApplyComplex` accepts
`func(complex128) (complex128,error)`, maps XY and preserves each input Z.
Neither callback runs during frame sampling or rendering.

```go
target := object.Copy()
err := target.ApplyComplex(func(z complex128) (complex128, error) {
    return z*z, nil
}, gomath.DefaultPointwiseOptions())
if err != nil { return err }
return scene.Play(2, gomath.Transform(object, target))
```

Mapping is about the origin. Each cubic contracts handles toward anchors by
`HandleScale` (default `.01`), maps its four points, then expands the mapped
handle distances by the reciprocal. This matches Manim's default
`VMobject.apply_function` policy and approximates mapped tangents. An explicit
finite scale in `[1e-6,1]` is required;1 directly maps controls. There is no
automatic curve subdivision, symbolic derivative, global injectivity check or
error bound for the nonlinear image. Subdivide source geometry when the
function varies within an existing long cubic, as the `pointwise` demo does.

The operation preflights the entire source tree, stages independent paths,
invokes the callback synchronously and validates the complete result before
committing. Object/child pointers, IDs, order, path topology, colors, style,
ZIndex, names and Scalar remain. Previously captured Scene/Replay geometry is
independent. Ordinary Transform animates from source to the mapped target;
this is interpolation of resulting cubic controls, not continuous evaluation
of a changing mathematical function at each animation alpha.

Existing bitmap planes, clip paths and gradient frames only represent affine
geometry. Any such node anywhere in the source rejects before the first
callback, including an inherited group clip or gradient. No visible content
is silently discarded. Use ApplyMatrix for supported affine paint/image edits.

Limits reuse the checked annotation tree policy:4096 objects,depth64,
16384 contours,65536 cubics, valid independent ownership/IDs and source/result
control coordinates within +/-1e12. The target must contain geometric bounds;
an entirely empty group rejects. Restored handles, not merely callback outputs,
must fit the coordinate bound. Invalid options, nil functions, malformed trees,
unsupported visual nodes, callback errors/panics and invalid results leave
source geometry unchanged. Returned callback errors retain `errors.Is` identity.
Panic payloads are not formatted.

Calls follow preorder objects, contours and curves: P0, contracted P1,
contracted P2, P3, four per cubic; a contour with no curves calls its Start once.
Repeated anchors are not deduplicated. At most278528 callback invocations occur
under the shared limits. A callback must return and be deterministic; use pure
captured data. There is no timeout or recovery from Goexit/process termination,
and external callback side effects cannot be rolled back. Authoring must not
mutate the same object concurrently or through a callback closure.

The independent development fixture constructs24 cubic families directly in
pinned Manim CE0.22.1, covering affine/quadratic maps, complex square/exp,
`.01/.25/1` handle scales, nested groups and preserved Z. Seven linear Transform
samples per family add168 ordered full-control states, tolerance2e-11. Go tests
also check exact invocation order, metadata/pointer retention, late errors,
hostile/nil panics, nonfinite and expanded-handle overflow, preflight budgets,
and independent concurrent Scene snapshots. Existing references and thresholds
stay unchanged. All fixture scripts, inputs and test sources are under tests/.

This does not implement full `prepare_for_nonlinear_transform`, nonlinear paint
or image warping, all ApplyFunction/ApplyMethod authoring-builder semantics, or
general updater scheduling. No Python/native library/process is used by the Go
API or its ordinary tests; Python only generated independent development data.
