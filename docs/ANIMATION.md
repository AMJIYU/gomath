# Animation timing, easing and indications

`Play(seconds, animations...)` advances authoring and saves independent initial
and final snapshots. Targets in one Play must be disjoint, including parent/child
trees. `Transform` keeps source IDs and existing child references; replacement
installs the real target only at the Play endpoint. Sampling after authoring is
read-only and can run out of order or concurrently.

## Easing

The default `Smooth` is Manim's normalized sigmoid at inflection 10. The previous
cubic polynomial remains `SmoothStep`. `SmootherStep`/`SmoothererStep`, the common
Manim rates, and sine/polynomial/exponential/circular/back/elastic/bounce ease
families are also available. Constructors such as `SmoothRate`, `PausedRate`,
`WiggleRate`, `RunningStartRate`, `NotQuiteThere` and `SquishRate` validate their
parameters and return `(RateFunc, error)`.

Input animation windows clamp to [0,1], then the rate transforms progress.
Finite rate overshoot is preserved for vector geometry and scalar interpolation;
style channels remain bounded, motion-path fractions clamp to the measured
path, and camera progress retains its existing bounded policy. A custom function
must be deterministic, finite and safe for concurrent reads of its captures.

## Nested groups

```go
group := gomath.LaggedGroup(.35,
    gomath.Shift(left, geom.V(2, 0)).WithDuration(2),
    gomath.AnimationGroup(
        gomath.Shift(middle, geom.V(2, 0)),
        gomath.Shift(right, geom.V(2, 0)).WithDuration(.5),
    ).WithRate(gomath.Smooth),
)
if err := scene.Play(4, group); err != nil { return err }
```

A leaf defaults to relative duration 1. A group defaults to its natural span;
`WithDuration` rescales its allocated runtime. Child start advances by
`lag * previousChildDuration`; the span is the **maximum child end**, even if
an earlier child lasts longer than the last. `AnimationGroup` uses lag 0.
`Play` scales the root schedule to its requested duration. Each group has its
own rate/window clock; its warped time resolves the child windows recursively.
At Play's endpoint every leaf finishes, even if a parent's rate returns to zero.
Trees are limited to 10,000 nodes and depth 64.

These parallel groups still require disjoint targets. A lag of 1 alone does not
provide delayed initialization for repeated operations on one object.

## Shared-object Succession

```go
sequence := gomath.Succession(
    gomath.Shift(object, geom.V(2, 0)),
    gomath.Shift(object, geom.V(3, 0)).WithDuration(2),
    gomath.Rotate(object, math.Pi),
)
if err := scene.Play(4, sequence); err != nil { return err }
```

Each step begins from the preceding step's final state. `Shift`, `MoveTo`,
`Scale`, and `Rotate` capture numeric arguments when constructed, but capture
geometry and rotation/scale centers when that step begins. This also applies
when a stored relative animation is passed to a later ordinary Play. Explicit
Transform targets retain their absolute geometry. Nested sequences and parallel
groups as individual steps are supported; parallel steps retain disjoint writes.
Child `WithDuration` values determine runtime shares recursively. Leaf rates and
windows work within their steps. Cleanup, removal, introduction and actual
replacement pointers take effect at the respective step boundaries. A later
FadeIn can reuse a FadeOut object with its restored authoring opacity.

The whole operation stages private canonical object copies and commits only
after every step validates. Invalid late children leave authoring objects,
topology, snapshots and duration unchanged. Existing child pointers survive
ordinary transforms; replacements install the real target pointer. At most
100,000 input/generated objects are captured per sequence, in addition to the
shared 10,000 animation-node/depth-64 budget. Unrepresentable fractional or
absolute child boundaries are errors. Immutable segments preserve out-of-order
sampling and concurrent replay.

Current bounds: Succession must be the sole Play argument, or a direct child of
another Succession. Sequence group clocks must be Linear (or nil) and their
window [0,1]. A sequence nested inside a parallel group, returning/warped sequence
clock or partial sequence window rejects explicitly. This subset is not complete
Manim Succession parity: Manim delays some scene cleanup until outer cleanup,
whereas GoMath cleans each step. The pinned 51-state fixture verifies shared
relative motion, centers, unequal/nested timing and every rectangle control
point, including interior times. It does not establish cleanup or animation
builder semantics parity; the reference uses deferred ApplyMethod and Rotating.
Run the `succession` example for a composed scene.

## Copy transforms

