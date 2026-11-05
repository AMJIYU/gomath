# Deterministic physical-time streamline flow

`field.Flow` implements `render.FrameSource` with `Duration() float64` and
`Sample(absoluteDisplayTime) (*gomath.Frame, error)`. It animates visible geometry,
not an index counter or a scalar attached to an otherwise unchanged line. The
runtime is pure Go and retains no field/color callback or integrator.

```go
options := field.DefaultFlowOptions()
options.Duration = 6       // display seconds
options.FlowSpeed = 1.5    // physical seconds / display second
options.TraceTimeWidth = .4 // physical seconds of visible trailing trace
options.StartDelays = []float64{0, .2} // one per line, or nil for all zero
options.Loop = true
flow, err := streamLines.NewFlow(options) // RK4 or AdaptiveStreamLines
// render.RenderScene(ctx, flow, renderConfig, sink)
```

An independently timed source can use
`field.NewFlow([]field.FlowLine{{Times: times, Object: line}}, options)`.
Each source is one open, exactly continuous cubic contour; `Times` has one more
entry than the number of cubics. Its finite timestamps must be strictly
increasing or strictly decreasing. One timestamp and zero cubics is a valid
invisible seed; a timed cubic with identical endpoints is retained. Empty sets
are valid. Nil source objects, nil stream receivers, and sampling a zero-value Flow are
errors; construct Flow through NewFlow or a stream adapter. Default options are explicit; zero-valued limits are not defaulted.

## Time and window contract

For line timestamps `t[i]`, elapsed physical times are
`e[i] = abs(t[i]-t[0])`, and `L = e[last]`. Let `w = TraceTimeWidth`,
`d = StartDelays[line]`, `s = abs(FlowSpeed)`.

At display time `u <= d`, the path is empty. Otherwise the head is
`h = (u-d)*s`, reduced by binary64 modulo `L+w` only when `Loop` is true.
The visible elapsed interval is `[max(0,h-w), min(L,h)]`. An empty interval
produces an empty path, rather than a point cap. Negative `FlowSpeed` reflects
the interval to `[L-hi,L-lo]`: the pulse traverses the source in reverse. A
negative integration clock alone does **not** reverse source traversal; positive
speed follows the recorded point order, even for backwards integration.

Each interval is located by binary search in actual elapsed timestamps. Within
a recorded step, physical time maps linearly to that step's cubic parameter.
The boundaries are trimmed with de Casteljau subdivision, retaining the exact
stored curve shape up to floating-point rounding. Interior segments are copied.
Adaptive accepted times remain nonuniform; no arc-length remapping, new field
evaluations, integration, smoothing or dense ODE output occurs during sampling.

The pulse enters, travels, drains its tail, then restarts. Its per-line loop
period is `(L+w)/s` display seconds, independently of other lines. At an exactly
representable loop boundary the new pulse is empty. No line-tail/line-head
connector is introduced. Without looping, the path becomes empty after draining
and stays empty. Width can exceed the line span; the whole line is then visible
during the middle of the pulse. Zero display duration is valid; samples outside
`[0,Duration]`, including negative/nonfinite values, return errors. No warm-up,
random phases, rate functions, repeated flashes within a window, or end-animation
fade are implicitly added.

RK4's frozen result stores accepted count and total time rather than the original
step-size option. Its adapter reconstructs uniform interior times from
`Time/Steps`, fixing the last timestamp to the recorded `Time`; division and
multiplication can introduce binary64 rounding. Adaptive's adapter uses its
stored timestamps directly. Terminated lines use only their actual accepted
prefix; the requested final integration time is never substituted. Neither
adapter changes any integrator source,
termination reason, point sequence or existing child reference.

## Snapshots and visible-content policy

