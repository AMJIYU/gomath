# Native 2D gradient fills

GoMath supports immutable linear and concentric radial vector fills in pure Go.
This is a bounded paint implementation, not complete SVG/Manim gradient parity.
Rendering and SVG export require no system fonts, cgo, native libraries,
subprocesses or external renderer. The SVG importer is a separate integration.

## API and paint selection

```go
stops := []gomath.GradientStop{
    {Offset: 0, Color: gomath.Color{R: 1, A: 1}},
    {Offset: .4, Color: gomath.Color{G: 1, A: .7}},
    {Offset: 1, Color: gomath.Color{B: 1, A: .3}},
}
linear, err := gomath.NewLinearGradient(geom.V(-2, 0), geom.V(2, 0), stops)
if err != nil {
    return err
}
object.SetFillGradient(linear)

radial, err := gomath.NewRadialGradient(geom.V(0, 0), 2, stops)
if err != nil {
    return err
}
other.SetFillGradient(radial)
```

`GradientStop` contains `Offset float64` and `Color Color`. Constructors copy the
stop slice and validate all parameters. `Gradient` has private immutable storage;
its zero value is invalid. Snapshots and copies safely share paints. `Stops()`
returns an independent slice, `Matrix()` returns an affine matrix by value, and
`Kind()` returns `LinearGradient` or `RadialGradient`. `ColorAt(geom.Vec3)` evaluates
the field analytically and returns an error for invalid/out-of-range points.
`Validate()` accepts a nil paint, as required by the optional style field;
`ColorAt` on nil returns an error.

`Style.FillGradient *Gradient` replaces `Style.Fill` when non-nil, including its
alpha: the stored solid color does not tint or attenuate a gradient. Use stop
alpha or `SetOpacity` for transparency. `Object.SetFillGradient` selects a paint
on the node and all current descendants, as `SetFill` does for a solid color.
It does not establish inheritance for children added later. All descendants use
the same world-coordinate field at assignment time.

`SetFillGradient(nil)` exposes the stored solid `Fill` again. `SetFill(color)`
and `SetColor(color)` explicitly clear gradient fills and select the solid
color. Directly changing `Style.Fill` while a gradient is present leaves the
gradient active. Stroke remains an independent solid color. Bitmap tinting,
including a gradient on an image plane, is rejected; opacity and image fades
remain supported.

## Supported paint semantics

- Linear: endpoints define the zero/one positions; contours perpendicular to
  that axis have equal colors in the initial frame.
- Radial: focus equals center, inner radius is zero, and color position is
  distance from center divided by radius. A transformed radial field can form
  an ellipse/sheared ellipse. Off-center focus and nonzero inner radius are not
  represented by this API.
- Extend: **pad only**. Positions before/after the ramp use the first/last stop.
  At a coincident offset, the last stop wins, enabling hard edges. If several
  stops have offset zero, positions strictly before zero use the first stop,
  while position zero uses the last coincident zero stop.
- Stops: 2–256 entries, nondecreasing finite offsets in `[0,1]`, with explicit
  first/last offsets 0 and 1. Duplicate offsets, including endpoints, are legal.
  A single stop, unsorted list or omitted endpoint is an error; the constructor
  does not sort, extend, clamp or silently discard input stops.
- Colors: valid **straight encoded sRGB** channels and alpha in `[0,1]`.
  RGB and alpha interpolate separately within each stop interval. This is not
  linear-light RGB and not premultiplied color interpolation. Compositing then
  uses premultiplied sRGB with actual vector coverage and the product of node/
  ancestor opacities, preserving the renderer's existing opacity model.

Repeat/reflect spread, gradient strokes, pattern/image paints, mesh/conic
fields, object-bbox units, automatic color-space conversion and full SVG focal
models are outside this core slice. Import code must explicitly map supported
units into world coordinates and reject unsupported semantics.

## Coordinates, transforms and camera

Creation coordinates are current **world coordinates**, not percentages of an
object's bbox. Assigning the same paint to multiple shapes initially samples
one world-space field. Geometry bounds are unchanged by a paint or its alpha.
Gradient fills pass through the existing nonzero-winding vector rasterizer,
including curved boundaries, holes, path reveal and nested intersecting Clips.
A gradient never replaces geometry with a rectangular color image.

`Gradient.Transform(geom.Mat4)` returns a new validated paint. The input must
be finite and 2D affine: perspective, z mixing and nonzero z translation are
rejected. Translation, rotation, shear, reflection and nonuniform scaling are
supported when the resulting frame remains within numeric bounds. Paints are
inverse-mapped through their complete affine frame; transforming only the
linear endpoints would give incorrect results under shear/nonuniform scaling.

The six-element matrix uses SVG order `[a,b,c,d,e,f]`:

```text
worldX = a*u + c*v + e
worldY = b*u + d*v + f
```

A linear paint samples `u`; a radial paint samples `hypot(u,v)`. The initial
linear matrix is `[dx,dy,-dy,dx,startX,startY]`, with `dx,dy=end-start`;
the initial radial matrix is `[radius,0,0,radius,centerX,centerY]`.

