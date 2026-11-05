# Bounded curved directed networks

`network.NewCurvedDiGraph(vertices, []CurvedEdge, CurvedOptions)` constructs a
separate directed graph type. The original `NewGraph`, `NewDiGraph`, their
straight-edge defaults and original production files remain unchanged.
The new runtime is pure Go and uses no external graph/layout process.

```go
options := network.DefaultCurvedOptions()
options.Options.Layout.Scale = 3
options.Options.TipSize = .16 // actual path-length trim, not an adaptive maximum
options.TipWidth = .08       // full triangular base width
loop := network.DefaultLoopOptions()
loop.Side, loop.Radius, loop.Clockwise = network.LoopTop, .35, false

g, err := network.NewCurvedDiGraph(
    []string{"a", "b"},
    []network.CurvedEdge{
        {From: "a", To: "b", Sweep: math.Pi / 2},
        {From: "b", To: "a", Sweep: math.Pi / 2},
        {From: "a", To: "a", Loop: &loop},
    }, options,
)
// Add only g.Object to a Scene. Vertex/Edge return borrowed children.
```

There is exactly one edge per ordered pair. Reciprocal pairs are independent;
duplicate ordered pairs, even with different route settings, are rejected.
Vertices must have unique, nonempty IDs. Input order controls adjacency and
rendering order (edge groups first, vertices last). `Edges()` returns ordinary
ordered pairs; `CurvedEdges()` returns copied route records and copied loop
options. Vertex/route ID strings and loop pointer inputs are snapshotted before
layout. `Options` and limits are copied values. There are no route/style callbacks:
callback invocation count and callback work are exactly zero.

`CurvedDiGraph` has `Object`, `Directed`, `Vertices`, `Edges`, `CurvedEdges`,
`Vertex`, `Edge`, `Neighbors`, `SetPositions`, `UpdateEdges`, and `Relayout`.
Its underlying Graph is private, so callers cannot accidentally invoke a legacy
straight-edge updater through embedding. `Neighbors` lists outgoing neighbors
in input order; a self-loop lists its vertex once. Nil/zero graph update methods
return errors, and nil accessors return empty/false. Empty graphs and singleton
vertices without edges are valid. `Directed()` always reports true, including
on a nil receiver, because it describes this graph type.

## Ordinary curves and endpoint clipping

For nonloops, `Sweep` is a finite signed angle in `[-pi,pi]` radians; zero creates
a straight connection. `Loop` must be nil. Positive sweep is counterclockwise
about +Z; an arc from left to right bows below its chord. Giving both reciprocal
edges the same positive sweep separates them on opposite sides. Signs/routes
are explicit; no reciprocal-edge heuristic or random perturbation is installed.

The implementation calls `gomath.NewArcBetweenPoints` between current vertex
bbox centers. Nonzero arcs use its eight tangent cubic approximation segments;
they approximate a circle and are not rational conics. Rectangle-face cubic
roots isolate the source exit and destination entry. The unique exposed interval
is extracted by exact de Casteljau subdivision up to binary64 rounding. The
shaft start and arrow apex consequently lie on the actual current **axis-aligned
vertex bboxes**, not on the original radius or a guessed centerline ray. This
also supports edited/scaled rectangular vertex paths. It does not promise
intersection with the vertex's filled outline: bbox clipping can leave a gap to
a round node's true outline. Stroke thickness and caps are not offset into the
bbox intersection calculation.

Touching/overlapping endpoint bboxes are rejected, even when a wider routing
could go around them. Coincident centers, no unique exposed interval,
nonrepresentable endpoints, collapsed bboxes, disconnected/invalid geometry and
consumed shafts return errors. Ordinary major arcs beyond pi are outside this
API's supported subset. There is no obstacle routing: edges/self-loops can cross
other unrelated vertices and edges. The endpoint certification concerns only
the edge's own source and destination bboxes.

## Explicit self-loop geometry

