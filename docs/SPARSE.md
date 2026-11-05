# Immutable sparse real matrices and conjugate gradients

`gomath/sparse` is a portable standard-library-only package. It has no cgo,
BLAS, external process, runtime Python/SciPy, or dependency on `linalg`. This
slice implements CSR arithmetic, sparse matrix multiplication and preconditioned
CG; it does not implement factorization, eigenproblems, or an SPD proof.

## API and ownership

```go
a, err := sparse.NewCSR(2, 2,
    []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{4, 1, 1, 3})
if err != nil { return err }
options := sparse.DefaultCGOptions()
options.Preconditioner = sparse.Jacobi // explicit; default is none
result, err := sparse.SolveCG(ctx, a, []float64{1, 2}, options)
if err != nil { return err }
x := result.Solution() // independent copy, approximately [1/11, 7/11]
_ = x
```

- `NewCSR(rows, cols, rowPtr, colInd, values)` copies input arrays.
  `NewCSRWithLimits(..., Limits)` accepts an explicit per-call resource policy.
- `Dims() (rows, cols int)`, `NNZ() int`, and `At(row, col) (float64, error)`
  expose dimensions, stored count and a checked lookup. Missing entries are zero.
- `Data() (rowPtr, colInd []int, values []float64)` returns three independent
  copies. Its allocation is bounded by the matrix's validated storage size.
- `Transpose()`, `Add(b)`, `Sub(b)`, `MulVec(x)`, `Mul(b)` allocate independent outputs.
  Each has a `WithLimits` variant taking `Limits` as the final argument.
  `MulContext(ctx, b, limits)` additionally supports cancellation; nil context
  is an error.
- `SolveCG(context.Context, *CSR, []float64, CGOptions) (*CGResult, error)`
  returns a result **only on success**. `Solution()` copies the vector;
  `Iterations()`, `RelativeResidual()` and `ResidualNorm()` expose diagnostics.
  A residual norm is represented as `fraction * 2^exponent` to avoid overflow;
  its nonzero fraction is in `[0.5, 1)`. The relative residual is a float64
  ratio and can round/underflow; it is `+Inf` when a nonzero residual is accepted
  using absolute tolerance and the RHS has zero norm. With nonzero RHS, an
  overflowing relative-residual diagnostic instead returns `ErrRange`. Use the scaled norm for
  diagnostics across the full exponent range.

Nonnegative rectangular shapes, including `0×n` and `n×0`, are valid. CSR requires
exactly `rows+1` pointers, first pointer zero, last pointer NNZ, monotone pointers,
matching value/index lengths, in-range **strictly increasing** columns in every
row, and finite values. Unsorted columns and duplicates are errors; constructors
do not sort or coalesce them. Explicit zeros are allowed and preserved by
construction/transpose. Add/Sub/Mul omit zero results, including explicit zeros.
No operation implicitly normalizes, clips, symmetrizes or regularizes a matrix.

The nonnil zero value is a valid `0×0` matrix (`Data` returns pointers `[0]`).
Nil matrices are invalid for arithmetic/lookup; nil dimension/count/data access
returns zero/nil. There are no mutators or retained input slices. Completed
matrices/results can be shared concurrently. Caller-provided arrays must not be
modified concurrently with a call that reads them. `b` is read during the solve
and is never modified or retained; `InitialGuess` is copied to private storage.

## CG contract and numerical boundaries

CG requires a real **symmetric positive definite** square matrix. It checks exact
numerical symmetry of the supplied matrix (an absent entry equals explicit zero).
It does not accept a symmetry tolerance or modify the matrix. Successful symmetry
checks and positive curvature on encountered directions **do not prove SPD**.
A symmetric indefinite or singular matrix may converge for selected RHSs, and a
zero RHS can return immediately without exploring any direction.

