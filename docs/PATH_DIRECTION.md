# Checked cubic path reversal

This batch verifies reversal against Manim Community **0.22.1**, commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. Runtime code is pure Go and uses
only the standard library and existing `geom`/`path` packages. Python and Git
are development reference tools. No old path implementation, Object curve
editing, dependencies, examples or shared status/parity documents are changed.

```go
backwardContour, err := contour.Reversed() // (path.Contour, error)
backwardPath, err := outline.Reversed()    // (path.Path, error)
err = object.ReverseDirection()           // receiver path only
err = object.ReversePoints()              // receiver and all descendants
// Check every error before using the result.
```

## Pinned source semantics

| Manim method | Actual CPU VMobject behavior | Go operation |
| --- | --- | --- |
| `Mobject.reverse_points()` (`mobject.py:1678`) | Visits `family_members_with_points`; reverses every array returned by each node's `get_array_attrs`. VMobject's relevant array is its complete cubic point array. Returns self. | `(*Object).ReversePoints() error`, bounded vector family |
| `VMobject.reverse_direction()` (`vectorized_mobject.py:2136`) | Assigns `self.points[::-1]`; touches only this node, including when it has children. Returns self. | `(*Object).ReverseDirection() error` |
| `VMobject.get_direction()` (`vectorized_mobject.py:2116`) | Calls `shoelace_direction(get_start_anchors())`. Start anchors are `points[::4]`. `space_ops.py:666` integrates their Y values against X using `np.trapezoid`, without appending a closing point; positive means CW, otherwise CCW. | Audited only |
| `VMobject.force_direction()` (`vectorized_mobject.py:2161`) | Accepts only `CW`/`CCW`; reverses this node once if its reported direction differs. Does not recurse or recheck the result. | Deliberately omitted |

Full-array reversal reverses both contour order and curve order. Every cubic
`(P0,P1,P2,P3)` becomes `(P3,P2,P1,P0)`; a contour's new start is its actual
last `P3`. An open contour stays open. No point is projected to XY, flattened,
resampled, fitted, snapped, or moved to a canonical seam. Direct assignments
retain all stored cubic control values, with `reversed.At(t)` describing the
same shape as `original.At(1-t)` up to evaluation rounding. Reversing twice
restores controls and authored metadata; nil versus non-nil empty slices are
preserved by these APIs.

The direction classifier is not a signed cubic-area or arbitrary-plane 3D
orientation calculation. It combines all contours' start anchors in one
sequence. Empty arrays, single points, constant curves and a single straight
cubic report CCW; forcing CW can leave them CCW. A VMobject group with no own
points never orients its children through `force_direction`. The reference
generator records 18 probes plus assertions for the group and straight-line
cases. A Go `ForceDirection` or `GetDirection` API would need its own explicit
contract and numerical validation. This batch provides the verified reversal
operations only.

## Geometry and ownership contract

`Contour.Reversed` and `Path.Reversed` are checked immutable operations. Every
successful result has independent contour/curve storage, including when two
input contours share the same curve slice. Every error returns the zero result
and leaves all input storage untouched. An empty path remains empty. Point-only
contours retain their start and closure flag and move with the reversed contour
order; they do not become constant cubics.

Go's authored contour boundaries are authoritative. No approximate Manim
subpath discovery is run. A standalone single-point VMobject is independently
verified; mixing arbitrary point-only contours between cubic contours is a Go
representation extension, covered by Go invariant tests. Manim's raw mixed
point array can have different grouping behavior and is not a parity claim.

All starts and controls must be finite and componentwise within `[-1e9,1e9]`.
Joins must be exact: `Start == first.P0` and `previous.P3 == next.P0`.
This is stricter than the existing `Path.Validate` tolerance. Coordinates may
have any supported Z value; no planarity is required.

A contour marked `Closed` must end **exactly** at its start if it has curves.
Any nonzero implicit closing edge is rejected, including gaps below `1e-12`.
No implicit edge is materialized and no extra cubic is added. Closing the edge
explicitly before the call is the caller's responsibility. Point-only closed
contours are accepted as degenerate loops. Unclosed contours may have coincident
endpoints or tiny gaps and keep their actual endpoint and `Closed=false`.
Consequently an authored Circle with a floating closing gap must be explicitly
closed before this strict API will accept it.

Object methods keep Object pointers, IDs, names, scalar values, Z order,
child slice storage/order, styles (including gradient/dash pointers), and Clip
pointers. Only selected `Path` fields are replaced. They reject nil receivers,
nil/repeated/cyclic nodes, zero or duplicate IDs, and trees above the shared
bounds. Selected nodes must pass `ValidateVisual`, the stricter reversal geometry
check, and have no Bitmap. These wrappers concern vector paths; other Manim
array subclasses such as point clouds or image arrays are outside the scope.