`TransformFromCopy(source, target)` keeps the source unchanged and animates the
actual target from source geometry to its own captured geometry. A missing
target is introduced at that step's start; its own IDs and existing child
pointers are retained, including when used in Succession. Source geometry is
captured when the step begins. As with ordinary morphs, bitmap assets must
match, and topology alignment can add transparent placeholders.

Manim implements this by swapping source/target and reversing the interpolation
input. GoMath uses the equivalent reflected leaf rate `1-rate(1-alpha)`;
nonsymmetric easing is therefore intentionally different from a normal forward
Transform. Reflection happens inside the child's window and ancestor clocks.
A pinned 44-state control-point fixture verifies linear, quadratic, overshooting
and nested group/window cases. This is not full arbitrary-topology/bitmap
conversion or source-token matching parity.

## Independent-layer fade transforms

`FadeTransform(source, target)` supports different bitmap assets and image/vector
transitions by compositing two real layers. The outgoing layer fits the target's
bounds while fading out; the incoming layer begins fitted to the source and
fades to its own geometry. Default fitting stretches each nonzero axis.
`FadeTransformWithOptions` can instead uniformly match dimension0/1/2. A zero
source extent stays unscaled, and an empty peer produces a fade in place.
Invalid/unrepresentable fits, including degenerate bitmap planes, return errors.

The target is introduced at the step's start with its own identity. Source is
removed and its captured author geometry/style restored at the step's endpoint.
The actual target remains available for subsequent animation. In ordinary
parallel groups cleanup still occurs at Play's endpoint; in Succession it occurs
at that step's endpoint. Both layers use the group clock (default Smooth), so
WithRate/WithWindow apply to the complete fade. ZIndex/drawing order and ordinary
alpha compositing determine overlapping layer colors; this is not a linear RGB
pixel blend. Existing topology inside each layer is preserved independently.

The 33-state pinned rectangle fixture compares both layers' complete control
points and opacity under stretched, width-matched and height-matched fitting.
Separate Go tests render different red/blue bitmap assets and image/vector
transitions, verify source restoration/target identity and sequential reuse.
This evidence does not establish arbitrary nested opacity, full Manim scene-ID
timing or FadeTransformPieces compatibility. In Manim the actual target is
installed at cleanup; GoMath uses its ID throughout the incoming layer.

## Growing and indications

`GrowFromPoint`, `GrowFromCenter`, `GrowFromEdge`, `GrowArrow`, `SpinInFromNothing`
and configurable `SpinIn` introduce vector objects from a collapsed snapshot.
`WithPointColor` selects the initial color. Edge direction signs choose critical
bounding-box coordinates; GrowArrow uses the first path start and scales the
whole tip. IDs and existing child references are preserved.

`Indicate[With]` temporarily scales/recolors with ThereAndBack. `WiggleObject[With]`
uses independent scale/rotation centers, and `ApplyWave[With]` deforms vector
geometry with bounded traveling ripples. The scalar rate `Wiggle` has a separate
name from the object animation. Wave handling includes Manim VMobject's .01
handle-to-anchor pre-function scaling/restoration. These pose animations
currently reject bitmap trees explicitly. Options are copied during authoring;
custom wave functions retain the callback purity/concurrency contract.

Pinned JSON fixtures check 728 rate values, 55 eased transform states, 44 group
states and 88 indication/growing states. See `tests/data/root/README.md` for
provenance, tolerances and the limits of that evidence. This is not full Manim
animation or per-symbol compatibility.

## Passing flashes and erasure

`ShowPassingFlash(object)` and `PassingFlashWithWidth(object, width)` show a
moving sliver in explicit cubic parameter space, introduce absent vector
objects, and remove them at Play's endpoint. Default width is .1, rate Smooth;
width must be finite in (0,1e6]. Cleanup restores full author geometry/child
references for reuse. `Uncreate`/`Unwrite` erase an existing vector object by
decreasing Reveal then remove it. Its stored Reveal ends at zero; reset it to 1
before reintroducing its full stored path with Create. Bitmap inputs reject.
Groups retain the existing Play-endpoint cleanup policy.

FadeOut now restores the author's captured geometry/style after removal, as
Manim's cleanup does. A later FadeIn can therefore reuse its original opacity.
Pinned endpoint fixtures cover 27 passing-flash/Uncreate states and one FadeOut
cleanup opacity. This does not verify every partial-path control point, family
lag, fill behavior, or the representation of an erased authoring object.