`CGOptions` contains `Limits`, `MaxIterations`, `RelativeTolerance`,
`AbsoluteTolerance`, `Preconditioner`, and optional `InitialGuess`.
`DefaultCGOptions()` selects 1000 iterations and relative tolerance `1e-10`.
Zero `MaxIterations` also selects 1000; values outside 1–100000 are rejected.
Tolerances are literal finite nonnegative numbers: a zero `CGOptions{}` requests
an exactly zero **computed original residual**, rather than selecting a hidden
tolerance. Success requires:

```text
||b - A*x||₂ <= max(AbsoluteTolerance, RelativeTolerance * ||b||₂)
```

Every candidate recomputes `b-A*x` using the original matrix and RHS; recurrence
residual alone never decides success. Directions restart from the original
residual every 32 iterations or if the recurrence residual becomes zero before
the original residual passes. Restarts can increase iteration counts; no claim
of convergence in at most n iterations is made. A residual tolerance is not a
forward-error bound for an ill-conditioned matrix.

`NoPreconditioner` is the default. Explicit `Jacobi` uses the original positive
diagonal entries; missing/zero/negative diagonals cause `ErrBreakdown`. It
preconditions iteration, while the stopping equation always remains `A*x=b`.
No other preconditioners are implemented.

Sparse dot products use fixed 4352-bit standard-library `math/big.Float`
accumulators. Under the hard NNZ/dimension caps, sums of binary64 input products
(including original residual components) fit exactly before final nearest-even
binary64 rounding. This preserves cancellation even when individual products
would overflow or underflow binary64. Norms and Jacobi division retain this
extended precision. CG step coefficients, direction-recurrence coefficients and
direction scales instead use **53-bit significands with an extended exponent
range**: binary64 effective precision without premature scalar overflow or
underflow. This separation is intentional. Applying the accumulator's 4352-bit
precision to recurrence coefficients can leave an artificial `2^-4352` tail at
ordinary cancellation sites; strict vector range checks would then reject a
well-scaled problem. Coefficients now have explicit iteration precision, while
exact product sums and strict vector range checks remain in place.
Search directions are scaled
by their maximum absolute component; large/small **uniformly scaled** systems
can therefore solve without forming an unscaled, overflowing `A*p`.

Vector components are binary64. An overflowing component or a nonzero component
that rounds to zero returns `ErrRange`, including intermediate `A*p`, Jacobi
application and direction normalization. Consequently some solvable systems
with an excessive dynamic range return a range error; this is not an arbitrary
precision solver. No NaN/Inf solution or silent underflow-to-zero vector is
returned. Accumulator precision is fixed; this API offers numerical verification,
not interval arithmetic or a formal exact-real residual certificate.

Errors are matchable with `errors.Is`: `ErrShape`, `ErrStructure`,
`ErrNonFinite`, `ErrOptions`, `ErrLimit`, `ErrRange`, `ErrNotSymmetric`,
`ErrBreakdown` (including encountered nonpositive curvature), `ErrConvergence`.
Cancellation returns the context error. A nil context is `ErrOptions`.
Every error returns nil output/result, including cancellation, exhausted work,
numerical range errors and iteration exhaustion. No partial iterate is exposed.

## Storage and work limits

Limits apply per call, independently of the policies used to construct operands;
objects do not contain mutable/cumulative budgets. Zero fields select defaults.

| Field | Default | Hard maximum |
|---|---:|---:|
| `MaxDimension` | 1,048,576 | 4,194,304 |
| `MaxNNZ` | 1,048,576 | 16,777,216 |
| `MaxStorageBytes` | 64 MiB | 512 MiB |
| `MaxWork` | 16,777,216 units | 17,179,869,184 units |

Negative dimensions, invalid limits, or oversized shape/storage/work are errors.
Dimensions are capped even for zero-storage shapes. Checked unsigned size
arithmetic and these hard caps avoid native-int multiplication/allocation
overflow on 32-bit and 64-bit systems.