`ReverseDirection` edits only the receiver. Descendant geometry, visual state
and Bitmap content are not validated or edited; descendant structure and stored
geometry counts still participate in preflight. Thus an empty group can call
`ReverseDirection` successfully without changing any child. `ReversePoints`
validates and reverses all nodes, including point-only and empty ones.

Preflight uses a bounded iterative traversal before staging geometry. Every
selected output is computed before publishing any path. A failure in a later
node leaves exact original path/curve/child slice storage and metadata in place.
Input path storage may alias across distinct Objects: staging separates it on
success. Reusing the same Object node twice is rejected. Ownership outside the
supplied tree cannot be discovered, so callers must still follow Object's
exclusive-tree ownership contract.

Atomicity is failure atomicity for authoring, not synchronization between
writers or readers. Do not edit a tree while the same tree is being sampled or
otherwise written. Independent Object copies can be edited concurrently;
immutable path inputs can be shared by concurrent reversal calls. Completed
Scene authoring can be sampled concurrently into independent snapshots.

## Shared resource budgets

| Bound | Path/contour call | Complete Object tree |
| --- | --- | --- |
| Contours, including point-only | 4096 | 4096 summed over all nodes |
| Explicit cubics | 65536 | 65536 summed over all nodes |
| Work units | `8*curves + 2*contours <= 1048576` | `12*curves + 3*contours + 2*nodes <= 1048576` |
| Accounted storage | `192*curves + 112*contours <= 12582912` bytes | Path formula plus `64*nodes`, same 12582912-byte cap |
| Nodes/depth | N/A | 4096 nodes, 64 levels with root at level 1 |

The four path limits are exported as `MaxDirectionContours`,
`MaxDirectionCurves`, `MaxDirectionWork`, `MaxDirectionStorage`; Object limits
reuse `MaxAnnotationObjects`/`MaxAnnotationDepth`. All counts are checked before
arithmetic that could overflow, geometry scans or output allocation. Complete
tree geometry is conservatively charged for **both** Object operations even
when only the receiver is edited. A family never gets a fresh budget per node.

Storage charges both borrowed and new cubic/contour records using portable
conservative sizes, plus bounded tree staging. It excludes Object allocations
owned by callers, map/allocator overhead, immutable paint/clip/bitmap backing
storage, GC retention and RSS. Work units are deterministic scan/construction
charges, not CPU instruction counts. The other caps bind first with these
constants; there is no independent claim that the work cap is reachable.

A single 65535-cubic contour fits; 65536 cubics exceed the storage cap despite
passing the count cap. In an Object with 65535 cubics, one contour and one node,
only 16 accounted bytes remain. Adding even one empty child exceeds that cap.
Count, storage, node and depth boundaries, combined sibling workloads and early
allocation rejection are tested explicitly.

## Verification and measured scope

`tests/reference/path_direction.py` checks the installed version and Git HEAD,
requires a clean reference checkout, and verifies **143 loaded Manim Python
files** byte-for-byte as `installed == local == pinned Git blob` before creating
numerical results. An assertion also rejects a newly loaded unchecked upstream
module. `reference.json` records the file hashes, generator hash and environment.
Generation uses exclusive output and the final fixture is separately promoted
after checking its results against the first generation.

Fixtures contain eight path cases, six nested-tree operations and 18
force-direction audit probes. Actual linear `Transform.begin/interpolate`
produces 30 path states (20 interior) and 24 tree states (16 interior). The Go
path interpolation and actual Go Scene/Transform are checked against all stored
controls at absolute vector tolerance `1e-10`; static reversal controls match
exactly. Animated cases have matching per-contour curve counts and tree topology.
Automatic unequal-topology alignment, keyed matching, raster pixels and OpenGL
are outside these comparisons.

Tests additionally cover exact controls/endpoints, double reversal, open and
explicitly closed 3D paths, multiple/point-only/constant/empty contours,
non-finite and oversized inputs, exact joins, implicit closure policy, rollback
after staged outputs, pointer/ID/style preservation, aliases, cycles, bounded
trees, shared aggregate budgets, randomized curves, concurrent immutable calls,
independent Object edits and concurrent Scene sampling. Go tests do not launch
Python or any reference process.

Final commands, environment, raw logs and hashes live in
`tests/data/path/direction/validation.json`, `freeze.json` and adjacent logs.
Focused root checks and root race are run serially; no whole-root or whole-root
race suite is launched by this task. `CGO_ENABLED=1` is used only for Go's race
instrumentation. Normal tests, benchmarks, vet and purecheck use CGO disabled
and `noasm`.

Path benchmarks measure checked reversal/allocation for 64, 1024 and 16384
3D cubics split over 1 or 16 contours. Object benchmarks repeatedly reverse a
group of 1, 64 or 1024 children, each containing 16 3D cubics, including preflight,
visual/geometry validation, staging and publication. Input construction and
Python, I/O, rasterization, encoding and Transform sampling are excluded.
Image size is not applicable and rendered frames per operation are zero.
Measurements are Go-only costs, without a claimed speedup over Python/Manim.
