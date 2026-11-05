# Object annotation geometry

The checked root constructors return independently owned ordinary objects:

```go
box, err := gomath.NewSurroundingRectangle(target,
    gomath.SurroundingRectangleOptions{PaddingX:.2, PaddingY:.1, CornerRadius:.08})
background, err := gomath.NewBackgroundRectangle(target,
    gomath.DefaultBackgroundRectangleOptions())
underline, err := gomath.NewUnderline(target, gomath.DefaultUnderlineOptions())
cross, err := gomath.NewCross(target, gomath.DefaultCrossOptions())
```

Handle each returned error before using its object. Construction reads the target
once, without mutating it or keeping a target reference. Finish source authoring
before construction; later source edits cannot update the annotation. Group the
source and annotation to transform them together, or explicitly reconstruct an
annotation through the existing redraw interfaces. Independent constructors may
read a completed target concurrently; returned mutable objects need ordinary
authoring synchronization.

## Bounds, geometry and style

All four use the complete target tree's geometric world-space XY bounds. Cubic
extrema come from Path.Bounds, rather than the control hull. Stroke width, clip,
fill, opacity, visibility and partial Reveal do not shrink or expand those
bounds. Bitmap plane paths participate like other paths. This is geometry, not
an alpha-mask or visible-ink measurement. It is also not a font advance/line box.

Annotations lie in the XY plane at the target bounds' center Z. A target with
varying Z gets an XY enclosure, without adding depth or 3D occlusion semantics.
An already rotated/sheared target gets its world-axis-aligned enclosure. Group
transformations subsequently transform annotation geometry normally.

| Constructor | Geometry and initial style |
| --- | --- |
| NewSurroundingRectangle | Width = target width + 2×PaddingX, height = target height + 2×PaddingY; centered on target bounds. RoundedRectangle's circular cubic corners; radius zero gives a sharp rectangle. Transparent fill, pure yellow stroke, ordinary default stroke width. |
| NewBackgroundRectangle | Same box geometry with explicit Fill and transparent zero-width stroke. Default options select black alpha .75, zero padding/radius. The zero options' transparent fill is meaningful. |
| NewUnderline | Straight left-to-right line across target minX..maxX at target minY−Buffer. Default buffer .1; zero buffer is meaningful. Ordinary white/default-width stroke. |
| NewCross | Two diagonal line children fitted to width/height, scaled about target center by explicit positive Scale. Default options choose 1; zero Scale rejects. First runs upper-left to lower-right, second upper-right to lower-left. Initial red stroke width .06. |

Every annotation copies target.ZIndex once. Add a background before its target
at that ZIndex, or explicitly choose another layer. Nothing edits scene order.
Styling uses normal Object methods: later background SetFill/SetStroke calls are
not forced back to black. Normal Create uses Reveal, rather than Manim's special
BackgroundRectangle.pointwise_become_partial fill-opacity behavior. There is no
scene-background-color lookup, automatic attachment or special lifecycle.

Padding and buffer are finite, nonnegative and at most1e12. Derived box/cross
width and height must be finite in [1e-9,1e12]; Underline needs only positive
width. Corner radius follows RoundedRectangle's [0,min(width,height)/2] range.
Cross Scale must lie in [1e-9,1e12], with the same derived dimension checks.
Points, control coordinates and translated outputs must remain within ±1e12.
Collapsed geometry after floating-point translation rejects. Normal binary64
rounding remains; no exact real-number or rational-conic guarantee is made.

A point can receive a positive-padding box. An empty tree has no bounds and
rejects. Zero-area targets cannot produce a cross or a zero-padding box;
vertical-only targets cannot produce an underline. Invalid options, malformed
geometry/styles, nil/shared/cyclic nodes, duplicate identities or exceeded
limits return nil/error without changing the target.

## Work and ownership limits

A nonrecursive preflight runs before recursive validation/bounds, with shared
aggregate limits across the entire tree:

- MaxAnnotationObjects:4096, including empty groups and leaves.
- MaxAnnotationDepth:64, with root depth1.
- MaxAnnotationContours:16384, including zero-curve contours.
- MaxAnnotationCurves:65536, including hidden/fully clipped geometry.

Children are counted before expanding traversal storage. Every authored control
is checked; then existing complete object validation and geometric bounds run
within these caps. Existing bitmap/clip/gradient validation has its own limits.
No callbacks, flattening, clipping, tessellation or output proportional to the
source tree is performed. Geometry work is linear in the bounded source tree
and curves, with O(nodes+depth) traversal/identity scratch. These are algorithm
bounds, not process RSS or wall-clock guarantees. Native rounded output has
four corner cubics and straight edge cubics; other output sizes are constant.

## Reference and validation

tests/reference/annotations.py pins Manim CE0.22.1 commit
16ea443e2a5f1d79db0a721161411ce916d6e733. Forty-eight constructions cover
straight-path targets, rotated polygons, multi-object groups, distinct padding,
buffers and cross scales. Seven linear Transform times per construction give
336 states, including interiors. Complete cubic-control **multisets** agree
within2e-11, disregarding rectangle contour starting corner/winding.

Those references do not verify curved-target control-hull bounds, rounded
defaults, raster/style equality or special background creation/lifecycle.
Separate analytical tests check actual cubic extrema, ignored stroke/clip,
center Z, background order policy, independent source/output geometry, stable
cross child references through Transform and independent sampled frames.
Inclusive node/contour/curve quotas and malformed/degenerate/range cases are
tested. All test sources, data and development generators live under tests/.

Constructor-only benchmark on Apple M2, Go1.27.3 darwin/arm64, CGO0/noasm,
GOMAXPROCS1:2.182–2.429µs/op,2352B/op,12allocs/op. Target is a prebuilt group
with a rectangle and shifted circle; options are prebuilt. Excludes target
construction, animation, rendering, I/O and independent references. Shared-host
results are observations, not a Manim or whole-library speed comparison. Raw
results are tests/data/root/annotations-benchmark-20261105.txt.

The `annotations` CLI example combines real math glyphs with rounded surrounding
geometry, an explicit blue background/underline and a false-statement cross.

## Independent elbow primitive

`Elbow(width,angle)` constructs the two open sides
`(0,width) -> (width,width) -> (width,0)`, rotated counterclockwise about world
zero. Width is a side length, not the rotated bounding-box extent. Width must
be finite in [1e-9,1e12], angle finite, and every resulting coordinate within
±1e12. It has ordinary default styling and no target reference. This standalone
marker is distinct from NewRightAngle's two-line intersection/corner contract.

`tests/reference/elbow.py` pins the same Manim version/commit:15 constructions
and105 linear Transform states agree at every ordered cubic control within2e-11.
Independent tests check side lengths/perpendicularity across tiny/large scales,
large finite angles and coordinate failures. Style/raster/lifecycle parity is
not inferred from those geometry samples.
