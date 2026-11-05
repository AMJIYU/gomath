# Polar coordinates and radial plots

`graph.NewPolarPlane(radiusRange, worldRadius, options)` constructs concentric
cubic circles and evenly spaced radial rays. Radius Min must be zero; Max is
the data radius of the outer world circle. Positive Step selects circles, and
the outer boundary is always included, even when Max is not a multiple of Step.
The zero-radius circle is omitted.

```go
options := graph.DefaultPolarPlaneOptions()
options.Rays, options.Offset, options.Clockwise = 12, .25, true
p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 2.5, options)
if err != nil { return err }
point, err := p.Point(2, math.Pi/3)
if err != nil { return err }
radius, angle, err := p.Coordinates(point)
```

Embedded NumberPlaneOptions supplies grid/axis colors and positive stroke widths;
ShowAxes adds two orthogonal full-diameter axes. Rays must be 1..4096 (default20).
Offset is finite radian phase of azimuth zero; Clockwise reverses positive angle.
Both affect grid and mapping. In pinned Manim, these options primarily control
grid/labels, while inherited polar_to_point uses ordinary Cartesian trig.
GoMath deliberately uses one coordinate convention for geometry and data.

Point requires finite nonnegative radius and finite radians; extrapolation beyond
the grid is allowed. Angles reduce before phase addition. Negative radii reject.
Coordinates projects off-plane points into the current affine frame, restores
phase/direction and returns angle in [0,2*pi). Exact authored origin uses zero
angle. Manim returns atan2's signed angle; references compare modulo2*pi and
ignore undefined origin phase.

Whole-Object affine transforms move the frame and grid together, including shear
and out-of-XY-plane rotation. Mapping inherits ComplexPlane's finite arithmetic
and 1e-12 sine-angle condition; nearly collinear frames reject. Binary64 rounding
can lose precision. The hidden reference triangle stays inside the outer circle's
convex hull after affine transforms. Nonlinear edits and private-frame edits or
removal are unsupported. Circles/Rays copy selection slices but borrow actual
children; editing grid lines does not redefine mapping. Circles use four quarter
cubics, not exact conics or identical Manim control points.

Plot(f,start,end,samples) samples a radius callback at 2..1000000 uniformly spaced
finite increasing radian parameters. Nonfinite values break contours; negative
finite radii or mapped overflow return error and nil object. Unsampled
discontinuities need explicit splitting. Output owns a geometry snapshot and
does not follow later plane transforms. Callbacks must not mutate the plane.
Read-only mapping is concurrent-safe after serial authoring.

Tick validation bounds radial intervals below10000. Dimensions, finite diameter,
style and frame precision validate before grid publication. Labels/fraction
formatting, faded subdivisions, partial angular grids, nonlinear reflow and
complete PolarPlane defaults remain pending.

Central Go tests cover circles/rays, nonintegral outer boundary, offset/CW,
affine/off-plane inverse, origin, borrowed selections, plot gaps/ownership,
invalid inputs/frames and bounded counts. The hashed generator
tests/reference/polarplane.py freezes Manim CE0.22.1 at commit
16ea443e2a5f1d79db0a721161411ce916d6e733: fourteen rigid poses, 336 forward
mappings/radius inverses and252 nonzero-radius phases within2e-12. This does not
verify fonts/grid control points, offset/CW parity or full animation timing.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=graph -pkg=examples/scenes -count=1 -vet=all
go run ./tests/cmd/run -pkg=graph -race -count=1 -vet=all
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=graph -run '^$' \
  -bench '^BenchmarkPolarPlaneMapping$' -benchmem -benchtime=200ms \
  -count=3 -p=1 -tags noasm
```

2026-11-04, Apple M2/darwin-arm64/Go1.27.3, CGO0/noasm and one thread:
one Point(2,.7) plus one Coordinates measured139.9/143.7/138.8ns, 0 B/allocations.
Construction, plotting, rendering and I/O excluded; image/frame count inapplicable.
Shared-host load was uncontrolled; no general or Manim speedup claim.

The actual polar-plane CLI scene renders a radial function on CCW and phased CW
grids, moving dots from sampled tracker values. Its640x360 SSAA4 PNG was visually
inspected. Three independent exported-SVG/CairoSVG checks at .5/1.25/2.8seconds
passed existing thresholds: RGBA MAE .110566–.111337/255, maximum34. Provenance:
artifacts/polar-plane-demo-svg-20261104/provenance.json. This shares Go scene
construction and is separate from the pinned coordinate reference.
