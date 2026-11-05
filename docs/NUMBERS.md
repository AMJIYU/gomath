# Fixed decimal and exact integer objects

`gomath/number` builds editable horizontal vector glyphs with the existing pure-Go
`text` font engine. It supports finite real binary64 values in fixed notation,
exact int64 values, optional signs/comma grouping, and literal text units. It does
not implement Manim's full DecimalNumber, complex numbers, locale formatting,
TeX units, shaping, bidi, font fallback, or automatic updaters. Runtime and tests
do not launch Python, TeX, Cairo, or another process.

```go
opts := number.DefaultOptions() // 2 decimal places, grouping, white, .5 em
opts.IncludeSign = true
opts.Unit = "kg"
n, err := number.New(1234.5, opts) // "+1,234.50 kg"
if err != nil { return err }
n.Object.Shift(geom.V(2, 0))
if err := n.SetValue(-98.125); err != nil { return err }
fmt.Println(n.Text(), n.Value()) // "-98.12 kg", -98.125

i, err := number.NewInteger(9007199254740993, number.DefaultIntegerOptions())
if err != nil { return err }
fmt.Println(i.Text()) // "9,007,199,254,740,993"
```

`DecimalNumber` and `Integer` expose `Object *gomath.Object`, `SetValue`, `Value`,
`Text`, `AnchorPoint`, and `Source`. Pass `.Object` to normal scene APIs. A zero
`Options` is not a default configuration: use the appropriate default function.
`NewInteger` requires `DecimalPlaces == 0`; it rejects `DefaultOptions()` rather
than silently ignoring its two decimal places. Constructors and `At` return nil
plus an error on failure. Nil/uninitialized receiver calls to `SetValue`,
`Source`, `At` and `AnchorPoint` return errors. Nil `Value`/`Text` accessors return
zero/empty text.

## Numeric policy

Precision is explicitly bounded to 0–18 places. Decimal formatting rounds the
represented binary64 input directly to fixed notation using ties to even. There
is no intermediate multiplication by a power of ten or integer conversion.
For example the represented value `2.675` formats as `2.67` at precision 2;
exact `1.125` becomes `1.12`. Binary64 is not exact decimal arithmetic and values
above 2^53 cannot recover integers lost before entering this API. `Integer`
instead retains int64 throughout, including both int64 endpoints.

`Value()` retains the original input bits, including negative zero, even when
`Text()` rounds them. A negative sign is removed only if the **formatted
magnitude** is zero; `IncludeSign` then produces `+0.00`. Nonzero rounded
magnitudes retain their negative sign. NaN/infinities are errors. Every finite
binary64, including subnormals and MaxFloat64, can be formatted without exponent
notation or silent numeric overflow, subject to the selected font/geometry
budgets. Signed values and grouping are applied before the unit is appended.

This zero-sign rule intentionally differs from pinned Manim's independent
`numpy.round` check near negative half-cent boundaries. Four reference records
around binary64 `-0.005` at precision 2 format as `-0.01` here while the pinned
method removes that sign. These are recorded differences, not parity successes.

## Font, unit, baseline and anchor

Nil `Font` selects the existing real embedded Go face according to `Style`
(`text.Regular`, `Bold`, `Italic`, `BoldItalic`). A nonnil `*text.Font` is the
exact supplied face; `Style` must be `Regular`, even when the supplied face is
itself bold/italic. `text.Parse` owns its bytes and fonts are immutable. Missing
glyphs always error, including a digit or minus first needed by a later update.
There is no synthesized face, missing-glyph placeholder, or fallback lookup.

`Unit` is literal single-line UTF-8 text in that same face, preceded by one real
ASCII space when nonempty. Spaces within/trailing the unit are retained. The
limit is 64 runes / 256 bytes. Controls, Unicode format characters, combining
marks and whitespace other than ASCII space are rejected. Other covered runes
are laid out in input order; shaping/bidi semantics are not provided. Unit text
is not parsed as markup or TeX. Kerning is applied across the whole display,
including the numeric/unit boundary and within the unit.

