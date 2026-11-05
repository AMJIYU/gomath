# Bounded cubic editing

This slice implements checked contour subdivision and anchor handles against
Manim Community **0.22.1**, commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. It does not change `path.Align`,
Object methods, tree alignment, rendering, or animation policy.

```go
contour, err := path.FromAnchors(anchors, path.AnchorSmooth)
// Check err before using contour.
expanded, err := contour.WithCurveCount(17)
edited, err := outline.WithAnchorMode(path.AnchorJagged)
```

`AnchorMode` accepts only `AnchorJagged` (`"jagged"`) and `AnchorSmooth`
(`"smooth"`). All three operations return independent storage and an error on
invalid or unsupported input. No operation mutates its input, including failures
after a later contour or generated handle is checked. Concurrent calls may share
inputs once authoring has finished; concurrent writes to those inputs are not
supported.

`Contour.WithCurveCount(total)` counts **explicit** stored cubics, including
constant cubics. It preserves `Start`, `Closed`, and every original cubic's
shape. It accepts the current count as an independent copy and rejects smaller,
negative, or excessive counts. For original count `n`, output curve index `j`
belongs to original curve `floor(j*n/total)`. Equivalently, original curve `i`
gets `ceil((i+1)*total/n)-ceil(i*total/n)` parts. Each original curve is split at
`k/parts`. This matches the pinned `bezier_remap` allocation used by
`insert_n_curves(total-n)`; neither length-based allocation nor longest-curve
bisection is used.

Go computes allocation products in `int64`. The source constructs its repeat
indices with NumPy's signed 32-bit `"i"` dtype, which can overflow for large
count products. That overflow behavior is not reproduced. Pinned numerical
parity is established for the checked small cases; the large-limit tests are
Go contracts for the intended integer allocation rule.

The cubic blossom constructs each interval directly in constant work, without
flattening, sampling-based fitting, a dense matrix, or successive subdivision
of a shrinking remainder. “Exact” here means a polynomial restriction up to
IEEE-754 rounding, not bitwise agreement with NumPy's matrix multiplication.
Outer endpoints and exact stored joins survive. A point-only contour expands
to the requested number of constant cubics, matching
`insert_n_curves_to_point_list`; Manim's public wrapper also retains a trailing
new-path point, which this contour representation does not store. Zero curves
on a point-only contour preserves the point and closure flag. An implicit
closing edge remains implicit and is not counted.

`FromAnchors` takes one or more anchors and joins each consecutive pair. One
anchor produces a point-only contour, a deliberate Go extension to the
point-array constructor. No closing anchor is appended. Exact repetition of
the first anchor at the end sets `Closed`. Jagged handles lie at one-third and
two-thirds of each segment. Smooth handles solve the pinned open tridiagonal
or closed cyclic system in linear time. The open spline has natural endpoint
second derivatives; exact closed input has matching first and second
derivatives at the seam. Repeated interior anchors, constant curves, 3D
coordinates and the two-curve cyclic special case are supported.

The smooth solver selects the cyclic system using the source's componentwise
test `abs(last-first) <= 1e-8 + 1e-5*abs(first)`. This can classify endpoints as
approximately closed without changing them. The final second handle uses the
actual last anchor, as in the source. Such input keeps its small endpoint gap;
the constructor sets `Closed` only for exact repetition and makes no exact seam
continuity promise for approximate closure. For two anchors the handles are
linear regardless of closure tolerance.

`Path.WithAnchorMode` processes authored contours independently, preserving
their boundaries, starts, explicit anchors, curve counts, and `Closed` flags.
An empty path stays empty and point-only contours survive. It changes only the
handles. A closed contour with an implicit closing edge is rejected in either
mode: explicitly close the contour before requesting handle conversion. The
operation neither adds a closing curve nor guesses a smoothing seam. Stored
contours are authoritative; this method does not rediscover Manim subpaths
using approximate point equality or recurse through an Object family.

