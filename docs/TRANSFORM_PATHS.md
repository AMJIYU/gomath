# Circular transform paths — wave 8

Implemented against Manim Community **0.22.1**, pinned source commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. Source hashes, generator hash,
checked geometry states, frame hashes, explicit limits and validation results
are recorded in `tests/data/root/transform-paths/manifest.json`.

```go
err := scene.Play(1,
    gomath.Transform(source, target).
        WithPathArc(math.Pi / 2).
        WithPathArcAxis(geom.Vec3{Z: 1}).
        WithRate(gomath.Linear),
)
```

`Animation.WithPathArc(float64)` and
`Animation.WithPathArcAxis(geom.Vec3)` return independent animation values;
deriving another option chain does not mutate an earlier one. Arguments are
captured by value. `Play` checks these options before committing authoring state.
Axis selection alone leaves the angle zero. Zero axes use OUT; finite nonzero
axes are normalized using component scaling, including huge and subnormal axes.

The same options apply to `ReplacementTransform` and `TransformFromCopy`.
`ClockwiseTransform(source, target)` defaults to `-math.Pi` and
`CounterclockwiseTransform(source, target)` defaults to `math.Pi`, matching the
source constructors. Both use the usual Smooth rate and accept option overrides.

## Geometry and endpoint behavior

For every pair of corresponding contour starts and cubic controls `s`, `e`,
the path evaluates

```text
v = e - s
c = s + v/2 + cross(unit_axis, v/2) / tan(angle/2)
p(progress) = c + rotation(unit_axis, progress*angle) * (s-c)
```

The exact positive-pi branch omits the cotangent term, as in the source. Each
point has its own center. Differently shaped endpoints therefore change shape
while their corresponding points traverse circles. No single whole-object
rotation is substituted. Standard tree/path alignment, styles, scalar values,
contour closure flags, existing child pointers and IDs retain their established
contracts.

The pinned implementation has an observable spatial limitation: a point's
coordinate along the axis remains its **starting** axial coordinate. At unit
progress, the result is `e - unit_axis*dot(e-s, unit_axis)`. An oblique axis with
axial displacement consequently does not reach `e`. This implementation preserves
that source behavior, including the authored final Transform state; it does not
silently snap the geometry to a different endpoint. Geometry tests include this
case. Tilted-axis coverage establishes 3D point trajectories, not a new 3D camera
or arbitrary spatial paint capability.

`ReplacementTransform` first authors the source's rate-dependent final state,
then installs the actual target pointer in the source's tree position. Cleanup
occurs at the Play/sequence-step endpoint, including for returning rates and
animations with shorter windows. The removed source is still the same object.

`TransformFromCopy` preserves the source and animates the actual target. Manim
swaps source/target and evaluates `rate(1-alpha)`. The internal forward style
clock is `1-rate(1-alpha)`; the geometry evaluates the original **reversed** point
path. Merely reflecting the rate or negating the angle is insufficient for the
oblique-axis case. Pinned tests cover both oblique copy geometry and asymmetric
square rates. Default copy rates leave the target at its original destination.

With no path options, or `abs(angle) < 0.01`, the existing straight interpolation
and pixel output are retained. This threshold is taken from
`manim/utils/paths.py`; exactly `0.01` selects a circular path.

## Timing, ownership and errors

Leaf windows clamp input time before applying the rate. Finite rate overshoot,
including negative output, continues around the same point circles. Nested
parallel group clocks, leaf windows and child finish behavior use existing
composition plumbing. A group finishes each leaf at its own final clock even
when the group rate returns to zero. Path options on a group are rejected; attach
them to its Transform leaves. Succession captures the next child's geometry
after the previous child's finish and cleanup, and retains its existing linear
group-clock/full-window restrictions.

Preparation operates on private snapshots. A failed Play or late Succession
step leaves the timeline, input geometry, IDs, source/target pointers and child
pointers intact. Every circular frame validates visual geometry, clip paths,
gradient frames and bitmap planes before committing its private result.
Sample-time errors return a nil frame and do not corrupt later samples.
Arbitrary user rates cannot be proven safe at all interior times during authoring;
an interior rate or paint-bound error is reported when that time is sampled.

The prepared arc track is immutable. Frozen Replay tracks may share it safely;
each sampled path and returned child tree is independent. Clips, gradients and
bitmap assets retain their immutable storage contracts. Rates must be pure and
safe for concurrent calls after authoring, as for other animations.