Self-loops require a non-nil `LoopOptions` and zero `Sweep`:

- `Side`: `LoopTop`, `LoopRight`, `LoopBottom` or `LoopLeft`.
- `Radius`: the positive circular construction radius.
- `Clockwise`: explicit direction about +Z; false is counterclockwise.

On the selected bbox face, the two anchors are symmetrically separated by
`2*min(.75*faceHalfExtent, .5*Radius)`. The circle center is on the outward side
of the face, at perpendicular distance `sqrt(Radius²-spread²)`. The **major arc**
with angle magnitude `2*pi - 2*asin(spread/Radius)` lies outside that face.
Direction selects anchor order and sweep sign. The implementation calls
`gomath.NewCurvedArrow` with these authored endpoints and explicit tip dimensions.
The entire actual cubic shaft and triangular head are then certified against
the endpoint bbox. A too-small loop, very flat/large loop, insufficient anchor
resolution, or an arrowhead that enters the node is rejected. Radius and tips
are never silently shrunk or normalized. A different radius may require an
explicitly narrower tip; there is no promise every finite combination fits.

Circular and Grid layouts ignore edge forces. Spring supports cycles and
reciprocal pairs using the existing deterministic seeded implementation;
self-loops are omitted from force accumulation because they exert no separating
force. Reciprocal entries remain two separately enumerated spring forces.
Resource validation conservatively counts all edges, including loops, in the
layout work allowance. No DAG-only layout or undocumented fallback is used.

## Arrowheads, styles and authoring transactions

`Options.TipSize` is the **physical path length removed** at the destination.
`TipWidth` is full triangle-base width. Zero values are invalid; start with
`DefaultCurvedOptions`. Both are explicit positive dimensions; this API does not
inherit legacy Graph's `min(TipSize, exposedLength*.4)` tip scaling.

The nonloop implementation measures the clipped original cubics and trims the
shaft by arc length. It keeps the parent's `NewCurvedArrow` semantics: the base
midpoint is exactly the trimmed shaft endpoint, the apex is the bbox boundary
point, and the base is perpendicular to the endpoint secant. It retains the
clipped original cubics rather than rebuilding a subtly different circle.
The self-loop directly uses the parent arrow constructor with the same policy.
This is not Manim's endpoint-rescaling/tip API.

`SetPositions` requires exactly one finite planar position for every vertex.
`UpdateEdges` follows direct node moves/scales after authoring, and `Relayout`
computes a deterministic new layout. Each first validates topology, options,
counts and geometry, builds all replacement paths privately, then publishes the
paths in one non-failing commit. A later-edge failure cannot publish earlier
edge/vertex paths. An `UpdateEdges` error preserves the caller's already edited
vertex paths as they stood on entry; it does not undo earlier user authoring.

All successful updates retain the original graph root, edge root, vertex,
shaft and head object references **and IDs**, and all styles/name/scalar/Z-index
values. Styles are not rewritten, including authored opacity, gradients,
dash/cap/join settings. Style coordinates are not re-anchored automatically.
Replacing/reordering root children, replacing shaft/head objects, changing IDs,
adding nested children/bitmap/clip, or adding root/edge-group geometry returns
`ErrCurvedTopology`. A valid source vertex remains a leaf with a finite planar
bbox. Authoring mutations require exclusive access; getters of object references
are borrowed, not a thread-safe mutation API.

Scenes take independent snapshots. Whole-tree animation retains stable identity
and shaft/head joins, and supports concurrent arbitrary-order sampling after
capture. Moving individual vertices in a scene does **not** automatically
recompute edge routing each frame; no mutable graph updater is retained. Call
explicit authoring operations before capture, or animate the entire tree. The
middle-frame tests verify a translated graph against a separately authored pose;
they do not claim nonlinear node-morph edge-following or full Manim graph parity.

## Numerical and resource contract