| Bound | Checked rule |
| --- | --- |
| Coordinates | Every input and generated start/control component finite and within `[-1e9,1e9]` |
| Connectivity | `Start == first.P0` and each `previous.P3 == next.P0`, exact component equality |
| Curves | At most `65536` input and output cubics per call, summed across a path |
| Contours | At most `4096` for path-wide handle conversion |
| Work | At most `1048576` conservative units, preflighted before output allocation |
| Storage | At most `12582912` conservative accounted bytes, preflighted before output allocation |

Insertion charges `4*inputCurves + 8*outputCurves + 1` work units.
Construction charges `anchorCount + 4*curves` for jagged or
`anchorCount + 24*curves` for smooth. Path conversion charges
`4*inputCurves + contourCount` plus the same mode-dependent curve charge.
These fixed units bound linear scans, construction and solver work; they are
not CPU instruction counts. For example, smooth construction permits `41943`
curves exactly at the work budget and rejects `41944`.

Storage accounting is `96*(inputCurves+outputCurves) + 56*contourCount +
40*(scratchCurves+1)` bytes. Scratch is zero for insertion, the output count
for construction, and the largest contour count for path conversion. It
conservatively charges borrowed inputs and worst-case anchor/solver arrays;
it is a bound on accounted live geometry, not total process RSS, allocator
rounding, or uncollected garbage. Other bounds may become binding first. In
particular, cloning an existing `65536`-curve contour exceeds this conservative
storage cap, while expanding a point to `65536` curves succeeds. Smooth handles
can overshoot the coordinate range even with valid anchors; that returns an
error with no partial result.

The exact-join requirement is intentionally stricter than the existing
`Path.Validate` tolerance. Implicit closing edges are accepted for subdivision
but rejected for handle conversion. Curve removal, resampling by length,
unbounded workloads, Catmull-Rom smoothing, arbitrary spline boundary
conditions, Object wrappers and a replacement for automatic Transform
alignment are outside this slice.

`tests/reference/curve_edit.py` uses an already installed Manim environment and
verifies six installed source files byte-for-byte against the pinned Git
blobs and local checkout before generating exclusive fixtures. The checked
JSON records 24 anchor cases, 18 insertion cases and 20 actual linear Transform
states, including 12 interior states. It covers unequal original counts,
nonuniform lengths, open/closed/3D/constant anchors, repeated anchors, closure
tolerances, jagged reconstruction and both small cyclic/open solver cases.
Single-anchor point metadata is a Go contract test, not claimed Python parity.

The Go Transform reference test explicitly prepares both contours to the same
count with `WithCurveCount` before calling the existing Transform. It matches
all cubic controls at absolute tolerance `1e-10`, and checks source identity.
It does not establish automatic unequal-topology alignment or pixel parity.
Go-only invariant tests cover shape preservation at interior parameters,
first/second derivative continuity, natural endpoints, strict errors, exact
limit boundaries, atomicity, aliasing, deterministic repeated and concurrent
calls, and randomized subdivision. No test invokes Python or a reference
subprocess.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -run '^TestCurveEdit' -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=path -run '^TestCurveEdit' -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -run '^$' -fuzz '^FuzzCurveEditSubdivision$' -fuzztime=15s -parallel=4
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -run '^$' -bench '^BenchmarkCurveEdit$' -benchmem -benchtime=200ms -count=3
CGO_ENABLED=0 go test -tags noasm ./...
CGO_ENABLED=0 go vet -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

The race detector alone uses CGO as Go development instrumentation. Production,
normal tests and benchmarks use `CGO_ENABLED=0` and `noasm`. There are no new
dependencies, native bindings, runtime subprocesses, commits, or changes to
existing path code. Manim MIT notices are retained in the handle source and the
existing `LICENSES/manim-LICENSE*.txt` files. File hashes, staged and promoted
validation, benchmark environment and scope are recorded in
`tests/data/path/curve-edit/validation.json`. Shared status/parity docs and
Object/example integration belong to the parent change.