`Object.ApplyMatrix`, `Shift`, `MoveTo`, `Rotate`, `Scale` and group layout carry
the paint with geometry, creating an independently transformed descriptor while
sharing immutable stops. Existing geometry/stroke-width behavior is retained.
Copy/snapshot/assignment preserve paints; succession commits the complete style.

Camera projection transforms gradient frames alongside paths, bitmap planes and
clips. Projection does not mutate the source frame and retains sharing between
identical paint pointers. Camera zoom is applied consistently through the frame
width used for sampling and export. A reusable rasterizer restores its default
view after a camera frame. The rasterizer itself remains worker-local; immutable
frames/paints can be rendered concurrently with independent rasterizers.

## Animation subset and explicit errors

FadeIn/FadeOut, vector Create/Write/reveal, shift/move, positive scale and Rotate
carry the gradient. Full-turn and half-turn rotations use the existing angular
track rather than linearly interpolating affine matrices. Succession retains
paint state through each child's cleanup.

General Transform/ReplacementTransform/TransformFromCopy can interpolate affine
frames when gradient kind, stop count and every offset/color are identical.
Presence changes, kind changes and any stop/color/alpha changes are rejected
while preparing the animation. Use separate FadeOut/FadeIn layers for those
transitions. Equivalent immutable descriptors are allowed; pointer equality is
not required. A geometry morph with an unchanged field samples that field as
its coverage changes; it does not infer a material deformation from vertices.

Affine frame interpolation is checked over the entire `[0,1]` interval using
the determinant quadratic. Singular/ill-conditioned or excessive inverse paths
are rejected before authoring commits. In particular a direct Transform to a
half-turn matrix is rejected because its linear interpolation collapses; use
Rotate for that motion. Paint-frame extrapolation is unsupported: a gradient
track whose rate returns outside `[0,1]` produces an explicit error, even when
the number is finite. Nonfinite rates continue to fail as before. Animations
that collapse a gradient to a point (including zero-scale growing poses) fail
validation instead of selecting a solid fill.

Constructors/`Gradient.Transform` return errors immediately. Object's existing
chainable mutation methods defer invalid geometry/paint errors to validation,
scene authoring or rendering, as for Clip. Failed scene authoring is atomic.
Raster rendering returns no frame on failure; SVG file export aborts its atomic
output and never publishes a partial file.

## Bounds

| Resource/numerical input | Limit |
| --- | --- |
| Stops per paint | 2–256, including explicit offsets 0 and 1. |
| Gradient points, sampled points, painted path control points | Finite x/y within `±1e9`, z=0. |
| Affine coefficients, including translation | Finite and within `±1e9`. |
| Inverse linear coefficients | Magnitude at most `1e9`. |
| Frame determinant | `abs(det)/maxLinearCoefficient² >= 1e-12`; singular frames reject. |
| Raster coverage scratch per gradient layer | At most `16<<20` pixels, one byte/pixel (16 MiB). |
| Gradient raster work per frame | At most `64<<20` ROI pixels, counted before sampling; reset per frame. |
| Exported SVG paint-server stops | At most 65,536 total stops across unique paint pointers. |

The raster ROI includes padding and any clipped intersection. Offscreen/empty
coverage does not require a paint image. Coverage scratch is reused by each
worker; no full RGBA gradient image or per-pixel interface allocations are
needed. These budgets supplement, and do not replace, existing raster dimensions,
clip depth/curve/work limits and output-frame allocations. Clip implementation
and semantics are retained. The bounds are not a strict process heap/time quota.
Affine transition checks use a conservative maximum-coefficient/minimum-
determinant inverse bound and may reject extreme otherwise-invertible paths.
This is a floating-point implementation, not lossless conditioning or exact
arithmetic.

## SVG export

The exporter emits real `linearGradient` / `radialGradient` definitions with
`gradientUnits="userSpaceOnUse"`, `spreadMethod="pad"`,
`color-interpolation="sRGB"`, explicit stop RGB/opacity and a full
`gradientTransform="matrix(...)"`. Paths reference them with `fill="url(#...)"`;
object/ancestor opacity remains in fill opacity. Shared immutable paint pointers
reuse one definition. Definitions coexist with the existing Clip paint wrappers,
stroke attributes, camera transforms, image assets and Z order.

There is no screenshot/image substitution. The default nil-paint serializer and
solid raster branches are retained; the new optional Style field is omitted from
JSON when nil, preserving the existing mathtex layout/style digests.

## Verification and independent reference provenance

