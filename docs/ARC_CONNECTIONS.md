# Connections between two points

`NewArcBetweenPoints(start,end,options)` owns a checked open path with eight
tangent cubic segments and exact authored endpoints. Its `ArcConnection.Object`
can be styled and added to ordinary scenes. Start/End/Center/Normal/Radius/Sweep
are construction metadata, without automatic recomputation during later edits
or animation. Zero options create a line; `DefaultArcBetweenPointsOptions()`
selects a counterclockwise π/2 sweep. Angles are radians, |sweep|<2π.

A nonzero signed Radius overrides finite Angle with the minor arc of that
absolute radius, which must be at least half the chord length. Negative radius
selects clockwise. Otherwise Angle selects positive counterclockwise or negative
clockwise, including reflex sweeps. Radius is derived from chord/(2|sin(sweep/2)|).
There is no complete-turn arc with distinct endpoints or automatic radius clamp.

Zero Normal means +Z. A nonzero explicit normal must be finite, normalizable and
perpendicular to the chord (normalized dot tolerance1e-12). Its direction sets
counterclockwise orientation. The implementation projects this nearly perpendicular
direction to the exact chord plane. Arbitrary 3D chords therefore need an explicit
compatible normal. Zero-sweep lines do not need a circle plane; arrowheads do.
The existing 2D raster still projects world geometry without depth occlusion.

Coordinates, output controls, circle center and radius are bounded by1e12;
chord lengths must be at least1e-9, and nonzero radii at least1e-9. Invalid
or excessively large derived geometry returns nil/error. Explicit normal magnitude
can be subnormal; component division avoids an overflowing normalization reciprocal.

The construction uses chord-frame trigonometric identities. In particular, the
normal displacement is computed from products of small sines, without subtracting
large almost-equal circle-center coordinates. A1e-12 sweep across a unit chord
retains its1.25e-13 midpoint bulge. Cubics approximate the circle; this is not an
exact rational-conic representation.

## Curved arrows

`NewCurvedArrow` returns an owning group containing the shaft and one filled
triangle; `NewCurvedDoubleArrow` or Options.Double adds the start triangle.
Default tip length/full width are.25/.2 world units. `TipLength` is the actual
cubic arc distance removed from each end, not the triangle's Euclidean height.
The base center is the trimmed shaft endpoint, and the head joins it to the
exact authored tip; the perpendicular base is in the arc plane. This avoids
a detached head and keeps all children usable through normal scene transforms.
TipWidth is the full base width. A shaft consumed entirely by tips is an error.

Construction uses the existing bounded arc-length integrator/inverse and de
Casteljau trimming, with absolute tolerance max(1e-12,estimatedLength×1e-10).
Its convergence/range errors propagate; there is no straight-line fallback.
Filled tips have the ordinary white fill and transparent stroke, and callers
can SetColor the owning group. This explicit trimming/secant geometry differs
from Manim's endpoint-rescaling/tip placement API; custom tip shapes and exact
Manim tip defaults remain pending.

## Tangential fillets

`NewTangentialArc(first,second,TangentialArcOptions{Radius:r,Corner:q})`
constructs a circular fillet tangent to the selected rays of two infinite XY
lines. Radius is explicitly positive; zero Corner means {1,1}, otherwise each
entry must be -1 or 1. Input objects follow NewAngle's single straight-cubic
contract. The unique intersection and selected ray directions determine tangent
points at distance r/tan(cornerAngle/2) from the intersection. Those points may
extend beyond the finite input segments. Parallel/collinear or ill-conditioned
intersections reject rather than produce an empty arc. The chosen endpoint order
matches pinned Manim's positive minor sweep. Inputs are read once and not retained.
Construction metadata and coordinate/radius bounds follow ArcConnection.

## Evidence

`tests/reference/arc_connections.py` pins Manim CE0.22.1 commit
16ea443e2a5f1d79db0a721161411ce916d6e733.33 sweep/radius/line constructions
produce231 linear Transform states, including animation interiors. Every cubic
control agrees within2e-11 for XY chords (including a constant nonzero Z).
Sixteen additional TangentialArc constructions, including all four corners,
two radii and oblique lines, produce112 checked linear Transform states with
the same control-point tolerance.
The source generator hash is checked in ordinary Go tests. These are control
geometry comparisons, not complete API, arbitrary3D defaults, raster or tip parity.

Separate tests check circle/plane invariants, tiny bulges, explicit signed radius,
subnormal normal scaling, invalid/oversized inputs, actual arrow base/shaft joins,
stable child pointers and independent sampled frames. Tests, benchmarks and
fixtures all live in tests/. The `arc-connections` CLI example displays signed
and reflex arcs, one/two arrowheads and an explicit negative radius. The
`tangential-arcs` example shows selected corners and an oblique fillet,
with supporting lines and tangent points transformed together. Analytical
fillet tests independently check center-to-line distance, selected rays and
endpoint tangents, with invalid/parallel inputs rejected.

Constructor-only observations on Go1.27.3/darwin-arm64 Apple M2, CGO0/noasm,
GOMAXPROCS1:0.933–0.942µs,1968B/5alloc per eight-cubic ArcBetweenPoints.
Input options/endpoints are prebuilt; this excludes arrowheads, animation,
render and I/O. Raw command/results are in
tests/data/root/arc-connections-benchmark-20261104.txt. Shared-host observations
do not establish a Manim comparison or whole-library speed advantage.
