# Adaptive planar streamlines

`field.IntegrateAdaptive` and `field.NewAdaptiveStreamLines` add explicit
Dormand–Prince RK45 integration alongside the existing fixed-step RK4 API.
The existing RK4 source, tests, point/time contract and fixtures are unchanged.
This is a numerical/geometry feature, not a claim of Manim adaptive parity.
There is no runtime dependency on SciPy, Python, a native solver or a process.

```go
o := field.DefaultAdaptiveOptions()
o.StartTime, o.EndTime = 0, 6
o.AbsoluteTolerance, o.RelativeTolerance = 1e-10, 1e-8
rotation := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
r, err := field.IntegrateAdaptive(ctx, rotation, geom.V(1, 0), o)
// On success, inspect r.Reason(): only TimeComplete means the end was reached.
// r.Times() and r.Points() return independent copies with matching indices.

style := field.DefaultAdaptiveStreamOptions()
style.Integration = o
streams, err := field.NewAdaptiveStreamLines(ctx, rotation, seeds, style)
// Add streams.Object to the scene; Lines()[i].Object is a borrowed child.
```

## Numerical and result contracts

The field is autonomous and planar: `x'=fn(x)`, with no velocity normalization.
`Function` takes an XY `geom.Vec3` and returns XY velocity; nonzero Z is unsupported.
Both endpoint times and their difference must be finite. Positive `InitialStep`,
`MinStep`, and `MaxStep` are magnitudes with `MinStep <= InitialStep <= MaxStep`.
The sign of `EndTime-StartTime` determines integration direction. No zero option
is silently replaced with a default; use the explicit default constructors.

`AdaptiveResult` has private state and copy-returning `Times()` / `Points()`.
These slices contain only accepted steps, including the initial time/seed.
`Time()` is the last accepted **absolute time**; it is not a nominal step times a
count. Accepted times are strictly monotone in the chosen direction and generally
nonuniform. `Stats()` returns actual `AcceptedSteps`, `RejectedSteps`, and
`Evaluations`. A rejection is a completed embedded-error trial with normalized
error greater than one; a stage interrupted by domain/invalid/stagnation checks
is not a completed rejected trial. Field calls returning invalid values still
count. `Stop()` returns the failed stage's time/point, or the terminal endpoint.
The stop point may be outside the domain or nonfinite and is diagnostic only;
it is never appended to accepted geometry. Accessors on nil results return zeros.

The fifth-order solution uses the Dormand–Prince seven-stage tableau and the
embedded fourth-order estimate. Error control uses

```
scale_i = atol + rtol * max(abs(point_i), abs(candidate_i))
error = sqrt((error_x/scale_x)^2/2 + (error_y/scale_y)^2/2)
```

Tolerances must be finite, nonnegative and not both zero. A zero component scale
contributes zero if its embedded error is zero; otherwise it forces rejection.
Acceptance requires estimated normalized error <= 1. This is **local embedded
error control**, not a bound on global error, forward error or geometric
interpolation error. No tolerance floor is imposed. The controller has safety
factor 0.9, fifth-root scaling, shrink floor 0.2 and growth cap 10. Growth is capped
at one immediately after a rejection. Accepted endpoint derivatives are reused
by FSAL; a fully completed sequence has `1+6*(accepted+rejected)` field calls.
Termination during a trial can have fewer calls.

Rational tableau coefficients use common integer denominators. Numerators are
summed using a fixed 6400-bit standard-library `math/big.Float` scratch path,
then divided only after cancellation and rounded to a binary64 coordinate.
This avoids premature overflow in `weight*velocity` or `h*velocity`, and preserves
exact constant-field cancellation in the embedded estimate. It is an independent
implementation of mathematical coefficients, without copied SciPy source.
The precision covers binary64 inputs and the fixed integer weights over their
full exponent span; it does not grow with problem history. Error normalization
also uses extended-range scratch, so finite tolerance scales are not silently
turned into binary64 infinity. Nonzero coordinate underflow to zero, coordinate
overflow and nonfinite coordinates terminate as `InvalidPoint`. Tiny normalized
errors may round to zero and very large ones to infinity for controller scaling;
those cases still lie unambiguously below/above the acceptance threshold.

## Explicit termination and boundaries

A nonnil result with `Reason()` reports an accepted prefix:

