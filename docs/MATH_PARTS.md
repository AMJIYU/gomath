# Explicit mathematical fragments

`mathtex.NewParts(sources, options)` constructs one owning scene object with a
stable direct child for each of 1–256 ordered TeX fragments.
`mathtex.LayoutParts` additionally returns `PartsMetrics{Advance, Height, Depth}`.
Each fragment must be separately valid within the existing bounded Go mathtex
grammar. Construction is checked and atomic: failure returns nil and, for
`LayoutParts`, zero metrics. There are no new dependencies or runtime native,
Python, subprocess, or external TeX requirements.

```go
parts, metrics, err := mathtex.LayoutParts([]string{
    `\mathbf{x}`, `=`, `\frac{a}{b}`, `\quad`, `\phantom{W}`,
}, mathtex.DefaultOptions())
if err != nil {
    return err
}
scene.Add(parts.Object())
fraction, _ := parts.Part(2)
target := fraction.Copy().Shift(geom.V(0, .4)).SetColor(gomath.Blue)
if err := scene.Play(1, gomath.Transform(fraction, target)); err != nil {
    return err
}
_ = metrics.Advance // Original semantic advance, including phantom/space width.
```

## Fragment layout and boundaries

The first fragment starts at x=0, y=0. Every subsequent fragment starts at the
sum of previous fragments' semantic advances, on the same y=0 baseline.
Height/depth are the maxima of the individual extents. These metrics and
`PartInfo` placements describe initial authoring, and remain unchanged by later
object transforms. They are not recomputed ink bounds.

No gap, kerning, script attachment, or style declaration crosses a boundary.
Each fragment starts with the supplied `Options`; local font, color, alphabet,
and display-style declarations remain local. Preserve intended spacing with
valid spacing fragments such as `\quad`, or put atoms that need ordinary row
spacing in the same fragment. The existing Go row layout inserts .04em between
atoms inside one fragment. Thus `{"n", "g"}` and `{"ng"}` intentionally differ.
This API makes no claim that independently laid out fragments equal a single
concatenated TeX/MathTex expression.

For example, `{"x", "_i"}`, `{"\\frac{", "a}{b}"}`, and a `\left`/`\right`
pair split between fragments fail. `{"x_i", "\\frac{a}{b}"}` is valid. Double
braces do not trigger automatic isolation; they are ordinary group syntax.
There is no macro expansion spanning fragments, substring source mapping,
automatic tokenization, or automatic TransformMatchingTex implementation.

Empty strings, whitespace-only fragments, `{}`, spacing, and phantom fragments
retain an indexed handle and their exact key. Zero ink is represented by an
empty object, with semantic dimensions carried separately. No invisible paths
are manufactured to influence `Object.Bounds()`.

## Keys, ownership, copies, and snapshots

`Keys()` returns an independent ordered slice of the exact original strings,
without trimming or normalization. `Info(i)` returns a `PartInfo` value with
index, exact source, object ID, original x, advance, height, and depth.
`Part(i)` returns its borrowed live child and a success boolean. Invalid indexes
return false. Repeated keys stay separate through their indexes and IDs.
Object names are initialized from the exact source, but subsequent changes to
`Object.Name` do not change matching keys.

`Select(key)` uses exact equality and returns every matching child in authoring
order. The selection holds live pointers; its `Objects()` accessor copies only
the slice. Geometry/style edits and ordinary child or parent `Transform`
operations preserve these handles. A selection is not another owning group.
Add only `parts.Object()` to the scene. Adding its borrowed children again
violates the library's tree ownership rule. `selection.Copy()` creates a
checked standalone group with fresh IDs; an unmatched selection copies to an
empty group. `parts.Copy()` creates an independent Parts tree with fresh IDs,
the same keys and original layout metrics, and handles into that new tree.

Do not change direct fragment IDs, reorder/replace direct fragment children,
or introduce cycles/shared children. Checked copies and snapshots reject these
ownership changes. Geometry below a fragment may change through ordinary
authoring and Transform. Removing/replacing a handle via arbitrary child-slice
surgery is outside this API's stable-handle contract.

`Snapshot()` validates and captures the current authored geometry, exact keys,
placements, and IDs. It requires exclusive access to the live tree during
capture. A `PartsSnapshot` keeps its captured geometry private. Its `Object()`
and `Part(i)` return detached deep copies retaining captured IDs; `Select(key)`
returns handles into detached copies. Returned values may be edited without
changing the snapshot. Use `.Copy()` to give a captured object fresh IDs before
adding it alongside its source in a scene. Immutable font/gradient/clip/bitmap
assets follow gomath's existing sharing rules.

After authoring completes, concurrent reads/captures and independent scene
sampling are supported. Do not mutate the live tree while capturing it or
sampling its scene. Once captured, a snapshot may be read concurrently even
while the original live tree is edited. The captured IDs, exact keys and
ordered fragment roots provide data for a future matching planner; no matching
animation or automatic correspondence policy is supplied here.

