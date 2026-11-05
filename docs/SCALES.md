# Checked linear and logarithmic coordinates

`graph.Scale` is an immutable increasing **coordinate-to-data** map. Its zero
value is identity. `NewLinearScale(factor)` accepts finite factor>0;
`NewLogScale(base)` accepts finite base>1. The explicit checked API preserves
the existing `Axes` and `NumberLine` linear mapping contracts.

| Scale | `Forward(u)` | `Inverse(x)` |
| --- | --- | --- |
| Identity | u | x |
| Positive linear | factor×u | x/factor |
| Logarithmic | base^u | log(x)/log(base), x>0 |

Calls return errors for nonfinite inputs, overflow, complete underflow to zero,
or a nonpositive logarithmic data value. Fractional/subnormal results remain
ordinary float64 approximations, with no arbitrary-precision or conditioning
guarantee. Negative factors, bases<=1, symlog, power/custom callback scales and
automatic scale selection are not represented.

## Number lines

```go
scale, err := graph.NewLogScale(10)
if err != nil { return err }
line, err := graph.NewScaledNumberLine(
    graph.Range{Min: -2, Max: 3, Step: 1}, // exponent units
    8, scale, graph.DefaultNumberLineOptions(),
)
if err != nil { return err }
point, err := line.Point(100) // data units: exponent2
if err != nil { return err }
value, err := line.Coordinate(point) // approximately100
if err != nil { return err }
_ = value
```

Range and tick spacing use scale coordinates; on the above line ticks carry
data values .01,.1,1,10,100,1000. `DataBounds` returns the converted endpoint
values. `Point(data)` extrapolates within the scale's domain and checks finite
geometry. `Coordinate(point)` projects onto the line, ignoring perpendicular
offsets, then converts to data. Origin/direction and tick/text options are the
same as NumberLine. Nonempty tick values must remain strictly increasing after
conversion; a range collapsing in float64 is rejected.

`Ticks()` returns a fresh slice with actual data values and borrowed tick/label
object references. Optional label formatters receive **data values**, not
exponents. Default labels use shortest round-tripping decimal strings; callers
can format powers or provide a different numeric policy. Existing number-line
limits apply: fewer than10000 tick intervals, at most1000 labels and16384 total
label runes. Source objects/fonts/formatters retain their ordinary concurrency
requirements during construction.

## Axes and plots

`NewScaledAxes(xRange,yRange,width,height,xScale,yScale)` creates actual axes and
ticks in scale coordinates. Use the identity zero `Scale{}` for a linear side
to obtain a semilog plot. A perpendicular axis crosses at coordinate0, meaning
data1 for a logarithmic side. This explicit placement policy is not a claim to
all Manim Axes placement/default semantics.

`Point(x,y)` and `Coordinates(point)` use checked data coordinates. Endpoints
must convert to finite strictly increasing data bounds. `Plot(f,samples)` takes
2..1000000 **uniform X scale-coordinate samples** and passes actual data x to f.
For log X this means geometrically spaced x, rather than uniform data spacing.
The resulting object is a real vector polyline. NaN/Inf function values break
contours; a finite y outside the log domain errors instead of silently deleting
content. Precision loss/overflow/budget errors return no partial object.

Plotting does not detect unsampled discontinuities, certify a curve error bound,
clip against the axes rectangle or perform symbolic transformations. Adaptive
log plots, logarithmic integral-area semantics, minor ticks and automatic labels
are pending. Existing linear adaptive/integral APIs remain separate.

Mapping parameters are fixed at construction. Transforming an exposed Object
does not update this map; set NumberLine origin/direction at construction, and
avoid using transformed geometry with the original mapping. Completed mapping
values may be read concurrently, while ordinary mutable Object authoring still
follows the scene snapshot rules.

## Source-pinned checks

`tests/reference/scales.py` pins Manim CE0.22.1 and commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. Its JSON fixture covers ten
identity/linear/log NumberLine configurations, 90 scalar/point/inverse samples
including extrapolation and perpendicular projection, and60 tick centers.
Go tests enforce provenance/generator hash, scalar tolerance2e-13 relative to
max(1,magnitudes), and point tolerance2e-12. No external runtime is required to
run the Go tests. Separate tests cover scale errors, log-log power-law geometry,
actual function arguments, gaps, resource limits and data-label selection.

This reference excludes fonts, animated mappings, custom callbacks and general
Manim Axes defaults. The runnable `logarithmic` demo renders y=x² as a straight
line in log-log space with a data-mapped moving marker.

The integrated demo also passed independent CairoSVG rendering of the exported
SVG at .5,1.5,3.5 seconds: RGBA MAE0.05541–0.05546/255 and maximum channel
delta17. Record: `artifacts/logarithmic-demo-svg-20261104/provenance.json`;
generator: `tests/reference/demo_svg.py`. This checks exported paint/geometry,
including the moving marker; it shares Go construction and is not a separate
mathematical mapping oracle.