Dimensions (vertex radius, edge width, tip size/width, loop radius) are finite and
in `[1e-9, 1e12]`. Coordinates/control points and translation deltas are planar
and bounded in magnitude by `gomath.MaxPrimitiveCoordinate` (currently 1e12).
Actual bbox dimensions must also fit the range. With
`epsilon = max(1e-12, 16*ulp(max(1,abs(bbox coordinates))))`, bbox dimensions must
exceed `64*epsilon`, and tip dimensions must exceed `16*epsilon`. These are
explicit precision restrictions; not every finite large-coordinate/small-feature
configuration is representable.

Face roots use derivative-isolated monotone cubic intervals and at most 64
bisections per crossing. Adjacent parameter cuts within 2e-14 are coalesced;
ambiguous/no exposed intervals return errors. Endpoint certification is relative
to the epsilon above, rather than claiming exact real-arithmetic boundary signs.
The actual shaft/triangle curves are checked, not just a few displayed samples.

For nonloops the absolute length tolerance is
`max(1e-12, min(controlPolygonLength*1e-10, TipSize*1e-6))` with optional length
lookup disabled. If floating-point quadrature cannot meet that request, there is
no looser fallback. For loops, the parent's inherited
`max(1e-12, Radius*abs(sweep)*1e-10)` request must be <= `TipSize*1e-6` before its
constructor is called. This prevents a large global tolerance silently swallowing
a short tip. Further bounded parent measurement failures are propagated as
geometry errors. The contract is numerical cubic geometry, not an exact conic or
bitwise equivalence with an external renderer.

`CurvedLimits` has explicit `MaxVertices` (<=4096), `MaxEdges` (<=65536),
`MaxCubics` and `MaxContours` (each <=1,000,000), `MaxWork` (<=10^13) and
`MaxStorageBytes` ([8 MiB, 1 GiB]). Defaults use 512 MiB storage. Zero count/work
limits are meaningful; zero byte limits are invalid rather than defaulted.

The construction/update reserve for `n` vertices, `m` arrows, `C` cubics, `K`
contours and `N` retained name bytes is:

- Modeled bytes: `8 MiB + 2048*n + 4096*m + 1024*(C+K) + N`.
- Work: `4*(n+C+K) + 100,000,000*m`, plus layout reserve (`n` for Circular/Grid,
  or `(n*n+m)*iterations` for Spring).

Construction reserves `C=5*n+11*m`, `K=n+2*m`: every directed edge reserves eight
shaft cubics plus three triangular edges; every cubic stores four controls.
Updates reserve the larger of the current tree and future output: at least the
authored vertex cubics plus `11*m`, and authored vertex contours plus `2*m`.
The original dot reserve is also retained. Point-only contours are counted;
vertex path replacement cannot evade the geometry budget. Names include private
vertex/route identifiers and authored object names (the original vertex ID/name
is conservatively counted twice). Names are checked incrementally before copying.

The work number is a conservative abstract kernel allowance, **not CPU
instructions, milliseconds, or a measured runtime claim**. Each edge allowance
covers fixed-eight-segment arc construction, face-root enumeration,
certification, bounded quadrature/inverse kernels and transient lookup work.
Parent quadrature has <=8191 interval evaluations per integral, 15 speeds per
interval and <=64 inverse iterations; the parent loop lookup is <=65536 knots.
The 8 MiB scratch reserve accounts for per-edge geometry, root lists, measurement
records/lookup arrays and bounded scratch; edge temporaries are processed
sequentially. Vertex/edge maps, indices, transaction output and controls are
included in the aggregate reserves. Counts use checked caps before products or
input-dependent copies, including on 386. No adjacency matrix is allocated.

These limits are modeled owned storage/work, **not RSS, cumulative allocation,
GC scheduling or CPU-time guarantees**. Caller-owned input storage and independent
Scene snapshots/retained frames are excluded; concurrency and retention multiply
live memory. No route callback or unbounded geometric search is installed.

