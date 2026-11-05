# Legacy Graph / DiGraph authoring transactions

`network.Graph` and the embedded Graph in `network.DiGraph` support bounded,
explicit batch topology changes. Runtime implementation is pure Go, with no
new dependencies or external layout/geometry process.

```go
g, err := network.NewDiGraph(nil, nil, network.DefaultOptions())
// Check err after each operation in real code.
err = g.AddVertices([]network.VertexSpec{
    {ID: "left", Position: geom.V(-2, 0)},
    {ID: "right", Position: geom.V(2, 0)},
})
err = g.AddEdges([]network.Edge{{From: "left", To: "right"}})
err = g.RemoveEdges([]network.Edge{{From: "left", To: "right"}})
err = g.RemoveVertices([]string{"right"})
```

## API and ordering

The new methods return `error`; newly published objects are available through
`Vertex` / `Edge` after success. There is no partial-result object list on error.

- `AddVertices([]VertexSpec) error`: append vertices in batch order.
  `VertexSpec` contains `ID string`, explicit `Position geom.Vec3` and optional
  `Style *gomath.Style`. New vertices are dots with the original constructor's
  radius. A nil Style uses its original vertex fill and the existing Dot policy
  (transparent zero-width stroke). A non-nil **complete** Style is copied by
  value. Gradient/dash pointers refer to immutable constructor results.
- `AddEdges([]Edge) error`: both endpoints must already exist and differ.
  Append unique edges in batch order, using the original edge color/width and
  original directed/tip policy. No implicit vertex creation or layout occurs.
- `RemoveVertices([]string) error`: remove all requested vertices and **all
  incident edges**, whether incoming or outgoing. Survivor vertex and edge
  order is unchanged. Retained borrowed references to removed nodes remain
  usable ordinary objects; they are absent from the graph's tree and indices.
- `RemoveEdges([]Edge) error`: remove the requested keys in one transaction.
  For an undirected Graph, `(a,b)` and `(b,a)` identify the same edge. Metadata
  retains the originally authored orientation; lookup canonicalization does
  not reorder `Edges()`. DiGraph reciprocal keys are distinct.

Empty/nil batches are successful no-ops **after validating current graph state**.
An empty constructed graph is valid. Unknown items, empty/duplicate vertex IDs,
repeated removal requests, duplicate edge keys and self-loops reject the whole
batch. Duplicate undirected requests include reversed pairs. Nil/zero `*Graph`
authoring operations return an error; construct Graph/DiGraph with a constructor.
The methods do not add topology changes to the private `CurvedDiGraph` wrapper.

Root children keep the legacy **all edges, then all vertices** ordering. New
items append within their category. `Vertices`, `Edges`, `Neighbors` and all
private maps are updated together; Neighbors retains surviving input edge order.
Strings and incoming slices are snapshotted; metadata accessors return copies.
Inputs and authoring objects must not be concurrently mutated during a call.

## Geometry and identity

Existing root, vertex, edge, shaft and head pointers and IDs survive successful
transactions. Existing paths, names, scalar/Z-index values and complete styles
also survive, except for the root's child slice and intentional removals. New
objects are built privately and published only after the whole batch succeeds.
The implementation does not reconstruct or replace the old object tree. A
temporary metadata record reuses the existing line/arrow geometry kernel for
**new** links only. Failed calls may consume internal object ID numbers; they
never publish those objects and do not promise gap-free global IDs.

New links retain the original geometry policy: center-to-center straight lines,
clipped to endpoint **axis-aligned bboxes**, not circular filled outlines. For
directed links, physical tip trim is
`min(original TipSize, exposedLength*.4)` and full triangle width is `.8*trim`.
No implicit edge color/style inheritance from an edited neighbor is installed.
The original constructor's values define new-object defaults. Mutations do not
reroute surviving links after independent vertex edits; call `UpdateEdges`
explicitly. `SetPositions`, `UpdateEdges` and `Relayout` keep their original
geometry arithmetic and now share bounded state/identity preflight checks.

Root and directed-edge groups must retain empty paths; vertices and undirected
links remain leaves; a directed link retains its original two leaf children.
Replaced roots/children, changed IDs, reordering, nested children, bitmap/clip
content and cycles are rejected before generic tree traversal. Every current
object's style, scalar, path and ownership is validated, including objects a
transaction intends to remove. A remove operation does not repair an invalid
authoring tree. Standalone objects can be edited outside the graph API, but the
next operation validates the state present at entry; it never rolls back earlier
caller edits.