All tests and benchmarks are centralized under `tests/_cases/root`, `render`
and `export`. Relevant checks use only the central runner:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=export -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=export -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go vet -tags noasm . ./render ./export
```

Completed on 2026-11-04: all three complete package suites and all three complete
race suites passed, with `-tags noasm -vet=all`. Statement coverage was 91.5%
(root), 90.9% (render) and 86.9% (export). The final empty-group SVG paint-reference
regression also passed a focused race run. The existing mathtex control-point /
style fixture test passed unchanged. Production vet for the three packages and
`CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck` passed; the latter checked 41
non-standard packages with no native inputs or runtime process-launch imports.
Whole-repository test integration is handled separately.

Tests cover analytic linear/radial colors, stop alpha, hard/endpoint stops, pad,
immutability, transform invariance under shear/reflection/nonuniform scale,
object/group/camera semantics, snapshot ownership, concurrent sample/render,
bitmap rejection, malformed/numerical/resource errors, atomic animation/export
failure, reveal, succession, paint-server attributes and reuse. Existing root/
render/export tests include Clip behavior, Manim raster and CairoSVG dash
references, camera/bitmap/stroke behavior and solid-fill regressions.

`tests/reference/gradients.py` independently hand-authors SVG sources and invokes
CairoSVG 2.7.1 / Cairo 1.18.4. The frozen SVG, PNG and provenance SHA256 records
are under `tests/data/render/gradients`. No gomath implementation generates those
source-scene oracles. Their 160×120 premultiplied RGBA comparisons measured:

| Independent scene | MAE / channel | Maximum channel difference |
| --- | ---: | ---: |
| Linear hard stop with a nonzero-winding hole and pad | 0 | 0 |
| Affine radial, Clip and opacity | 0.0416406 | 1 |
| Linear changing stop alpha | 0.11375 | 1 |

Tests require MAE ≤0.55 and maximum difference ≤4 to allow 8-bit quantization /
coverage rounding differences. Curved coverage and transformed-camera equality
also have direct native tests; the three external scene references use integer
rectangular geometry to isolate paint semantics. Gradient edges are sampled at
pixel centers, then multiplied by antialiased coverage. They are not analytically
integrated across each pixel, so arbitrarily narrow color transitions may alias.

A second development check, `tests/reference/gradients_export.py`, interprets the
native SVG exports through CairoSVG and compares them to native PNGs. Artifacts
and provenance are under `tests/data/export/gradients`. Across the same three
scenes, MAEs were 0.0108333 / 0.0472396 / 0.153021 and maximum differences
1 / 1 / 2. These export checks are independent SVG interpretation, not independent
scene construction. They complement the hand-authored references above.

Development regeneration (Cairo libraries/Python are never required by Go tests):

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/gradients.py
# Use a fresh preview directory: exports intentionally reject existing files.
GOMATH_GRADIENT_PREVIEW_DIR=/path/to/fresh-preview CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -run '^TestGradientIndependentCairoSVGReferences$' -count=1
GOMATH_GRADIENT_PREVIEW_DIR=/path/to/fresh-preview CGO_ENABLED=0 go run ./tests/cmd/run -pkg=export -tags noasm -run '^TestSVGGradientPaintServersAndIndependentInputs$' -count=1
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/gradients_export.py /path/to/fresh-preview
```

## Measured workload

Measured on 2026-11-04, Go 1.27.3 (module Go 1.26.0), darwin/arm64 Apple M2,
GOMAXPROCS=8, `CGO_ENABLED=0 -tags noasm`, one raster worker and warmed reusable
rasterizer. One 640×360 frame contains a radius-2 vector circle in an 8-unit
frame, no stroke, three stops including changing alpha, over the default opaque
background. Linear endpoints are (-3,0)/(3,0); radial center/radius are (0,0)/3.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -p 1 -run '^$' -bench '^BenchmarkGradientRaster$' -benchmem -benchtime=150ms -count=3
```

| Paint | Three ns/op samples | B/op | allocs/op |
| --- | --- | --- | ---: |
| Linear | 4,246,510 / 4,240,031 / 4,227,591 | 950,577 / 950,440 / 950,440 | 17 |
| Radial | 5,130,767 / 5,129,699 / 5,179,147 | 950,437 / 950,437 / 950,437 | 17 |

Each operation renders one frame, including fresh output RGBA allocation,
validation, flattening, coverage, gradient evaluation and compositing. Camera,
Clip, animation sampling, SVG export and external renderer timing are excluded.
Only this new hotspot was benchmarked for the slice. An initial generic image
adapter caused hundreds of thousands of per-pixel interface allocations; the
final direct coverage compositor removes them without changing the public API.
The timing is a local measurement, not a cross-renderer performance claim.

## Integrated animated demo

The `gradients` CLI scene combines a rotated linear vector rectangle, a radial
circle with a native Clip, real vector text, and geometry/paint animation.
Independent CairoSVG re-rendering of its SVG output at .75,2.25,3.5 seconds
passed with RGBA MAE0.08480–0.08620/255 and maximum channel difference26.
The record is `artifacts/gradient-demo-svg-20261104/provenance.json`, produced
by `tests/reference/demo_svg.py`. It shares Go scene construction and adds
end-to-end export/raster checks, not another independent scene-input oracle or
an overall performance result.
