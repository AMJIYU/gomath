# Exact-key fragment matching

`gomath.TransformMatchingParts(source, target, sourceKeys, targetKeys)` returns
a checked `Animation` for the direct children of two plain object groups.
`mathtex.TransformMatchingParts(sourceParts, targetParts)` supplies exact keys
from separately valid formula fragments. Neither API infers fragments from a
concatenated expression or hashes glyph outlines.

```go
a, err := mathtex.NewParts([]string{"x", "+", "x", "=", "y"}, options)
if err != nil { return err }
b, err := mathtex.NewParts([]string{"y", "=", "2", "x"}, options)
if err != nil { return err }
animation, err := mathtex.TransformMatchingParts(a, b)
if err != nil { return err }
if err := scene.Add(a.Object()); err != nil { return err }
if err := scene.Play(1.5, animation.WithPathArc(.55)); err != nil { return err }
// The scene now owns b.Object() and its real live fragment handles.
```

Keys use exact UTF-8 string equality, including whitespace and empty strings.
Each side accepts at most256 parts and65536 total key bytes. Construction
clones strings/slices and captures direct child IDs/order. Geometry is captured
when `Play` begins; changing the direct child IDs/order between construction
and Play rejects. Changes within a fragment remain subject to ordinary checked
object ownership and the matching budget.

## Correspondence and cleanup

Equal keys form buckets. Members retain authoring order within a bucket;
buckets use first source-key occurrence, followed by target-only keys. This
deterministic order differs from Python's hash-set iteration. Overlapping
fragments at equal ZIndex can therefore paint in a different order from Manim.

If a matching bucket has `n` children and its peer has `m>n`, output slot `j`
comes from original child `floor(j*n/m)`. The first copy retains its identity;
additional copies have new IDs and start/end transparent on that side.
Corresponding bucket trees then use the existing checked Transform alignment.
That engine's longest-curve subdivision and index-based contour/descendant
alignment remain in effect. Automatic arbitrary glyph-topology alignment is
not proven equal to Manim's different point-array alignment.

Unmatched source fragments fade out while translating their complete group
center to the unmatched target group's center. With no unmatched target, that
center is the origin. Unmatched target fragments fade in at their own authored
positions. Centers use GoMath's true geometric cubic bounds, excluding stroke;
the independent pinned comparisons below use straight closed polygons where
these agree with the source. Empty fragments carry keys but no invented ink.
Repeated copies and temporary bucket parents exist only in immutable tracks
and private sampled trees; sampling allocates no new object IDs.

At the segment endpoint the actual target root replaces source in its existing
root/nested scene slot. Target pointers/IDs and formula selections remain live.
The removed source's paths, direct child pointers and topology remain unchanged,
so it can be re-added or used in a later Succession step. Source child IDs stay
findable during interpolation, beneath private bucket parents. The source root's
direct sampled topology is temporarily different from its authored topology.
Incoming ghosts have private IDs; target handles enter the scene at cleanup.

Both roots must have no own path, bitmap, clip or fill gradient and must have
opacity/reveal1. This slice accepts unclipped vector families; bitmaps and
clipped descendants reject. Compatible gradient-filled vector parts use the
existing Transform rules; incompatible gradient/fill-rule combinations reject.
Use the ordinary independently layered FadeTransform API for different assets.
There is no key-map alias policy, geometric shape matching, or alternate
transform/fade-transform mismatch policy in this slice.

## Clocks, ownership and bounds

`WithRate`, windows, parallel groups, `WithPathArc` and Succession use the
ordinary scene scheduling rules. Since some branches fade, evaluated progress
must stay in[0,1]; nonfinite/extrapolated rates reject without publishing a
sample or committing an invalid Play. Concurrent samples are independent after
authoring completes. Replay captures the immutable matching tracks. Managed
source-family updaters suspend during the complete Play segment by default;
source-child handles remain present in the temporary tree. Updaters on a root
that opt out of suspension must account for its sampled bucket topology.

Inputs are iteratively checked before recursion/copy: maximum8192 combined
nodes,16384 combined contours,65536 combined cubics, input depth60 and path/control
coordinates within±1e9. Pending children are checked before stack growth.
Repeated-bucket expansion is charged before each copy against the same aggregate
node/contour/curve caps. Both aligned trees together can contain the union of
both sides; two such trees and temporary authoring snapshots add memory beyond
these input/expansion payload caps. This is not a process RSS guarantee.
The existing transform-path preflight additionally limits path-alignment
points and worst-case subdivision work, even for straight trajectories.
No live authored tree is changed if construction, preflight, any Play track or
a later Succession step fails. User callback side effects remain outside that
transaction, as with other animation APIs.

## Reference evidence and measurements

`tests/reference/matching_parts.py` runs actual
`TransformMatchingAbstractBase` with an exact-key subclass on independently
specified four-cubic polygons. It verifies nine installed/local Manim files
byte-for-byte against CE0.22.1 commit
`16ea443e2a5f1d79db0a721161411ce916d6e733` before generating exclusive fixtures.
Nine cases include reorder, repeated-key growth/shrinkage, mixed/all mismatches,
empty source/target, plus circular reorder/mismatch trajectories.63 in-flight
states compare every complete cubic control, fill color and effective opacity
as named multisets, at absolute tolerance1e-10. Nine explicit Python cleanup
cases verify target installation and source restoration. These references
establish the default key-bucket policy for that geometry, not complete
TransformMatchingTex, arbitrary glyph topology or whole-formula TeX parity.

The initial reference accidentally captured AnimationGroup's non-introducer
group only, omitting FadeIn objects. Its generator, fixture and failed log are
retained as history under `tests/data/root/matching-parts/`; the corrected
generator captures every child animation's actual mobject. The straight-only
revision is also retained. Go tests never run Python or a reference process.

Separate tests exercise formula live target handles, strict errors, identity,
repetition budgets, nested replacement, group windows, atomic late Succession
failure, source reuse, managed suspension, replay and concurrent isolation.
The `matching-parts` CLI example animates `x+x=y` → `y=2x` → `2x-y=0`.

Sampling reuses already aligned bucket topology from `seg.before` rather than
cloning the entire tree again. For32 unique square keys, one Sample(.5) per
operation, three200ms samples on the shared Apple M2 host observed allocation
medians65168→34960B/op and334→173allocs/op. Time medians18592→20245ns/op
regressed8.89%. These non-interleaved samples demonstrate lower allocations,
not a speed advantage; other workers were active. Construction, rasterization,
images, I/O, encoding and Python are excluded. Exact commands, compiler, host,
source hashes and raw samples are in
`tests/data/root/matching-parts/performance/summary.json`. No Manim performance
comparison or universal improvement is claimed.