`MaxStorageBytes` limits **each operand's CSR payload** and the **combined new
array payload** of an operation separately. Existing caller-owned inputs are not
added to new workspace. It is not a bound on total process RSS, GC metadata,
allocator size classes, or cumulative bytes allocated. Integer slots are charged
8 bytes even on 32-bit. Slice/object headers and a constant number of fixed-size
4352-bit numeric temporaries are additional for earlier operations. SpGEMM
**also charges a 16 KiB numeric-scratch reserve** against the storage budget
when any product streams exist (details below). Numeric scratch has no dimension-
dependent precision. Work/iteration limits also bound cumulative numeric calls.
No full dense matrix or dimension-by-dimension workspace is created.

For rows R, columns C, NNZ Z (and Zb for a second matrix):

| Operation | New array payload reserve | Work reserve |
|---|---:|---:|
| Constructor | `8*(R+1) + 16*Z` | `R + Z + 1` |
| Transpose | `8*(C+1) + 16*Z + 8*C` | `4*(R+C+Z+1)` |
| Add/Sub | `8*(R+1) + 16*Zout` | `4*(R+Z+Zb+1)` |
| MulVec | `8*R` | `8*(R+C+Z+1)` |
| SolveCG | `80*n` (conservative; seven n-vectors) | below |

Add/Sub count the nonzero merged output before allocating it; cancellation is
accounted for when checking output NNZ/storage. Both operand NNZ/storage must
still satisfy the requested policy.

CG charges validation `4*(n+1) + Z*(bits.Len(n)+2)`. Let
`M = 8*(n+Z+1)`. Initial residual/norm reserves `M+8*n+1`; initial
preconditioner/direction reserves `16*n+1` if needed. Each iteration reserves
`2*M+96*n+1` **before** doing that iteration, including its residual check,
preconditioning and possible restart. These are conservative bounded-kernel
units, not FLOPs, milliseconds or a promise of low allocation overhead. Context
checks occur between phases and at least every 256 entries in numerical,
transpose-entry symmetry, diagonal and direction loops. CSR arithmetic has work
bounds but no context argument. CG never starts background workers.

## Sparse matrix multiplication (SpGEMM)

`a.Mul(b)`, `a.MulWithLimits(b, limits)` and
`a.MulContext(ctx, b, limits)` return a fresh canonical CSR for `A*B`. They require
`a.cols == b.rows`, with checked dimensions on **both operands**, including empty
and zero products. A zero-valued `0×0` receiver is valid. Incompatible or nil
operands return `ErrShape`; invalid limits return `ErrOptions`. Constructors
already enforce finite, sorted, duplicate-free immutable inputs. Successful
results have strictly increasing column indices, no duplicates and no stored
zeros. There is no approximation, dropping threshold or implicit normalization.

For one left row, each nonzero `A[i,k]` selects the sorted CSR row `B[k,:]`.
A bounded binary min-heap merges these streams by output column; tied columns
are ordered by their right-hand storage index, making traversal deterministic.
The existing exact product accumulator combines **all contributions to a single
output entry before rounding once** to binary64. Exact cancellation is omitted;
nonzero final underflow-to-zero or overflow returns `ErrRange`. Individual
products may lie outside binary64 range and still cancel or accumulate to a
representable result. Explicit zero left entries start no stream; stored zeros
on the right remain budgeted traversal entries and contribute zero.

There are no maps, hash buckets, dense `output.cols` marker arrays, row-by-column
workspace, transposed right-hand copy, or dense shape products. The heap stores
only `{position, end, factor}` per active stream; one accumulator handles the
current output column. A preflight scans the left structure and selected right
row lengths, checks the complete work reserve, and obtains the maximum stream
count **before allocating the heap or evaluating products**. The first numerical
pass counts actual nonzero results and checks output/storage limits as entries
are encountered. Output indices/values are then allocated exactly once; a second
numerical pass writes them. Row pointers are retained between passes. Every error
returns nil, including failure after earlier rows have produced valid entries.

Let `R = a.rows`, `ZA = a.NNZ()`, and, for each output row:

- `S_r`: number of nonzero left entries selecting a nonempty right row;
- `T_r`: sum of the selected right row lengths, counting stored right-hand zeros;
- `S = max(S_r)`, `Zout`: final nonzero output count.

The explicit new-storage accounting is:

```text
8*(R+1) + 24*S + 16*Zout + (16 KiB if S > 0, otherwise 0)
```

Each operand's CSR payload must **independently** fit `MaxStorageBytes`; those
preexisting arrays are not added to the new workspace reserve. The stream
count cannot exceed the left row's NNZ, the common dimension, or `MaxNNZ`; final
output NNZ must also fit `MaxNNZ`. A candidate column has no retained slot or
per-column accumulator. Only its nonzero final value counts toward output NNZ.
Cancellation therefore consumes traversal work and heap space without forcing
symbolic candidate columns into output storage. The 24-byte cursor charge is
conservative on 32-bit; pointer/index slots are charged as 8 bytes throughout.
Numeric temporaries are explicitly reserved at 16 KiB, in addition to array
payload. This models bounded kernel storage and is **not a cap on RSS, Go object
headers, allocator size classes, GC-retained garbage or cumulative allocations**.
In particular the reported `B/op` can exceed this reserve due to temporary
`math/big` allocation traffic. No map-memory estimate is needed because the
implementation does not use maps.

The full work reserve is checked before expansion:

```text
4*(R + ZA + 1) + sum_r 2*T_r*(8 + 2*bits.Len(S_r))
```

`bits.Len(0)=0`, and `T_r=0` when `S_r=0`. The factor 2 pays for both numerical
passes; heap initialization, movement and exact-sum kernels are included in the
conservative per-pair model. It is a bounded-kernel budget, not a FLOP or timing
estimate. Addition/multiplication counts use checked uint64 helpers; work
multiplication is also bounded by division against remaining budget before it
is performed. Hard shape/NNZ caps bound native-int indexing and heap children,
including on 386. No `rows*cols` integer allocation product is formed.

Cancellation is checked between phases and at least every 256 row/entry/pop/
heap-construction indices; a sift-down has at most the bounded heap height.
No background workers, shared scratch or cached mutable state are used.

## Independent validation and measured cost

All test sources and benchmarks are in `tests/_cases/sparse/`; fixtures and
failure corpus are in `tests/data/sparse/`, never production `testdata/`.
`tests/reference/sparse_scipy.py` generates seven deterministic CSR/CG/matvec
references using NumPy 2.3.5 and SciPy 1.17.1. The script uses SciPy APIs, not a
translation of this implementation. Ordinary Go tests read JSON and require no
Python or SciPy. Analytic tests independently evaluate original residuals,
exercise uniform `1e±300` and smallest-subnormal scales, mixed scales, long CG
restarts, singular/indefinite/nonsymmetric matrices, range errors, literal zero
tolerances, ownership/concurrency, malformed CSR, budgets and every cancellation
checkpoint of a representative solve.

The parent `sparse-solve` integration regression is retained exactly: 31 unknowns,
diagonal 2/offdiagonal -1, RHS `h*h` with `h=1/32`, default CG and Jacobi. It now
solves in 16 iterations, with an independently computed zero original residual.
There are also 84 Poisson size/scale/preconditioner combinations (n=1–95,
uniform scales 1e±200 and 1, both preconditioners). The checked-in SciPy data now
includes Poisson n=31, 32 and 63. Neither the scene nor its tests were weakened.