| Reason | Meaning |
|---|---|
| `TimeComplete` | Reached `EndTime` exactly, including a zero-duration interval. |
| `DomainExit` | The first trial stage outside the closed rectangular Domain was found before calling the field there. |
| `InvalidVector` | A callback returned nonfinite/nonplanar velocity or a speed whose `Hypot` is nonfinite. |
| `InvalidPoint` | A stage coordinate overflowed or nonzero coordinate underflow rounded to zero. |
| `Stagnation` | The speed at any evaluated stage was <= `MinSpeed`. |
| `NoProgress` | An accepted candidate cannot advance coordinates, or the time grid cannot represent the proposed step within `MaxStep`. |
| `MinimumStep` | A rejected trial cannot be reduced within `MinStep`/time resolution, or a nonterminal rounded step would fall below `MinStep`. |

**No field evaluation occurs outside Domain**, including all internal trial
stages and rejected trials. Domain checks are evaluated before error acceptance.
A too-large initial trial may therefore terminate a curved or stiff trajectory
whose exact solution would remain inside the domain. This policy is explicit:
there is no domain-triggered retry, clipping, boundary root search, endpoint
interpolation, dense output or RK4 fallback. Choose the initial step and domain
accordingly. All returned points are finite and inside Domain.

The final remaining interval can be shorter than `MinStep` so it can land exactly
on `EndTime`; it must still pass the same error test. If it fails, the result
reports `MinimumStep`, without forcing acceptance or lowering tolerance. Each
trial uses the actual representable time difference. If time rounding would
exceed `MaxStep`, integration reports `NoProgress`. A zero-duration integration
returns only the seed, zero counters, and makes no field call.

## Budgets, errors and callback behavior

`AdaptiveLimits` fields are literal per-call budgets, shared across **all** seeds
in one constructor. Zero is a zero budget, not an implicit default. Hard caps are
1,000,000 accepted steps, 1,000,000 rejected trials, 8,000,000 field calls and
512 MiB modeled storage. The constructor accepts at most 100,000 seeds. Defaults
are 10,000 accepted, 10,000 rejected, 100,000 calls and 64 MiB storage.

For maximum accepted count `A` and seed count `S`, preflight reserves:

- Numerical integration: `32*(A+S) + 64 KiB`.
- Streamline geometry: `1024*(A+S) + 64 KiB`.

Times and points share two private preallocated arenas whose total capacity is
`A+S`; each immutable line result views its accepted slice without another copy.
The geometry model additionally covers line records, objects, control points,
path copies, root child references and conservative overhead. Counts are checked
against hard caps before arithmetic/allocation; the native `int` storage ceiling
is also checked. A short trajectory can retain its unused arena capacity. The
same preflight policy applies to empty or zero-duration calls. Accessor copies
created later are caller-owned allocations outside the integration budget.

The model is not an RSS or cumulative-allocation cap. Go allocator/GC retention,
standard-library internals and callback-owned allocations are separate. Numerical
work is bounded by accepted/rejected/evaluation hard caps and the fixed tableau/
precision. Arbitrary user callback time and memory cannot be bounded internally.

Malformed options/seeds/styles or a nil context/function return
`ErrAdaptiveOptions`. Exhausted accepted/rejected/evaluation/storage budgets or
too many seeds return `ErrAdaptiveLimit`. Cancellation returns the context's
error. These failures return **nil**, never a partial numerical result or
incomplete set of line objects. Controlled physical/numerical stops retain their
explicit prefix reason instead. Validation covers every seed before any callback. The constructor then snapshots
the validated seed slice before field/color calls, so synchronous callback edits
to the caller's original seed slice cannot change later seeds. Caller slices must
not be mutated concurrently while preflight is reading them.
Cancellation is checked at entry, between stages, before/after callbacks, during
seed/geometry loops and at completion. A single fixed-precision operation,
allocation, geometry clone/validation or user callback is not interruptible.
Callback panics propagate rather than being converted to successful termination.

Constructors integrate in seed order and call optional `ColorFunc` exactly once
per valid seed sample, using the original unscaled seed speed. An invalid seed
sample and a zero-duration interval retain the base color without a color call.
A stagnating but valid seed may still be colored. Neither field nor color callback
is retained or invoked during rendering. Callbacks must be deterministic for
reproducibility and themselves safe for concurrent calls when shared between
concurrent integrations; no hidden global lock serializes them. Side effects from
callbacks already invoked cannot be rolled back if a later seed fails.

`AdaptiveStreamLines.Lines()` copies line records. Their `Result` pointers refer
to immutable numerical results, and `Object` pointers borrow owning-root children.
Geometry contains exactly one cubic line segment per accepted pair, without
resampling. Transforming geometry does not mutate original numerical samples.
Add the root or an independent `Object.Copy()` tree to scenes. Existing object
rules still prohibit mutating a scene tree while concurrently sampling it.