Construction snapshots the **current authored** paths/styles and clones names
without retaining a larger caller string backing allocation. A shifted/rotated
source keeps its geometry, while the recorded physical time schedule stays the
same. The adapters require the source root still to contain exactly its original
line children, in original order and with the recorded segment counts. Additional
root geometry or children are rejected. Root name, scalar, opacity, style and
Z-index are retained. Numerical point arrays need not match transformed geometry;
shape-changing edits deliberately retain the caller's original time schedule.

Each Flow allocates new identities once. Those identities remain stable across
its frames and differ from source identities. `Sample` creates private object,
child, contour and cubic slices; caller changes to one frame cannot affect later
frames. Authoring must finish before construction; afterward callers may change
sources independently, and arbitrarily ordered/concurrent samples are safe.
Sampling invokes no callbacks. Existing RK4/adaptive construction still owns the
field/color evaluation semantics, including caller responsibility for concurrent
construction callbacks.

Supported source content is finite scalar, finite geometry and a valid solid
stroke style with transparent fill and `Reveal == 1`. Stroke color, width,
opacity, cap, join and miter limit are preserved. Bitmap, clip, nested children,
closed/multiple contours, fill/fill gradient, dash and partial reveal are
explicit `ErrFlowUnsupported` cases; they are not silently discarded or reset.
Exact contour continuity is required. A syntactically invalid path/style is an
`ErrFlowOptions` failure. The source ID is replaced rather than validated as a
frame identity; input object trees are not shared into Flow.

The returned frame contains one group with one child per input line, including
invisible empty-path children, and no camera override. Use renderer configuration
or a caller-owned composition layer for cameras/background overlays. The Flow
itself is immutable and exposes no mutable template root.

## Bounds and numerical errors

`FlowLimits` specifies aggregate `MaxLines`, `MaxSegments`, `MaxStorageBytes`,
and per-call `MaxSampleSegments`, `MaxSampleWork`, `MaxSampleBytes`. Zero counts
are meaningful. Byte reserves must be at least 4096. Hard ceilings are 100,000
lines, 1,000,000 source/output cubics, 100,000,000 sampling work units and 1 GiB
for either byte reserve. Defaults allow 512 MiB reserves and 10,000,000 work
units. No allocation is proportional to display duration, cycle count, or a
hypothetical dense time grid.

Construction first checks counts/byte reserve before copying source geometry.
It reserves `4096 + 1024*lines + 256*sourceCubics + nameBytes`. This includes
private path/time arrays, adapter time arrays, records and conservative scratch;
construction work is bounded linearly by the line/segment caps. Names include
the owning root when an adapter is used. Caller-owned source storage is excluded.

Sampling first reserves
`2*lines + 4*sum(bits.Len(curveCount+1)) + visibleCubics` work units: two visits
per line, worst-case binary-search comparisons for both passes, and one unit per
emitted cubic (a bounded amount of subdivision arithmetic). These units are not
CPU instructions. It preflights the actual intersected segment count and
`4096 + 1024*lines + 128*visibleCubics + nameBytes` bytes before output geometry
allocation. Even invisible frames retain per-line records. Per-call failure
returns nil; no partial frame is exposed.

These are modeled storage/work reserves, **not** RSS, garbage-collector heap,
cumulative allocation or time guarantees. Retained old frames and concurrent
Sample calls multiply live output memory; bound renderer workers/retention
separately. Construction rejects unsupported data before deep-copying it.

All time inputs are finite. An overflow/underflow of positive duration-speed
products, nonrepresentable normalized timestamps, lost positive width in `L+w`,
lost reverse window, underflowed curve fraction, or nonfinite subdivision result
returns `ErrFlowRange`. Loop configurations requiring more than `2^48` cycles
within the active display horizon are rejected, limiting modulo phase-resolution
loss. Binary64 rounding still applies, particularly near boundaries; there is no
claim of exact real-arithmetic clock phase. `ErrFlowLimit` identifies budget
exhaustion and `ErrFlowOptions` malformed inputs. All constructor/sample errors
return nil rather than a partial Flow/frame.

## Pinned Manim comparison and differences