Errors are classified as `ErrCurvedOptions`, `ErrCurvedLimit`,
`ErrCurvedGeometry`, or `ErrCurvedTopology`; constructors return nil/error and
updates expose no partial path set. Underlying length convergence errors are
wrapped where available. Unsupported layout/route subsets are errors.

## Independent evidence and focused checks

Race instrumentation is a development check with `CGO_ENABLED=1`; product
tests/noasm/vet/crossbuilds keep `CGO_ENABLED=0`. This does not introduce a cgo
product implementation or an external runtime fallback.

All new tests, benchmarks, fuzz source/corpora, fixtures and reference tools are
under `tests/`; ordinary tests require no Python/NumPy/SciPy program. No previous
reference fixtures or old network source/tests are modified.

`tests/reference/network_curved_reference.py` independently constructs circles
and tangent controls from circle-center polar coordinates, clips using NumPy
polynomial roots, and locates arrowhead bases with SciPy quadrature and Brent
inversion. It does not call or translate the Go clipping/root solver. The central
JSON fixture contains six signed/straight/diagonal ordinary connections and all
eight side/direction self-loop combinations. Five parameter samples per retained
shaft cubic and all head vertices are compared within 2e-8 at ordinary scales.
This validates the selected mathematical subset; no Manim pixel-parity claim is
made. The Python reference repository remains untouched.

Tests also cover current bbox endpoints, real curvature, reciprocal separation,
head joins, cyclic/loop layouts, input copies, borrowed-node ownership, late-error
atomicity, replaced children/IDs, unsupported/degenerate/range inputs, exact
storage/work/control/contour boundaries, future-output reservations, independent
concurrent construction, interior-frame pixel equality and concurrent/out-of-order
Scene snapshots. Fuzz failures use `tests/data/network/testdata/fuzz/`.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -tags=noasm -count=1 -cover
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=network -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./network
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./network
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -run '^$' -fuzz '^FuzzCurvedAtomicPositions$' -fuzztime=8s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -run '^$' -bench '^BenchmarkCurvedNetwork$' -benchmem -benchtime=100ms -p=1 -count=1
python3 tests/reference/network_curved_reference.py # development only
```

The shared-host benchmark is a bounded observation of 12 circular-layout
vertices, 12 curved cycle arrows and three self-loops, with layout scale 3.
Construction and repeated SetPositions are measured separately. It excludes
rasterization, Scene animation, external references, images and media encoding.
It does not establish that the whole library is faster. Final results and source
hashes are recorded in `tests/data/network/curved_validation.json`; this slice
does not reuse the earlier fifth-wave whole-repository checkpoint as evidence.

Focused validation on November 5, 2026 (Go 1.27.3, Darwin/arm64, Apple M2):

| Check | Observation |
| --- | --- |
| CGO0 ordinary central suite | PASS, 0.062 s, 91.2% package statement coverage |
| CGO0 noasm central suite | PASS, 0.064 s, 91.2% package statement coverage |
| CGO1 race central suite | PASS, 1.271 s |
| Final-source fuzz, two workers | PASS, 12,512 executions in 8 s |
| CGO0 noasm vet / Linux 386 package build | PASS / PASS |
| Independent NumPy/SciPy reference generation and fixture tests | PASS, 14 cases |
| Build, 12 vertices / 15 arrows | 346,048 ns/op, 325,140 B/op, 2,832 allocs/op |
| SetPositions, same graph | 334,199 ns/op, 282,306 B/op, 2,551 allocs/op |

Benchmarks use `-benchtime=100ms -p=1 -count=1`; each reports one short shared-host
sample, not a statistical comparison. The scene test renders 192x144 pixels at
start, interior and end times and verifies the interior image against a
separately authored translated pose. Rasterization is excluded from benchmarks.
This is a focused network checkpoint; whole-repository integration belongs to
the parent task.