Validation on 2026-11-04 (Go 1.27.3, darwin/arm64, Apple M2):

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./sparse
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./sparse
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -fuzz '^FuzzCSR$' -fuzztime=12s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -fuzz '^FuzzCGDiagonal$' -fuzztime=15s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -fuzz '^FuzzMulVecRange$' -fuzztime=12s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -fuzz '^FuzzCGPoisson$' -fuzztime=15s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=examples/scenes -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -bench . -benchmem -benchtime=200ms -p=1 -count=1
```

The runner supplies the `noasm` tag. Focused tests currently cover 96.3% of
statements. No whole-repository test claim is made for this slice. The initial
CG fuzz assertion demanded bitwise equality for a rounded normalized direction;
it was corrected to a numerical tolerance and its `[]byte("1X")` case is retained
in `tests/data/sparse/testdata/fuzz/` as regression input. After the coefficient
precision fix, the full focused sparse suite, sparse race, vet, Linux/386 build,
and the full `examples/scenes` suite passed. Additional 15-second CG fuzz runs
passed 321,783 diagonal-system and 22,898 Poisson inputs.

Serial benchmark samples (CG remeasured after the coefficient-precision fix),
default GOMAXPROCS 8; construction/fixture reading,
startup and external reference generation excluded:

| Workload | Time/op | Bytes allocated/op | Allocations/op |
|---|---:|---:|---:|
| Transpose, tridiagonal n=1024, NNZ=3070 | 14.224 µs | 66,912 | 5 |
| Add same matrix, n=1024, NNZ=3070 | 31.934 µs | 58,720 | 4 |
| MulVec same matrix | 165.751 µs | 174,416 | 2,636 |
| CG, tridiagonal n=64, NNZ=190, diagonal 4/offdiagonal -1, RHS ones, Jacobi, rtol=1e-10 | 3.424 ms / 17 iterations | 4,735,472 | 25,687 |

These costs include robust extended-range accumulation and actual residual
recomputation on every CG iteration. `math/big` creates substantial temporary
allocation traffic; this first implementation prioritizes bounded, checked
numerical behavior over BLAS-like throughput. Cumulative allocation bytes in the
benchmark are distinct from the new-array storage budget and peak live memory.


### SpGEMM validation and measured costs

All product tests are central: `tests/_cases/sparse/mul_test.go`,
`mul_fuzz_test.go`, and `mul_bench_test.go`. The development-only generator
`tests/reference/sparse_product.py` writes `tests/data/sparse/product.json`.
Five independent SciPy cases cover random floating-point and exact integer
products, rectangular shapes and empty dimensions. Seven independent Python
Fraction cases operate on the actual binary64 input values and retain exact
rational strings plus rounded IEEE-754 output bits. They cover products which
individually overflow but cancel to tiny finite entries, products individually
below binary64 range which accumulate to a representable subnormal, exact-zero
cancellation, explicit zeros, mixed scales, rounding, and rejected final
range errors. Ordinary tests require neither development dependency.

Further tests cover exact work/storage boundaries, checked uint64 overflow,
both operand policies even for a zero/empty product, output expansion beyond
NNZ/storage budgets, heap space despite complete cancellation, every context
checkpoint of a representative two-pass product, deterministic arrays, input
and result ownership, and concurrent products of shared immutable matrices.
A 4,194,304-column product with two output entries succeeds under a 16,456-byte
new-storage policy, demonstrating that workspace does not scale with output
column count. The rational Go fuzz oracle independently computes each small
product using `math/big.Rat`, including extreme scales; a 20-second run passed
323,129 inputs with two workers. Existing CG implementation/numeric kernel and
CG/Poisson regression source checksums were unchanged by this slice.

Focused validation (same Go 1.27.3 / Apple M2 / darwin-arm64 environment):

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./sparse
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./sparse
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -fuzz '^FuzzSpGEMM$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -bench '^BenchmarkSpGEMM$' -benchmem -benchtime=200ms -p=1 -count=1
```

Single serial benchmark samples, default GOMAXPROCS 8, with construction,
startup, fixture/reference generation and other package work excluded. Each
sample includes preflight, both numerical passes and exact CSR output allocation:

