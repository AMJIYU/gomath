# Checked planar braces

`NewBrace(target, BraceOptions)` and `NewBraceBetweenPoints(start, end,
BraceOptions)` create real filled cubic outlines in pure Go. The fixed outline
is derived from the MIT-licensed Manim CE0.22.1 `brace.py` template at commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`; its notice is retained in
`LICENSES/manim-LICENSE.community.txt`. No runtime SVG parser, Python, font
substitution, native binding or external typesetter constructs the brace.

```go
options := gomath.DefaultBraceOptions() // down, buffer .2, sharpness 2
brace, err := gomath.NewBrace(formula, options)
if err != nil { return err }
brace.Object.SetColor(gomath.Blue)
placed, err := brace.LabelAtTip(label, .15) // independent copy of any ink object
if err != nil { return err }
group := gomath.Group(formula, brace.Object, placed)
if err := scene.Add(group); err != nil { return err }

segmentBrace, err := gomath.NewBraceBetweenPoints(start, end,
    gomath.DefaultBraceBetweenPointsOptions())
```

## Placement and outline

Direction is a finite nonzero XY outward vector; magnitude does not affect
placement. The orthogonal local X axis is `(-dy, dx)` after normalization and
local Y points inward. Target geometry is projected before taking complete
cubic extrema. This is not a rotation of an already enlarged world AABB.
The brace fits the local geometric width, with its top anchor edge at the
target's lower edge minus Buffer. Stroke, clip, alpha and Reveal do not alter
these authored geometric bounds. Varying-Z targets are projected to the XY
plane at their full bounds' center Z. Empty/zero-width projected targets reject.

The source template is affine in a nonnegative horizontal section length:

```text
section = max(0, (projectedWidth*Sharpness-.90552)/2)
horizontalScale = projectedWidth/(.90552+2*section)
```

The original 25 cubics, including straight and closing segments, remain in
source order. Zero-length section segments remain present so parameter-space
Create and equal-topology Transform retain their established ordering. Height
is approximately `.272985` world units regardless of width/sharpness; Sharpness
changes the horizontal cap stretch. This is a filled outline, not a stroked
centerline. Initial fill is white, stroke is transparent with width zero, and
ZIndex copies the target once. Ordinary SetColor/Transform/Write work normally.

The source template's anchor placement is intentional. Its actual curve extrema
can extend beyond its anchor edge by about `4.6e-11` world units before further
transforms. Target measurement uses actual extrema, whereas Manim VMobject
placement uses anchors. Straight-target comparisons match; arbitrary curved
target placement is an explicit GoMath policy, not a complete Manim claim.

Point constructors require distinct bounded endpoints sharing one Z. Zero
Direction automatically chooses `(dy,-dx)`; reversing endpoints chooses the
opposite side. A nonzero Direction overrides this. `DefaultBraceOptions` is an
explicit downward direction; use `DefaultBraceBetweenPointsOptions` for the
automatic segment normal. This API does not infer a general spatial plane.

## Tips and labels

`Brace.Object` is the actual editable object. `Tip()` follows the selected source
control point in its current path. `Direction()` matches Manim's accessor policy:
normalize tip minus the current **anchor bbox** center. On oblique or skewed
braces this can differ appreciably from the original outward vector or the
center of `Object.Bounds()`. The constructor retains no source target or updater.
Getters reject nil/replaced objects, changed IDs, children, bitmaps and changed
contour/curve topology. Ordinary geometry/style edits and Scene Transform retain
the selected tip, root pointer and ID. Concurrent getters require authoring to
have finished, as for ordinary objects.

`LabelAtTip(label, buffer)` returns a complete independent copy of a bounded valid
ink object. It rounds Direction's components to even, selects the label corner
facing the tip, then applies buffer on each selected axis. A diagonal applies
the gap on both axes; this follows default Manim `put_at_tip(use_next_to=True)`.
Actual label cubic extrema are used. Text/mathtex can be supplied by their normal
packages; root geometry has no font-package dependency. Zero-ink labels reject.
Inputs, child pointers and IDs are never changed. There is no automatic reflow:
after placing the label, group it with the brace for a common animation. The
alternative `use_next_to=False` policy, BraceLabel mutators, text-producing
convenience constructors and nonlinear ArcBrace remain pending.

## Limits and errors

The target/label tree shares annotation limits:4096 objects,64 depth,16384
contours and65536 cubic curves; even hidden geometry is validated and charged
before recursive traversal/copy. Source and output control coordinates must be
finite within ±1e12; projected width and point distance are in `[1e-9,1e12]`.
Buffer is finite `[0,1e12]`; Sharpness is `[1e-9,1e6]`. Output overflow rejects.

Before output allocation the conservative detail estimate
`.0001*min(1,horizontalScale)` must be at least32 ULPs at the target/buffer scale.
This prevents collapsed cap detail after large translations or extreme stretch;
it deliberately rejects some otherwise representable finite combinations.
Direction components may be subnormal; normalization avoids reciprocal overflow.
All constructor/label errors return nil/error without mutating input trees.
Sampling uses normal independent scene snapshots and runs no placement callback.

## Evidence

All tests, benchmarks, fixtures and development tools are in `tests/`.

- `tests/reference/brace_template.py` extracts source cubic data at two section
  lengths, with exact integer/third stretch coefficients; generated Go data is
  committed. Regeneration requires the pinned development environment, an
  explicitly new output file and gofmt. This is source migration, not an oracle.
- `tests/reference/braces.py` calls pinned Manim directly:32 constructions,
  224 equal-topology Transform states, all ordered controls within2e-11 and tips
  within2e-11. It covers straight targets/groups, both automatic segment sides,
  cardinal/oblique directions, zero and nonzero horizontal sections, sharpness,
  buffer and animation interiors. No Go construction/output is read.
- `tests/reference/brace_labels.py` adds12 independent rectangular-ink label
  placements within2e-11. No whole-font or curved-label parity is claimed.
- Central tests independently check true cubic extrema, projected width/gap,
  source/output ownership, malformed/degenerate/precision errors, tip identity
  through Transform and concurrent private snapshots. Existing root tests pass.
- The constructor-only benchmark uses a prebuilt rectangle/circle target,
  defaults, CGO0/noasm/GOMAXPROCS1. Three shared-host samples are5.965/6.407/
  12.206µs,6656B and10allocs/op. The large variance is retained; no speedup or
  Manim-relative claim is made. It excludes target construction, labels, frames,
  rasterization, encoding and reference tools.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -count=1 -vet=all
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=root -race -count=1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=root -run '^$' \
  -bench '^BenchmarkBraceConstruction$' -benchmem -benchtime=250ms -count=3 -p=1 -cpu=1
```

The seventh-wave whole-repository checkpoint must be recorded separately after
all concurrent modules freeze. The sixth checkpoint does not certify this code.