## Combined resource limits

Preflight checks UTF-8 and sums all fragment source runes before parsing. The
entire call shares `MaxSourceRunes=16384`, `MaxEnvironmentCells=4096`, and
`MaxLayoutCurves=1048576`. Parser nesting remains `MaxNesting=128` per syntactic
branch, with the existing row/column limits. Fragment boundaries do not consume
syntactic depth, permit unclosed groups, or reset cumulative cell/curve budgets.

One parser retains its resource counters across fragments. Operand auditing is
enabled before the first fragment; glyphs, supplemental symbols, accents and
legacy structural paths charge the existing curve budget once. Discarded
phantom operand ink stays charged. Source limits also bound work for hidden
groups and zero-ink text. Additional aggregate limits are:

| Limit | Scope |
| --- | --- |
| `MaxParts=256` | Ordered handles, including empty fragments |
| `MaxPartsNodes=262144` | Combined retained tree plus discarded path nodes |
| `MaxPartsGlyphs=16384` | Path-bearing nodes, including structural marks and discarded ink |
| `MaxPartsTreeDepth=1032` | Retained object-tree depth, including the Parts root |
| `MaxPartsCoordinate=1e9` | Absolute path/control coordinates and semantic extents |

Dimensions must remain finite and nonnegative, and positive advances must stay
representable after accumulation. Coordinates are checked again after each
fragment is positioned. Checked copies/snapshots audit current tree bounds,
curves, visual validity, ownership, and IDs before recursively copying it.
Invalid/overly deep authored trees fail before recursive copying. Original
authoring metrics remain historical data when geometry is transformed.

Parent integration additionally caps total stored contours at
`MaxLayoutCurves`, including zero-curve contours. It checks pending children
against `MaxPartsNodes` before growing the traversal stack and checks curve
counts before addition. Thus authored empty contours or very wide child
lists cannot bypass copy/snapshot preflight. Exact source keys are cloned
when retained, so a short caller substring does not retain a large backing
string. These refinements and their focused validation are recorded separately
in `tests/data/mathtex/parts/parent-integration.json`; the original agent
validation remains a record of its delivered revision.

## Reference scope and reproducible validation

Source review is pinned to Manim **0.22.1**, commit
`16ea443e2a5f1d79db0a721161411ce916d6e733` in `../manim`.
Manim's `tex_mobject.py` joins/isolation-processes TeX source with its own
rules. This explicit Go fragment API deliberately has different boundary
semantics. No whole-concatenation or full Manim MathTex parity claim is made.
The shared parity/status documents remain parent-owned; integration should
record this as bounded explicit fragment authoring.

`tests/reference/mathtex_parts.py` reads original Go TTF files with fontTools,
constructs finite fragment boxes manually, and rasterizes their independent
SVG glyph outlines using CairoSVG/Cairo. It never consumes Go layout/output.
Nine expressions cover baseline/advance, repeated keys, zero-ink fragments,
phantom fraction dimensions, real font faces, local color/style scopes, and
fractions. Four more fixtures compare whole-object and individual-fragment
Transforms at t=.35 and .70. These are independent affine glyph references,
not independent TeX/Manim layout references. Source-font extrema rounding can
differ by one font design unit.

Go reference tests verify generator/font/fixture SHA-256 hashes, semantic
metrics and per-fragment geometry. Pixel checks use 768×768 images, a 6-unit
frame width, 128 pixels/em and black background, requiring MAE≤.08, RMS≤1,
maximum channel error≤64, and ink-mask IoU≥.97. Tests also check source keys,
selection/copy ownership, stable IDs through interior Transforms, failure
atomicity, concurrent immutable snapshot access/sampling, and exact/over-limit
resource cases. Pure-Go tests read committed fixtures without starting Python.

```sh
# Independent fixture regeneration, development environment only:
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib python3 tests/reference/mathtex_parts.py \
  --gofont-dir /Users/eleven/go/pkg/mod/golang.org/x/image@v0.47.0/font/gofont

# Staged or promoted overlay (standard-library Python only):
python3 tests/reference/mathtex_parts.py --overlay-only
CGO_ENABLED=0 go test -tags noasm -overlay tests/data/mathtex/parts/overlay.json \
  ./mathtex -run '^TestParts' -count=1 -v

# Normal centralized tests after promotion:
CGO_ENABLED=0 go run -tags noasm ./tests/cmd/run -pkg=mathtex -count=1
CGO_ENABLED=0 go test -tags noasm ./...
CGO_ENABLED=0 go vet -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

Race detection uses the Go toolchain's development-only native race support;
its logs record `CGO_ENABLED=1` explicitly. Production checks use cgo disabled
and `noasm`. Benchmark logs report the six-fragment layout workload, toolchain,
hardware, allocations, and exclusions; they establish no global speed claim.
Validation logs and a path/hash manifest are in `tests/data/mathtex/parts/`.