| Product workload | Scalar pairs / pass | Reserved work units | Output NNZ | Time/op | Allocated bytes/op | Allocations/op |
|---|---:|---:|---:|---:|---:|---:|
| Tridiagonal square, n=256, input NNZ=766 each | 2,294 | 59,148 | 1,274 | 278.711 µs | 235,341 | 3,578 |
| Tridiagonal square, n=1024, input NNZ=3070 each | 9,206 | 237,324 | 5,114 | 1.110 ms | 942,917 | 14,330 |
| Dense-in-pattern 1×64 times 64×128, 64-stream fan-in | 8,192 | 360,712 | 128 | 1.631 ms | 1,404,901 | 16,139 |
| 1×1 times 1×1,048,576, two right/output entries | 2 | 52 | 2 | 424.1 ns | 608 | 13 |

There are no timing guarantees. The modeled new-storage reserves for these
workloads are respectively 38,896, 106,480, 19,984 and 16,456 bytes; cumulative
allocation traffic and RSS are separate quantities. High fan-in pays for heap
movement and exact summation. The fixed scratch reserve is conservative; tiny
products often allocate much less than the modeled reserve.

## Nonsymmetric systems: bounded BiCGSTAB

```go
o := sparse.DefaultBiCGSTABOptions()
o.Preconditioner = sparse.Jacobi // explicit; omit for no preconditioning
result, err := sparse.SolveBiCGSTAB(ctx, a, rhs, o)
// Only a successful result exposes an independent Solution() copy.
```

`SolveBiCGSTAB(context.Context, *CSR, []float64, BiCGSTABOptions)` accepts finite
real square CSR systems without symmetry or positive-definiteness checks. It uses
right-preconditioned BiCGSTAB, with no dense conversion, native backend or external
process. `BiCGSTABOptions` is an independent type containing `Limits`,
`MaxIterations`, `RelativeTolerance`, `AbsoluteTolerance`, `BreakdownTolerance`,
`Preconditioner` and `InitialGuess`. `BiCGSTABResult` independently exposes
`Solution`, `Iterations`, `ResidualNorm` and `RelativeResidual` with the same
copy/scaled-norm conventions as the CG result. Inputs are not mutated or retained;
caller slices must remain unchanged during a call. Shared immutable matrices and
completed results support concurrent readers/solves.

Jacobi divides search directions by the original diagonal. Every diagonal must
be nonzero, but **negative diagonal entries are accepted**. This is deliberately
different from CG's positive-diagonal requirement. Reusing the preconditioner
selector does not impose an SPD prerequisite. The initial guess is copied; a nil
guess means zero. Shape, finite-input, options, operand and storage checks,
including Jacobi diagonal validation, apply even if the initial guess would
already satisfy the stopping tolerance. A nonnil zero CSR solves the empty
system, subject to its explicit budgets.

The default constructor selects 1000 iterations, relative tolerance `1e-10`, zero
absolute tolerance and breakdown tolerance `1e-30`. Literal zero stopping
tolerances request exactly zero **computed original residual**; zero
`MaxIterations` selects 1000. The iteration hard cap is 100000. Stopping requires
`||b-A*x||2 <= max(atol, rtol*||b||2)`, recomputed from the original, unmodified
system at each half-step and full-step candidate. A vanishing recurrence residual
is insufficient. A half-step success counts as that outer iteration. At most
two candidate checks and two recurrence matrix products occur per iteration.
Every 32 full iterations, and after a vanishing recurrence residual that fails
verification, the method restarts its residual, shadow vector and directions
from the verified candidate and its actual residual. This restart policy can
change iteration counts from other BiCGSTAB implementations.

