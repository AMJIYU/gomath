# Object family curve editing

`Object.SetAnchorMode(path.AnchorSmooth)` / `MakeSmooth()` and
`SetAnchorMode(path.AnchorJagged)` / `MakeJagged()` apply the checked
[path handle operations](CURVE_EDIT.md) to a bounded vector object family.
`PrepareForNonlinearTransform(minimum)` raises each nonempty node's complete
path to at least `minimum` explicit cubics before `ApplyPointwise` or
`ApplyComplex`. These authoring operations return an error and commit all paths
only after the entire family succeeds. Object IDs, pointers, child references,
styles and original anchors remain stable.

Preparation uses the pinned Manim global equal-parameter curve allocation:
original cubic `i` of `n` receives
`ceil((i+1)*total/n)-ceil(i*total/n)` pieces. Allocation runs across all stored
contours of each node, rather than independently rounding each contour. Every
original cubic is restricted exactly up to floating-point rounding; no curve
is removed. Each node receives its own minimum, subject to the family budget.
An isolated point contour can expand to constant cubics; several point-only
contours with a positive requested count reject as ambiguous. Trailing points
mixed with actual curves remain points. Implicit closing edges are preserved
and not counted during preparation. This is uniform parameter subdivision,
not adaptive approximation with a nonlinear error bound.

All stored joins must be exact. Handle conversion rejects `Closed` contours
whose last explicit endpoint differs from their start. In particular, the
existing `Circle` constructor can retain a tiny trigonometric endpoint gap:
such a circle rejects `MakeSmooth`/`MakeJagged`. This slice does not alter its
geometry or silently snap the seam. Use an explicitly authored exact closing
cubic when handle conversion is required. Open approximately closed input
retains the path solver's documented tolerance behavior. Subdivision accepts
the circle's implicit closing edge.

Bitmap nodes, invalid/cyclic/shared object trees, empty families without any
geometric bounds, nonfinite or out-of-range coordinates, and over-budget
outputs reject before any authored path is replaced. Coordinates are bounded
by ±1e9, each path by 4096 contours, and the family by the annotation tree
limits and 65536 explicit output cubics. The additional conservative budget
is 1048576 work units and 12582912 accounted bytes across the family. These
limits include borrowed input/output geometry and solver scratch; they are
not a process RSS bound. See `object_curve_edit.go` and CURVE_EDIT.md for the
linear accounting formulas.

The development generator `tests/reference/object_curve_edit.py` pins Manim
CE 0.22.1 at commit `16ea443e2a5f1d79db0a721161411ce916d6e733` and records
12 constructions plus 84 ordered control-point Transform states, including
animation interiors, multiple contours and nested family edits. The Go test
matches these controls and independently checks identity, atomic failure,
limits, unsupported closure and concurrent independent copies. It does not
claim general Mobject alignment, arbitrary smoothing semantics, or pixel
equivalence. All test sources and reference data remain in `tests/`.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -run '^TestObjectCurveEdit' -count=1 -vet=all
```