IDs must be nonempty UTF-8 strings without NUL. Authored object names can be
empty, but obey the same encoding/no-NUL and byte-budget constraints. Positions
and controls must be finite planar values with each coordinate and translation
delta within `gomath.MaxPrimitiveCoordinate` (currently 1e12). Constructor radius,
edge width and tip size are positive finite values <= that bound. Stroke width
and conservative stroke extent must also fit the bound. These are tightened
malformed/range contracts; ordinary old defaults, paths and pixel tests remain
unchanged.

A vertex bbox must have positive dimensions exceeding
`16*ulp(max(abs(all XY bbox coordinates)))`. This scales with actual coordinates,
not an arbitrary scene-unit epsilon, allowing representable tiny graphs. Touching
or overlapping connected endpoints, invisible/collapsed tips, disconnected paths
and nonrepresentable geometry are errors. Isolated vertices may overlap because
there is no edge geometry to expose. No obstacle routing, self-loops, parallel
edges, custom vertex factories or callbacks are added by these APIs.

## Explicit resource budgets

`NewGraphWithLimits(vertices, edges, Options, GraphLimits)` and
`NewDiGraphWithLimits(...)` retain copied limits for construction and subsequent
operations. Existing `NewGraph/NewDiGraph` use `DefaultGraphLimits()`; their
signatures and `Options` fields are unchanged. There is no mutable limit setter.

| Field | Default | Valid hard ceiling |
| --- | --- | --- |
| MaxVertices | 4096 | 4096 |
| MaxEdges | 65536 | 65536 |
| MaxCubics / MaxContours | 1,000,000 each | 1,000,000 each |
| MaxNameBytes | 16 MiB | 64 MiB |
| MaxStorageBytes | 512 MiB | 1 GiB; minimum 64 KiB |
| MaxWork | 1,000,000,000 | 1,000,000,000 |

Zero count/name/work budgets are meaningful; zero storage is invalid. Joint
limits can constrain a graph before any individual ceiling is reached. Even an
empty graph reserves 67,584 bytes and 128 abstract work units. Layout and mutation
work may require more than merely retaining an already valid graph; for example,
a removal can fail a very tight work budget without changing anything.

Let `n,m` denote target vertices/edges, `C,K` reserved cubics/contours, `N` all
retained private vertex/edge ID bytes plus object-name bytes, `O=1+n+m` for Graph
or `O=1+n+3*m` for DiGraph, and `P` immutable paint reserves. Before transaction
allocations the implementation checks:

```
modeled bytes = 65536 + 4096*(n+m) + 2048*O + 1024*(C+K) + 4*N + P
modeled work  = 128*(1+n+m) + 16*(C+K) + N + extraWork
```

Construction reserves five cubics and one contour per dot, one cubic/contour per
undirected link, or four cubics/two contours per directed link. Preflight counts
all current authored geometry, including point-only contours, and retains the
larger current/future reserve. Future output includes at least five cubics/one
contour per vertex, or the actual authored vertex geometry when larger, plus
every normal link/arrow. Empty authored arrow paths cannot hide later output.
Add batches extend these reserves; removal validates and reserves the input
state before constructing survivors. Two metadata generations, map/index slots,
identity records, candidate children, path outputs and fixed scratch are covered
by the coefficients. There is no dense adjacency allocation.

Each gradient pointer is charged 16,384 bytes and each dash pointer 8,192 bytes
per object/spec, conservatively at public maximum dimensions even when shared.
Ordinary current-tree validation runs only after bounded shallow topology/count
checks. Batch capacities and target-slot reserves are checked before allocating
duplicate maps. Name byte/work bounds precede UTF-8 scanning or request lookup.
Removal request names have a separate capped byte sum and add that sum to work.

`extraWork` is `128*batchLength` for additions, `128*batchLength+requestNameBytes`
for removals, `n` for Circular/Grid layout, or `(n*n+m)*iterations` for Spring.
The original Spring cap of 20 million pair/edge iterations still applies.
Counts are capped before products; all computed values fit on 386/uint64. There
is no user callback or unbounded geometric search.

These are conservative modeled payload/work reserves, **not RSS, cumulative
allocations, GC scheduling or CPU-time guarantees**. Caller-owned input backing
storage, retained removed objects and independent Scene snapshots/frames are
excluded; independent graphs/calls multiply live storage. Individual-operation
work is O(vertices + edges + authored geometry + name bytes + batch size), plus
the separately bounded layout work. There is no context/cancellation API in
this bounded synchronous authoring subset.

New mutation errors use `ErrGraphInput`, `ErrGraphLimit`, `ErrGraphTopology` and
`ErrGraphGeometry`; use `errors.Is`. Existing constructor/layout/geometry errors
can also be returned by legacy operations. Failure returns no new node pointers
and changes neither metadata nor any current object field/path/style.

## Scene ownership and snapshots