Breakdown is checked for rho, the alpha denominator, and omega. The explicit
policy is `|dot(u,v)| <= BreakdownTolerance*||u||*||v||`, compared by exact squares
without square-root rounding. Tolerances must be finite, nonnegative, and the
breakdown tolerance must be less than one; zero disables the near-orthogonality
policy but still detects exact zero denominators. `ErrBiCGSTABBreakdown` identifies
these cases and zero Jacobi diagonals. `ErrBiCGSTABConvergence` identifies exhausted
iterations. These independent sentinels avoid reusing CG's positive-curvature or
CG-specific convergence messages. Other failures reuse `ErrShape`, `ErrOptions`,
`ErrNonFinite`, `ErrLimit`, `ErrRange`, or the context cancellation/deadline error.
Every error returns a nil result. Nonsingularity does not guarantee convergence
of this method, and a small residual does not certify conditioning or forward
accuracy. No SPD or invertibility certificate is produced.

### Numerical range and budgets

Internal vectors and recurrence coefficients use nearest-even 53-bit
significands with bounded extended exponents: each nonzero stored component or
coefficient must have `big.Float.MantExp` in `[-8192,8192]`. Exact matrix/vector and
dot product sums use a fixed 33024-bit accumulator, enough for up to the hard NNZ
cap of 16M products over that entire exponent span. Breakdown comparisons reserve
66176 bits for exact squared sums times squared binary64 tolerances. These are
fixed limits, not adaptive precision or a general arbitrary-precision API. Wide
accumulator results round through one scratch scalar before entering a vector,
so vectors cannot retain a separate wide mantissa per component.

The original candidate is binary64: nonfinite values and nonzero values that
would round to zero are `ErrRange`. Each actual residual component is formed by
one exact sum of original binary64 products followed by one binary64 rounding;
its nonzero underflow also returns `ErrRange`. Thus some subnormal systems fail
explicitly at an intermediate candidate even though a mathematical solution
exists. Nonzero subnormal values that remain representable are supported.
The original residual norm and RHS norm retain the existing scaled accumulation
policy. `ResidualNorm` returns a fraction in `[0.5,1)` and a base-two exponent,
or `(0,0)` for zero. Relative residual can be `+Inf` only for zero RHS accepted
with a nonzero residual using absolute tolerance; overflow for nonzero RHS is a
range error. An extremely small relative diagnostic may round to zero; the
stopping check still compares extended-range norms.

For dimension `N`, input NNZ `Z`, and `matWork = 8*(N+Z+1)`:

- Input CSR shape/NNZ/storage is checked against `Limits` independently.
- New solver storage is reserved as `2560*N + 256 KiB`. This includes nine
  extended vectors, three binary64 vectors, conservative headers/limbs and fixed
  numeric scratch, including the larger breakdown comparison. There is no
  allocation proportional to `N*N` or a hidden dense column workspace. Even an
  empty solve reserves the fixed scratch policy. Every count is bounded by
  validated dimension/NNZ hard caps before arithmetic or allocation; the native
  `int` storage ceiling also applies on 386.
- Initialization reserves `4*(N+Z+1) + matWork + 16*N + 1` work units.
- Each outer iteration reserves `4*matWork + 512*N + 1`, including half-step
  success, both residual checks, optional preconditioning and restart paths.
  Insufficient work stops before that iteration. Units conservatively budget
  bounded numerical kernels, not FLOPs or wall time.
- Context is checked at entry, reservations, completion and at least every 256
  entries during initialization and numerical loops. Single bounded `math/big`
  operations and Go allocations are not interruptible internally.

These storage reserves describe the algorithm's arrays/headers/limbs and modeled
scratch, **not a cap on RSS or cumulative allocations**. Go allocator metadata,
GC retention, standard-library scratch pools and caller-owned arrays are separate.
The fixed-precision `math/big` path favors explicit range behavior over throughput.

### BiCGSTAB validation

Tests, benchmark and fuzz sources are exclusively
`tests/_cases/sparse/bicgstab*.go`. The independent development-only generator
`tests/reference/sparse_bicgstab.py` produces `tests/data/sparse/bicgstab.json` using
NumPy 2.3.5 / SciPy 1.17.1. Eleven stored systems compare against both SciPy
BiCGSTAB and sparse direct solutions: random nonsymmetric diagonally dominant
matrices (n=7/29/73), explicit Jacobi variants, nonsymmetric convection n=31,
negative diagonal, a nonnormal upper-triangular system, and uniform `1e±300`
scales. Extreme reference cases solve an explicitly scaled equivalent system;
Go receives the original stored matrix and RHS. No third-party implementation
source is copied and ordinary tests require neither Python nor SciPy.

