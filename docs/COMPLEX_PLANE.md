# Complex coordinates and trajectories

`graph.NewComplexPlane(xRange, yRange, width, height, options)` owns a Cartesian
grid and a private three-point affine coordinate frame. It uses the existing
`NumberPlaneOptions` for grid/axis styles and explicit increasing ranges. A grid
centered on each range midpoint is centered on scene origin at construction.
There is no automatic range, equal-unit scaling, label generation or clipping.

```go
c, err := graph.NewComplexPlane(
    graph.Range{Min: -3, Max: 3, Step: 1},
    graph.Range{Min: -3, Max: 3, Step: 1},
    6, 6, graph.DefaultNumberPlaneOptions())
if err != nil { return err }
c.Object.Shift(geom.V(1, -.5))
point, err := c.Point(2 + 1i)
if err != nil { return err }
number, err := c.Number(point)
```

`Point` maps finite complex128 data, including extrapolation. `Number` inverts
the current affine frame; a point off that plane is orthogonally projected onto
it. Unlike the original `Axes` mapping, these methods follow direct whole-Object
translation, rotation, reflection, shear and nonuniform scale. The dual-basis
inverse handles shear; it is not Manim's pair of independent axis projections
for nonorthogonal axes. Both reject invalid, collapsed or nearly collinear
frames: sine angle between axes must exceed 1e-12. Axis magnitudes are scaled
independently, avoiding a false singularity for orthogonal extreme aspect ratios.

Finite input does not guarantee representable intermediate arithmetic or exact
recovery after binary64 rounding. Overflow returns an error. Strong anisotropy
combined with rotation, very large translations, or cancellation can erase
coordinate precision; the API does not provide arbitrary-precision geometry.
The tested orthogonal 1e-300 by 1e300 frame retains both coordinates.

`GridLines` returns borrowed actual children in a fresh slice. Changing the
selection does not replace stored children. Editing an individual grid line
does not redefine the coordinate map. Removing/replacing/editing the private
frame or applying nonlinear whole-tree transformations is unsupported; a missing
frame and collapsed basis are checked. The hidden frame stays inside plot bounds.

`Plot(f, start, end, samples)` samples `func(float64) complex128` at uniform
parameter intervals, with 2..1000000 samples and finite increasing endpoints.
Nonfinite callback values break contours; finite mapped overflow returns an
error and nil object. Unsampled discontinuities need explicit domain splitting.
The returned polyline owns its geometry and is a snapshot of the current map;
later plane transformations do not move it automatically. Callbacks must not
mutate the plane during plotting. Read-only mapping is safe concurrently after
authoring; authoring mutations remain serial.

All tests/fixtures/tools are centralized in `tests/`. The pinned reference
`tests/reference/complexplane.py` runs Manim CE0.22.1 at commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`, normalizes explicit range midpoints
and applies fourteen rigid 3D poses. Seventy n2p mappings and seventy off-plane
p2n projections match within 2e-12. Generator hash is checked. These are explicit
coordinate states, including intermediate poses; they do not establish complete
Manim animation timing, grid defaults, fonts or nonorthogonal inverse parity.
Separate Go tests check shear, reflection, invalid frames/inputs, extreme scales,
plot gaps, independent scene snapshots and concurrent read-only round trips.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=graph -count=1 -vet=all
go run ./tests/cmd/run -pkg=graph -race -count=1 -vet=all
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=graph -run '^$' \
  -bench '^BenchmarkComplexPlaneMapping$' -benchmem -benchtime=200ms \
  -count=3 -p=1 -tags noasm
```

2026-11-04, Apple M2/darwin-arm64/Go1.27.3, CGO0/noasm, one thread:
one checked Point(2+3i) plus one checked Number per operation took
123.0 / 102.4 / 103.1 ns, 0 B and 0 allocations. Grid construction, rendering,
sampling callbacks and I/O are excluded; frame dimensions/count do not apply.
Shared-host load was not controlled and there is no Manim speed comparison.

The `complex-plane` CLI scene renders an actual complex spiral and moves a dot
using sampled tracker values. Its 640x360 SSAA4 PNG was visually inspected.
Three independent exported-SVG/CairoSVG comparisons at .5/1.25/2.8 seconds
passed existing thresholds: RGBA MAE .072177–.072773/255, maximum delta59.
Provenance: `artifacts/complex-plane-demo-svg-20261104/provenance.json`.
This shares Go scene construction and independently rasterizes its exported
geometry; it is separate from the pinned Manim coordinate reference.