## Paint and image support

* **Clips:** matching clip presence and fill rules; planar `z=0` outlines; OUT or
  IN axes. Each aligned clip control follows its own corresponding-point arc.
  Child/ancestor intersection semantics remain unchanged. Clips do not inherit
  a guessed whole-object rotation or a straight moving mask.
* **Gradients:** matching kind/stops and identical first four affine-frame
  coefficients. The frame's world origin follows the circular path, and its
  basis remains unchanged. Linear and radial translation frames are supported,
  including combined clipped gradient rendering. Differing bases, tilted axes,
  presence changes and stop changes are rejected. This is a checked translation
  frame contract; it does not claim arbitrary deformation of a paint field.
* **Bitmaps:** matching immutable assets and valid four-edge affine image planes.
  Corresponding affine controls remain affine under the point-path linear
  combinations. Every produced plane is checked with `ImageCorners`; curved or
  perspective endpoint planes and incompatible assets are rejected. No nonlinear
  image resampling or geometry-warp fallback is introduced.

Planar gradient coordinate, inverse and conditioning limits still apply to
every produced frame. A circular translation can leave those bounds even when
its endpoints are valid; sampling reports that error. Straight-path gradient
extrapolation restrictions remain unchanged.

## Explicit bounds

| Control | Checked bound |
| --- | --- |
| Angle | finite, absolute value at most `2*pi`; nonstraight arcs require `abs(sin(angle/2)) >= 1e-6` |
| Axis | three finite components; robust normalization or zero-axis OUT fallback |
| Authored path/clip coordinates | each component within `+/-1e9` for circular paths |
| Controls | at most `1<<20` across checked endpoint trees, counting contour starts and four controls per cubic |
| Alignment work | at most `16<<20` contour subdivision/scan work units |
| Effective leaf rate output | finite, absolute value at most `1e6` for circular paths |
| Mode | Transform, ReplacementTransform, TransformFromCopy leaves only |

Existing tree/clip/bitmap/paint budgets also apply. Full turns, near-full-turn
singular centers, multi-turn angles, custom `path_func`, custom
`path_arc_centers`, path options on relative operations/Create/Fade/pose/path
movement, and path options on groups are unsupported. They are not silently
approximated or advertised as implemented APIs. Checked builder options can
only express the supported arc/axis controls.

## Evidence and reproducibility

`tests/reference/transform_paths.py` is development-only. It verifies eight
installed Manim source files byte-for-byte against the pinned Git blobs before
generating 105 independent geometry states and 12 Cairo PNG frames at interior
times. The fixture includes straight/tiny/threshold paths, positive and negative
arcs, arcs over pi, normalized and zero axes, oblique displacement,
ReplacementTransform, copy/reversed copy, asymmetric copy rates,
clockwise/counterclockwise defaults and positive rate overshoot.

Go tests require no Python, renderer subprocess or native binding. The pinned
geometry tolerance is `1e-10`. Cairo/Go frame comparisons require foreground-mask
IoU at least `0.96` and mean absolute red-channel error at most `1.5` on 128x96
black-background white-shape frames. Reference frame SHA256 hashes are checked.
These thresholds account for independent rasterizer edge antialiasing.
Clip/gradient frames additionally match an independently placed painted object
within one byte per channel. Timing, pointers, identity, cleanup, errors, numeric
bounds and concurrent scene/replay sampling have Go contract tests.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -run '^TestTransformPath' -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=root -run '^TestTransformPath' -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=root -run '^$' -bench '^BenchmarkTransformPath(Sample|Render)$' -benchmem -benchtime=200ms -count=3
CGO_ENABLED=0 go test -tags noasm ./...
CGO_ENABLED=0 go vet -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

CGO is enabled only for the Go race-detector development run. Product validation
and benchmarks use `CGO_ENABLED=0` and `noasm`. Production was staged in an
underscore directory; tests were staged under `tests/_cases/root/` with an
underscore filename and an explicit source overlay. The focused/full root suite
and focused race checks passed before production promotion. The underscore
production directory and temporary overlay were then removed.

Exact validation commands, benchmark workload/hardware/results and final file
hashes are in the fixture manifest. This slice adds the bounded behavior above;
it makes no comprehensive Manim parity claim. Existing reference fixtures were
preserved. Shared README, STATUS, PARITY, examples and CLI files were not changed
by this slice.