Further tests cover zero RHS with zero/nonzero guesses, empty and exact-guess
systems, exact and near breakdown, singular systems, zero/negative Jacobi
diagonals, nonzero underflow/overflow, an original residual that disagrees with a
zero recurrence residual, a norm exceeding binary64 range, periodic restarts,
column scales separated by 1800 binary exponents, exact work/storage boundaries,
all successful-path cancellation checkpoints, input/result ownership and shared
concurrency. An independent rational original-residual fuzz oracle uses exactly
known solutions and nonsymmetric systems over roughly `2^±900` uniform scales.
It permits explicit BiCGSTAB breakdown/nonconvergence, since nonsingularity alone
cannot promise this algorithm's convergence; every successful result is checked
against both the known solution and exact rational original-system residual.
Existing CG/Poisson source checksums remain unchanged.

Final focused checks (Go 1.27.3, darwin/arm64, Apple M2):

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./sparse
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./sparse
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -fuzz '^FuzzBiCGSTAB$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=sparse -run '^$' -bench '^BenchmarkBiCGSTAB$' -benchmem -benchtime=200ms -p=1 -count=1
```

The focused suite passed in 1.436 s with 96.4% package statement coverage; focused
race passed in 13.010 s. Vet and Linux/386 package crossbuild passed. The final
20-second fuzz run passed 158,433 inputs with two workers. A focused dependency
listing contains only the standard library and `gomath/sparse`, with no cgo files.
This slice did not rerun the whole repository or change the existing CG numeric
kernel/solver or its CG/Poisson regression sources.

Serial benchmark samples use RHS `e1`, diagonal 2.1, lower diagonal -1.25 and upper
diagonal -0.75, rtol `1e-10`, default breakdown policy and `MaxWork=1<<30`.
Construction, fixture generation, startup and other packages are excluded;
complete solver initialization, preconditioning and original residual checks
are included. `-p=1` serializes the benchmark package; default GOMAXPROCS was 8.
These are individual measurements, not timing guarantees.

| Convection system | Jacobi | Iterations | Time/op | Allocated bytes/op | Allocations/op |
|---|---|---:|---:|---:|---:|
| n=32, NNZ=94 | no | 32 | 2.429 ms | 2,867,479 | 24,441 |
| n=32, NNZ=94 | yes | 32 | 2.429 ms | 2,821,504 | 25,514 |
| n=128, NNZ=382 | no | 72 | 17.381 ms | 20,693,081 | 151,274 |
| n=128, NNZ=382 | yes | 73 | 17.586 ms | 19,874,842 | 165,183 |

Modeled solver storage is 344,064 bytes for n=32 and 589,824 bytes for n=128;
input CSR storage is checked separately. The much larger cumulative allocation
traffic illustrates why the policy is not advertised as an RSS or total-allocation
cap. Per-iteration work reservations are respectively 20,449 and 81,889 units.

## Integrated nonsymmetric solve

The `bicgstab` CLI scene actually solves a32-by32 nonsymmetric tridiagonal CSR
(lower=-1, diagonal=3, upper=-.5;94 nonzeros). An independently formed RHS comes
from a manufactured smooth vector. Jacobi-preconditioned BiCGSTAB returns after
10 iterations; another matrix-vector product recomputes relative residual
4.35e-11 before drawing the returned values. Three640x360 SSAA4 SVG/CairoSVG
comparisons at .5/1.25/2.8seconds pass: MAE .070417–.072937/255, maximum50.
Provenance: artifacts/bicgstab-demo-svg-20261104/provenance.json. This validates
actual numerical/plot integration, not whole-library numerical or speed parity.