Read-only reference: Manim commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`,
`manim/mobject/vector_field.py` (`StreamLine`, `StreamLines.start_animation`) and
`manim/animation/indication.py` (`ShowPassingFlash._get_bounds`). Manim retains a
per-line duration/time, chooses random positive/negative initial times, advances
`line.time += dt*flow_speed`, subtracts `virtual_time` when crossing the cycle,
and interpolates clipped `line.time/(duration/flow_speed)`. Its flash bounds use
normalized `time_width` and an upper bound `alpha*(1+time_width)`.

This API instead uses absolute deterministic display time, explicit delays,
physical-second widths, signed speed, actual per-step times, independent
`L+w` pulse periods, arbitrary-order sampling, and modulo for multiple cycles.
These policies intentionally differ; **full animated Manim parity is not claimed**.

The development reference executes those exact pinned methods through AST
extraction with minimal animation/container stubs and zero random phase. It
records source SHA256 and actual updater/flash bounds. A narrow matched case
(duration = virtual time = 2, Manim speed = 1, normalized width = .25) matches
Flow speed = 1.25, physical width = .5, loop period = 2. Another test explicitly
checks identical numeric speed parameters do not imply equal clocks. This is a
clock/window comparison, not an independent Manim raster comparison. The source
MIT license is retained under `tests/reference/field_flow_MANIM_LICENSE.txt`.

## Focused validation

All source tests/benchmarks live in `tests/_cases/field/`; no production testdata
or external program is needed by ordinary tests. `flow_reference.json` contains
72 clock samples with 200 cubic point comparisons generated using independent
Python `fractions.Fraction` Bernstein evaluation (not Go subdivision), covering
nonuniform timestamps, reverse physical clocks, signed flow, delay, clipping and
loop/drain endpoints. It also contains the pinned Manim clock probes.

`flow_test.go`, `flow_limits_test.go`, `flow_reference_test.go`, and
`flow_fuzz_test.go` cover malformed/nonfinite inputs, exact storage/work/output
boundaries, unsupported visible content, numerical range, immutable construction,
concurrent/out-of-order sampling, independent per-line clocks/delays, callback
counts, actual adaptive/RK4 paths and
pixel equality against a separately authored physical window. Existing field and
adaptive tests remain unchanged.

Focused commands (from repository root):

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./field
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./field
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -run '^$' -fuzz '^FuzzFlowBoundedWindows$' -fuzztime=10s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -run '^$' -bench '^BenchmarkStreamlineFlow$' -benchmem -benchtime=200ms -p=1 -count=1
python3 tests/reference/field_flow_reference.py # development only
```

Benchmark workload: 128 lines × 256 cubics with quadratic/nonuniform times;
physical width .125, looping, sample at display time .73. Construction and one
Sample are timed separately. Source integration/reference generation and raster
rendering are excluded; there is no image-size/frame-count workload. Samples
allocate independent geometry, so cost scales with visible cubics, while search
cost scales with lines × logarithm of stored segments. Environment and measured
results follow (2026-11-04, Go 1.27.3, darwin/arm64, Apple M2, CGO disabled,
central noasm runner, serial `-p=1`, `-benchtime=200ms`).


| Operation | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Build 128 × 256 | 536,929 | 3,521,160 | 515 |
| Sample window .125 at .73 | 65,418 | 302,505 | 388 |

Final field suite passed in 0.271s with 95.7% statement coverage; race passed in
2.366s. Focused vet and Linux/386 noasm build passed. The final 10-second two-worker
fuzz run executed 493,677 cases without failure. Ordinary tests include independent
reference, actual pixel, concurrency, malformed-input, range and budget checks.
The original five field/RK4/adaptive production-file SHA256 values and original
RK4 test-file hashes match the entry checkpoint. No whole-repository result is
claimed for this slice. Reproduction commands, source/reference hashes and measured
results are in `tests/data/field/flow_validation.json`.
