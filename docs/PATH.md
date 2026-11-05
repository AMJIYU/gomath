# Path length and repeated inverse queries

Paths store cubic Bezier contours. `Length`, `Measure`, `AtLength`,
`TrimByLength` and `Resample` use adaptive Gauss/Kronrod quadrature of analytic
speed and safeguarded Newton/bisection. Tolerances are explicit positive absolute
world-unit budgets; unattainable precision/work limits return errors. Contour
jumps have no length. At a positive-distance boundary the incoming positive
curve wins; stationary endpoint tangents use a one-sided limit.

`Path.Measure(tolerance)` returns an immutable snapshot with owned geometry,
cumulative curve lengths and a bounded optional inverse-query table. Finish
construction once and share it across animation samples. Successful queries
allocate no memory and are safe concurrently.

The table records endpoints of already accepted adaptive quadrature leaves.
A binary search identifies a small parameter bracket; Newton/bisection still
integrates local speed to the requested error budget. Interpolating lookup
entries never substitutes for the numerical inverse. This avoids repeatedly
integrating the entire cubic prefix for every iteration. The same acceleration
is used by distance trims and resampling.

`MeasureWithOptions(tolerance, MeasureOptions{MaxLookupSamples: 0})` disables
tables. `DefaultMeasureOptions` selects 65,536 knots across the whole path;
the hard maximum is 1,048,576. A knot stores two float64s: retained payload is
at most 1 MiB by default, 16 MiB at the hard maximum, plus ordinary curve,
contour and slice metadata. Capacity is bounded as well as length. Growing the
array temporarily retains its preceding allocation until GC. A curve that does
not fit the remaining budget retains the original quadrature/Newton method;
no partial table or accuracy reduction is used. Most ordinary paths need far
less than the default budget. There is no cache file or global mutable state.

## Go-only benchmark — 2026-11-04

Apple M2, darwin/arm64, Go1.27.3, GOMAXPROCS=1, CGO_ENABLED=0, noasm.
Five 300ms samples per case, one package at a time:

| Workload | Tables disabled | Adaptive table |
| --- | ---: | ---: |
| Repeated inverse query on one curved cubic, median | 55.402 µs | 1.840 µs |
| Bytes/allocations per inverse query | 0 / 0 | 0 / 0 |
| Build snapshot of 32 different curved cubics, median | 314.902 µs | 315.930 µs |
| Cumulative bytes/allocations per construction | 10,416 / 3 | 26,288 / 8 |

The query workload uses controls (0,0), (12,25), (-18,-32), (2,1), tolerance
1e-8 and 997 repeating evenly distributed distance fractions, endpoints
included. Snapshot construction is excluded from query timing. Construction
uses 32 connected cubics with controls (i+.2,2), (i+.8,-2), (i+1,0), tolerance
1e-6. It produces no images; Scene sampling, rendering, encoding and I/O are
excluded. Roughly 30.1x is this Go algorithm's observed query ratio, not a Manim
comparison or whole-library speedup. Other agents were developing during this
measurement; CPU contention/system noise were not isolated. No meaningful
construction-time difference is established by these samples. Allocated bytes
are cumulative Go allocations, not peak RSS.

Raw distributions live under `tests/data/path/`. Reproduce with:

```sh
GOMAXPROCS=1 CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -run '^$' \
  -bench '^Benchmark(Inverse|Measure)LookupOptions$' -benchmem \
  -benchtime=300ms -count=5 -p=1
```

Tests compare inverse queries against independent fixed-grid composite Simpson
integration, analytic stationary cubics, retracing curves, contour boundaries,
zero/tiny cache budgets, storage limits and concurrent snapshots. Model error
in cubic approximations of circles is independent of the numerical tolerance.

`TrimByParameter` is a separate exact de Casteljau interval in normalized stored
cubic parameter space. Cubics have equal spans regardless of geometric length;
it preserves contour gaps. `Partial`/Create retain their existing parameter
timing. `MoveAlongPath` uses numerical arc length, which differs from Manim's
approximate length-weighted cubic parameter mapping.