Vertex/Edge objects are borrowed descendants; add only `g.Object` as a Scene
root, or use `Object.Copy()` for an independent renderable tree. Completed
`Scene.Add/Play` captures independent snapshots. Later topology mutation does
not modify already captured frames or existing animation tracks, and no new
vertex/edge silently enters an old animation. Explicitly capture a new scene or
operation after authoring. Scene authoring is not concurrent with sampling;
independent captured-scene sampling may run concurrently with modifications to
the original graph because it uses its private snapshots. Graph mutation itself
requires exclusive authoring access; read-only metadata access is concurrent-safe
only when that same graph is not being mutated.

## Actual pinned reference and checks

`tests/reference/network_mutation_reference.py` calls actual Graph/DiGraph
methods from read-only `../manim` at commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. Bytecode writes are disabled. It emits
`tests/data/network/mutation_manim.json` with the source SHA-256/version and ten
valid operation states across the two graph types. Tests compare stable metadata
ordering, positions, selected axis-aligned start/end geometry and source cleanup
of layout/vertex/edge dictionaries. Source root/survivor identities and absence
of deleted descendants are recorded. Ordinary tests read static JSON without
starting Python. Provenance/license notes are in `mutation_reference.md`.

The correspondence is deliberately limited:

- Source Graph requires an explicit constant `buff=.12` to match Go's bbox
  clipping for the selected horizontal/vertical center pairs. This does not
  establish arbitrary shape/direction equivalence.
- Source appends submobjects in call order; Go keeps its legacy edge-first tree.
- Source can auto-create edge endpoints and partially apply invalid batches;
  Go requires explicit vertices and rejects invalid batches atomically.
- The pinned generic `DiGraph.add_edges` creates new Line instances without
  adding tips. Go retains its directed-arrow policy on new links. Matching
  endpoints does not claim matching directed tip geometry.
- No background updater, implicit mutation animation or full Manim parity is
  claimed. Captured Go timelines deliberately retain their old topology.

Tests also cover late-error rollback, surviving object paths/styles/identity,
undirected reverse lookup, directed reciprocals, Unicode/copied inputs,
nil/empty batches, malformed identities/topology/styles, exact resource
boundaries, future-output reserves, tiny/extreme geometry, independent concurrent
construction and captured-scene concurrent sampling. A 192x144 interior frame
is pixel-identical before and after later authoring mutations; a fresh capture
sees the changed topology. Old tests, curved production and old fixtures remain
unchanged. Fuzz failure corpora stay under `tests/data/network/testdata/fuzz/`.

Staged production was compiled with the central test overlay before promotion;
the optional development helper is `tests/reference/network_mutation_overlay.py`.
Delivered source uses the normal central runner:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -count=1 -cover
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=network -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./network
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./network
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -run '^$' -fuzz '^FuzzGraphMutationTransactions$' -fuzztime=8s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=network -run '^$' -bench '^BenchmarkGraphMutation$' -benchmem -benchtime=100ms -p=1 -count=1
PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/network_mutation_reference.py
```

Race is development instrumentation with CGO1; product/noasm checks use CGO0.
Benchmarks measure a 16-vertex/16-directed-edge circular graph at scale 3, plus a
round trip of four batches adding eight vertices/eight edges and removing them.
They exclude Scene capture, rasterization, reference Python, images and media.
They are bounded short shared-host observations, not whole-library speed claims.
Final observations, commands and delivered/protected source hashes are recorded
in `tests/data/network/mutation_validation.json`. This is a seventh-wave focused
checkpoint and does not reuse the sixth-wave whole-repository checkpoint.

Focused observations on November 5, 2026, Go 1.27.3, Darwin/arm64, Apple M2:

| Check | Result |
| --- | --- |
| Final CGO0 central network suite (runner adds noasm) | PASS, 0.110 s, 90.9% package statement coverage |
| Final CGO1 central race suite | PASS, 1.300 s |
| Staged final-production-byte fuzz, two workers / 8 s | PASS, 55,941 executions |
| CGO0 noasm vet / Linux 386 package build | PASS / PASS |
| Actual pinned Manim fixture and ordinary reference test | PASS, 10 states / 20 edge endpoint pairs |
| Construct 16 vertices / 16 directed edges | 60,420 ns/op, 118,690 B/op, 697 allocs/op |
| Four mutation batches, eight temporary vertices / eight edges | 136,354 ns/op, 110,296 B/op, 482 allocs/op |

Each benchmark uses one short `-benchtime=100ms -p=1 -count=1` sample; the second
row measures the entire four-operation round trip, not one edge insertion.
The fuzzed production bytes were promoted unchanged. The focused package suite
also exercises the original Graph and curved-network tests. Whole-repository
integration and media acceptance remain the parent task's responsibility.
