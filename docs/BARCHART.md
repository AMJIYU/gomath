# Signed bar charts

`graph.NewBarChart(values, yRange, width, height, options)` creates real vector
rectangles, axes and optional category names in pure Go. Bounds and dimensions
are explicit. Y must include zero and every value; there is no automatic range
inference, clipping or hidden normalization.

```go
options := graph.DefaultBarChartOptions()
options.Names = []string{"A", "B", "C"}
chart, err := graph.NewBarChart([]float64{2, -1, 0},
    graph.Range{Min: -3, Max: 4, Step: 1}, 8, 4, options)
if err != nil { return err }
if err = scene.Add(chart.Object); err != nil { return err }
target, err := chart.TargetBars([]float64{-2, 3, 1})
if err != nil { return err }
if err = scene.Play(2, gomath.Transform(chart.BarsObject(), target)); err != nil {
    return err
}
```

Every bar has four canonical straight edges, including zero-height bars. Positive
and negative values grow from the same baseline. Transforming a checked target
therefore interpolates signed values through a zero-height rectangle, retaining
source pointers and IDs. The enclosing scene chooses the easing. This is numeric
value interpolation; it does not reproduce every Manim rectangle contour-matching
choice.

`SetValues` stages all target paths and commits only after every input validates.
It preserves existing styles, bar pointers, IDs and children. `TargetBars`
returns a deep independent target with fresh IDs, retaining current styles and
child content; it never mutates the source. Counts must match exactly. Partial
updates, adding/removing bars, automatic value labels and color recomputation
during updates are not supported.

`BarsObject` is the borrowed group used as an animation target. `Bars` and
`CategoryLabels` return borrowed actual children in fresh selection slices.
Category names use explicit Go-font outlines and fixed ink placement below the
plot, even when values change sign. This differs from Manim's optional labels
above/below its baseline. `NumberLabelOptions.Font`, `Text` and `Gap` configure
names; its numeric `Format` callback is unused because names are literal strings.

`Point(category, value)` returns a checked world point; bar i is centered at
category i+.5. An invisible three-point affine frame lies inside the plot bounds
and moves with the whole chart Object. Subsequent Point/SetValues/TargetBars
calls follow whole-chart translation, rotation, reflection, scale and shear.
Singular/collapsed frames, overflowing coordinates or bar dimensions lost to
floating-point rounding return errors. Directly moving an individual bar does
not redefine the chart mapping; the next update restores that bar to the chart.
Removing/replacing/reordering chart frame or bar children is unsupported and
checked before updates. Arbitrary nonlinear edits to the hidden frame are outside
the mapping contract.

Limits: 1–4096 bars, finite `0 < BarWidth <= 1` in category units, 1–4096 palette
colors, fill opacity 0–1, nonnegative finite stroke width. Palette stops interpolate
evenly over bar indices using the existing Color.Lerp policy. Names must match
the count, be valid visible UTF-8, total at most 16384 runes and 1048576 outline
curves. Existing range/tick/font limits also apply. Invalid construction returns
no partial chart. Authoring is serial; after authoring, concurrent read-only
Point/TargetBars and frozen Scene samples are supported. Callers must not mutate
shared inputs during a call.

All tests/benchmarks/fixtures/reference tools are under `tests/`. The source-pinned
generator `tests/reference/barchart.py` constructs Manim CE 0.22.1 at commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`, calls `change_bar_values` at seven
explicit data interpolation times and applies two rigid poses. The 14 states in
`tests/data/graph/manim-bars-0.22.1.json` verify all four corners of four signed
bars, including zeros/sign crossings, at absolute tolerance 2e-12. Generator
SHA256 is checked. These do not verify fonts, automatic ranges, defaults,
category-label placement or Manim Transform's own path correspondence.

The separate Go regressions exercise affine transforms, zero-preserving animation,
failed-update atomicity, target/snapshot ownership, borrowed selections, concurrent
read-only targets, malformed counts/options and range/precision failures. The
CLI `bar-chart` example draws actual target-value animations, with no reference
program used at runtime.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=graph -pkg=examples/scenes -count=1 -vet=all
go run ./tests/cmd/run -pkg=graph -race -count=1 -vet=all
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=graph -run '^$' \
  -bench '^BenchmarkBarChartTargets$' -benchmem -benchtime=150ms -count=3 -p=1
```

On 2026-11-04, Go1.27.3/darwin-arm64/Apple M2, CGO0/noasm,
GOMAXPROCS1 and serial -p1, three 150ms TargetBars samples measured:

| Bars | µs/op, three samples | B/op | allocs/op |
| --- | --- | ---: | ---: |
| 64 | 57.024 / 56.124 / 56.416 | 101824 | 363 |
| 1024 | 954.431 / 955.219 / 954.899 | 1628672 | 5221 |

This includes whole-chart validation, affine mapping, four-edge construction,
independent deep target copying and target validation. It excludes initial chart
construction, fonts/names, rendering, I/O and external reference generation;
image dimensions/frame count do not apply. Values are fixed sin(i), Y[-1,1],
plot10×5. Shared-host load was not isolated. No Manim speed comparison is made.

`SetValues` stages checked paths directly rather than constructing and discarding
deep-copy targets. At 1024 bars the previous implementation allocated 1628672
bytes/5221 allocations per update; the current implementation uses 782320
bytes/2107 allocations (about 52% fewer bytes and 60% fewer allocations).
The same command with `-bench '^BenchmarkBarChartSetValues$'` measured
621.820 / 619.868 / 621.857 microseconds per update after the full-suite checkpoint.
Earlier before samples were 955.113 / 934.555 / 921.313 microseconds; shared host
load was not controlled, and an earlier after run had a 1094.009 microsecond
outlier. Allocation reduction is the firm result; these samples do not establish
a general time speedup. Initial chart construction, labels, rendering and I/O
are excluded, with the same sin(i) values, ranges and dimensions as above.

An attached affine bitmap follows the same four-edge value update; a zero-area
image remains valid and invisible under the existing renderer contract.
Atomic-failure regression checks stage an earlier valid bar before a later
out-of-range value, ensuring none is committed.

The 640×360 `bar-chart` example was visually inspected. Three SVG export/raster
comparisons at .75/1.25/2.8 seconds passed with RGBA MAE0.04968–0.06274/255,
maximum18–41; provenance is in
`artifacts/bar-chart-demo-svg-20261104/provenance.json`. This compares CairoSVG
interpretation of actual exported geometry; it shares Go scene construction and
is separate from the pinned Manim geometry fixtures. Full graph tests, graph race
and all example renders passed after the addition.