The initial baseline is y=0. `Anchor` selects x=0 in the complete **advance
box**, including spaces and unit: `Right` (default), `Left`, or `Center`.
`AnchorPoint()` returns that baseline anchor after supported object transforms.
It is not an ink-box edge. `Object.Bounds()` continues to report only real
outline ink; no hidden frame/path is inserted to enlarge it. One direct child
slot is retained for every displayed rune, including covered empty-outline
spaces. Text-engine ascent/descent are used to validate the real line metrics;
the number wrapper does not add a second line or expose synthetic ink metrics.

Outlines are laid out once at 1 em and transformed into the current baseline
frame. This preserves source outlines and kerned advances to floating-point
rounding, but does not promise byte-identical coordinates to a separate call of
`text.Layout` at an arbitrary size. Existing `text` and `mathtex` code and old
outputs are unchanged.

## Updates, references and affine edits

`SetValue` validates the current tree and prepares all new formatting, glyphs,
budgets and coordinates before any authoring mutation. Failure retains the
previous value, text, root, children, paths and styles. Temporary layouts may
consume new global Object IDs even on failure; ID allocation is not rolled back.

The root pointer/ID, root metadata/style and surviving child pointers/IDs remain
stable. A glyph slot means an ordinal position **counted from the right of the
whole formatted string**, including punctuation, spaces and unit; it does not
mean a decimal place, digit value or semantic character. Growth adds exactly
the required left slots and inherits their appearance from the old leftmost
slot. Shrink detaches left slots without clearing them. Detached references
retain their last state and are never recycled or reattached by later growth.
Surviving slots retain their per-slot style, ZIndex and Scalar. Unchanged text
updates only the stored numeric value after validating the tree.

Whole-object translation, rotation, reflection, shear, nonuniform scaling and
affine 2D-plane embedding into 3D are supported. The frame is recovered from
three real canonical control points and checked against every current point.
The source basis must have area / distance² >= 1e-12; transformed axis norms
must be at least 1e-12 (with a small floating boundary allowance), and their
normalized cross norm at least 1e-10. Coordinate-consistency tolerance is
128 machine epsilons times the per-coordinate multiply/add and basis scale.
This permits normal floating-point cancellation, not arbitrary outline edits;
extreme transforms may be rejected when the frame cannot be recovered reliably.

Changing identity, child topology, glyph paths independently, introducing a
bitmap/clip, or collapsing the frame causes later `SetValue`/`Source`/anchor
queries to error. Valid style edits are retained. Generic glyph-morphing
`Transform` can render these ordinary Objects, but its changed outlines cannot
generally be interpreted by a subsequent number `SetValue`. Finish numeric
authoring before such a morph, or animate from an immutable source factory.
Authoring mutation and reads are not synchronized; do not mutate an authoring
object concurrently with its methods. Scene/factory snapshots are independent.

## Sampling without mutable callbacks

`Source()` snapshots options, immutable font, current affine frame, root metadata
and each slot's appearance. `DecimalSource.At(float64)` / `IntegerSource.At(int64)`
return fresh independent editable objects and IDs and can run concurrently.
They retain no live author root, mutable path or caller font-input bytes.
`Source` is a factory, not `render.FrameSource`.

An existing tracker can drive an existing dynamic redraw without reading its
mutable authoring value from a callback:

```go
src, err := n.Source()
if err != nil { return err }
tracker, err := gomath.NewValueTracker(0)
if err != nil { return err }
scene := gomath.NewScene()
if err := scene.Add(n.Object, tracker.Object); err != nil { return err }
err = scene.AddDynamicRedraw(n.Object, func(frame *gomath.Frame, _ float64) (*gomath.Object, error) {
    value, err := frame.Value(tracker.ID)
    if err != nil { return nil, err }
    return src.At(value)
}, gomath.DynamicRedrawOptions{MaxObjects: number.MaxGlyphs + 1})
if err != nil { return err }
return scene.Play(2, gomath.ValueTo(tracker, 1000).WithRate(gomath.Linear))
```

Sampled callbacks must use absolute sampled state and return errors. Configure
the redraw's own curve/contour limits as well for unusually complex fonts.
`DynamicRedraw` preserves its root and assigns its own deterministic preorder
child-ID slots; that policy is distinct from authoring's right-counted slots.
Independent `At` calls intentionally have different IDs. A previously captured
Scene or Source is unaffected by later authoring edits.