## Independent validation

All tests and benchmarks are in `tests/_cases/field/adaptive*.go`. Fixtures are
`tests/data/field/adaptive_scipy.json`, generated only by development script
`tests/reference/field_adaptive_scipy.py` with NumPy 2.3.5 / SciPy 1.17.1. Six
systems cover harmonic motion, damped rotation, triangular linear growth, affine
flow, stiff decay and backward integration. **Sample times come from an
independent SciPy RK45 run**, not the Go integrator. A tighter DOP853 solve supplies
independent values at those times. Ordinary Go tests integrate to each stored
target without Python, SciPy, external processes or copied implementation code.
The wide reference domain admits stiff trial stages; separate tests enforce the
strict boundary policy.

Analytic checks cover harmonic phase/invariants at accepted times, exponential
convergence under tighter tolerances, exact constant velocity, signed/nonuniform
absolute timestamps, relative-only tolerance and zero components. Extreme cases
pair `1e±300` velocities with reciprocal time scales, exercise finite tolerance
scales exceeding binary64 range, and reject nonzero coordinate underflow,
overflow, stiff minimum-step failure and loss of coordinate/time progress.
Further checks cover exact global work/storage boundaries, zero evaluation and
rejection budgets, all numeric/constructor cancellation checkpoints, callback
cancellation, ownership and concurrent construction/sampling, actual rendered
geometry, borrowed children, and no callback execution during rendering.
Fuzz tests compare constant-flow trajectories across more than 1700 binary
exponents with an independent rational analytic oracle. Central TestMain keeps
any failure corpus under `tests/data/field`, never production `field/testdata`.

Focused validation on Go 1.27.3, darwin/arm64, Apple M2:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./field
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./field
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -run '^$' -fuzz '^FuzzAdaptiveConstant$' -fuzztime=15s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=field -run '^$' -bench '^BenchmarkAdaptiveRK45$' -benchmem -benchtime=200ms -p=1 -count=1
```

The final focused suite passed in 0.247 s with 95.6% package statement coverage;
race passed in 2.264 s. Vet and Linux/386 package crossbuild passed. The numeric
fuzz run passed 226,237 inputs in 15 seconds with two workers. Dependency
inspection found no cgo files. The original field/RK4 implementation and original
field/example test source checksums remained unchanged. This slice did not run
the whole repository or edit parent-owned examples/CLI/shared documents.

## Measured construction cost

Single serial samples (`-p=1`, default GOMAXPROCS 8), with damped rotation
`(x',y')=(-0.2*x-2*y, 2*x-0.2*y)` over absolute times `[0,6]`. Options use default
`atol=1e-9`, `rtol=1e-7`, initial/min/max steps `0.05/1e-10/0.5`, zero minimum
speed, and the default resource budgets. The numerical case starts at `(1,0)`;
the geometry case uses 25 seeds `(0.1+0.05*i,0)`. Both workloads verify
`TimeComplete`. Construction/setup of seed arrays, reference generation, Go
startup and unrelated package work are excluded. Timings include complete
integration and its preallocated arena; the geometry case additionally includes
objects, path copies and line-record/result checks. No rasterization, image
size or frame sampling is part of these benchmark workloads.

| Workload | Accepted | Rejected | Field evaluations | Time/op | Allocated bytes/op | Allocations/op |
|---|---:|---:|---:|---:|---:|---:|
| One numerical trajectory | 92 | 7 | 595 | 4.361 ms | 6,840,333 | 13,213 |
| Geometry for 25 trajectories | 2,262 | 138 | 14,425 | 105.504 ms | 158,666,520 | 321,026 |

The modeled new-storage reserves are respectively 385,568 and 10,331,136 bytes.
Cumulative allocations are much higher than these modeled live arrays/scratch:
the standard-library fixed-precision path creates substantial temporary traffic.
These results quantify the current correctness/range tradeoff and are not
throughput or memory/RSS guarantees. Caller callback costs also affect runtime.

## Integrated fourth-wave example — 2026-11-04

The adaptive-streamlines example integrates12 damped rotation seeds, checks every accepted state against exp(-.12t) times a planar rotation, and moves dots by interpolation of actual accepted times. Its892 accepted/72 rejected steps use5796 field calls. Three640x360/SSAA4 actual-exported-SVG checks yield RGBA MAE .087942–.090265, maxima20/20/40. It is integration evidence, not another solver oracle; provenance is in artifacts/adaptive-streamlines-wave4-svg-20261104/.