## Finite work and coordinates

| Quantity | Bound |
| --- | --- |
| Decimal places | 0–18 |
| Unit | 64 runes / 256 bytes |
| Display rune slots | `MaxGlyphs`, default/hard maximum 512 |
| Output cubic curves | `MaxCurves`, default 65,536 / hard maximum 1,048,576 |
| Font input | existing `text.Parse` limit, 64 MiB per font |
| Temporary text work | existing shared `text.MaxGlyphSegments`, 1,048,576 |
| Initial em size | finite [1e-12, 1e6] |
| Canonical metrics/control coordinates | finite, absolute coordinate <= 1e6 |
| World frame/control coordinates | finite, absolute coordinate <= 1e12 |

Zero glyph/curve limits select defaults; invalid limits error. The display
limit is checked before font layout. The text engine checks its segment budget
while loading glyphs; the tighter number curve budget is checked after that
bounded temporary layout. No permanent capacity is reserved for absent digits.
Only currently displayed slots exist; updates temporarily hold old and new
bounded geometry. Underflowed/singular/overflowed geometry is rejected rather
than shown as an empty or clamped result.

## Evidence and commands

Independent development reference `tests/reference/number.py` reads formatting
methods from Manim commit `16ea443e2a5f1d79db0a721161411ce916d6e733` by AST;
`numbers.py` SHA-256 is
`d2d0b1bf1505a14129fec5b72b5612818a9c1d3c6ce3b21fe2f9f1a16452deaa`.
It records 1,376 exact-bit binary64 formats and 48 exact int64 formats, with the
four zero-sign differences explicitly retained. Python repr is not a layout
oracle. Twenty separately specified layouts use fontTools source hmtx/glyf/kern
and manual advance anchors across four licensed real Go font faces, then Cairo
rasterization. This verifies source-font placement and pixels, not complete
Manim/TeX layout equivalence. Fixture provenance and notices are under
`tests/data/number/`.

Geometry is checked within two source design units per em in independent font
comparisons; image thresholds are mask IoU >= .95 and whole-image RGB MAE <= 1
byte on 960×384 canvases. Observed minimum IoU was .983016 and maximum MAE
.024230 bytes. New tests also cover late font failure, exact limits, public
slice/identity edits after update, right-slot detach semantics, affine anchors,
immutable fonts/snapshots, concurrent factory/frame reads, five independent
tracker samples crossing 1,000, and actual Write/Transform intermediate PNG/SVG
paths. They do not modify earlier pixel fixtures.

All test sources, benchmarks, generated previews and reference tools are under
`tests/`. Final command results, workload measurements and source hashes are in
`tests/data/number/validation.json`; this wave-7 evidence is independent of the
wave-6 checkpoint.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=number -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=number -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=number -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./number
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -pkg=mathtex -tags noasm -count=1
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=number -run '^$' -bench '^BenchmarkNumber$' -benchmem -benchtime=300ms -count=3 -p 1
```

The race command enables cgo only for Go's race instrumentation; the package's
normal/noasm build and rendering runtime remain pure Go. Benchmarks observe
bounded authoring/layout work on a shared host; they exclude rendering and
independent reference generation and make no whole-library speed claim.

Observed on Go 1.27.3, darwin/arm64, Apple M2, GOMAXPROCS=8, with three 300 ms
samples per case and package scheduling `-p 1`:

| Case | Workload | ns/op range | B/op range | allocs/op |
| --- | --- | --- | --- | --- |
| New | grouped `-12,345.50`, default real font | 46,189–54,676 | 124,985–124,987 | 251 |
| SetValue | successive `999 + (i%200)/100`, including grouping/width changes | 65,796–67,584 | 86,188–86,202 | 209 |
| SourceAt | immutable factory, `(i%2000)/100` | 21,239–32,409 | 55,468–55,722 | 118–119 |

These are small layout-only observations (no raster size or frame sequence),
with cached embedded fonts, excluding a separate font parsing workload, and
with no isolated-machine throughput or regression claim. Correctness rendering
used the separate 960×384 reference canvases and 800×320 scene previews.
