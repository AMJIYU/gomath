# Dense real and complex numerical foundation

`gomath/linalg` is a standalone, standard-library-only numerical package. Its
runtime uses Go float64 arithmetic (with a bounded exact-product accumulator
for general eigenvector checks and complex dot/update kernels), with no native bindings, cgo, subprocesses,
external numerical dependencies, or changes to the Python Manim reference.
It builds and runs with `CGO_ENABLED=0` and `-tags noasm`.

This package provides dense real matrix arithmetic, square LU direct solves,
column-pivoted Householder QR for full-column-rank least squares, Cholesky
solves for symmetric positive definite matrices, real symmetric eigenvalue
decomposition, and economy one-sided Jacobi SVD for arbitrary rectangular
matrices, including rank-deficient and zero matrices. General square real
matrices additionally support bounded real Schur factorization, complex
eigenvalues and separately computed complex right eigenvector columns. SVD provides numerical
rank, truncated pseudoinverses and minimum-norm least-squares solves with one
or multiple RHS columns. It does not provide all of a numerical library or SciPy.
A separate immutable `ComplexDense` now provides bounded complex128 arithmetic
and partial-pivot `ComplexLU` direct solves. There are no left/generalized
eigenvectors, general-complex eigendecompositions, sparse storage, BLAS backend,
or automatic differentiation. Rectangular matrices
work for arithmetic; QR supports coefficient matrices with rows >= columns;
LU, Cholesky, symmetric/general eigenvalue and real Schur decomposition require square matrices.
Unsupported shapes return errors.

## Ownership and integration API

Import `gomath/linalg`. The existing `gomath/matrix` package handles visual
notation and has no numerical dependency on this package. Integrators can pass
copies of numerical values into that presentation layer explicitly.

| Entry point | Contract |
| --- | --- |
| `NewDense(rows, cols int, data []float64) (*Dense, error)` | Copy exactly `rows*cols` finite row-major values. |
| `Zeros(rows, cols int) (*Dense, error)` | Allocate a zero matrix. |
| `Identity(n int) (*Dense, error)` | Allocate the square identity. |
| `(*Dense).Dimensions() (int, int)` | Read immutable dimensions. |
| `(*Dense).At(row, col int) (float64, error)` | Read a checked zero-based entry. |
| `(*Dense).Data() []float64` | Return a copy in row-major order. |
| `(*Dense).Transpose() (*Dense, error)` | Materialize an independent transpose. |
| `(*Dense).Add/Sub/Mul(b *Dense) (*Dense, error)` | Allocate elementwise addition/subtraction or matrix multiplication. |
| `(*Dense).MulVec(x []float64) ([]float64, error)` | Allocate the matrix-vector product. |
| `Factorize(a *Dense) (*LU, error)` | Copy and factor a square matrix with the default pivot policy. |
| `FactorizeWithOptions(a *Dense, opt LUOptions) (*LU, error)` | Factor with an explicit relative pivot policy. |
| `(*LU).Size() int` | Read the factorization size. |
| `(*LU).Pivots() []int` | Copy the final row permutation, **not** the sequence of swaps. |
| `(*LU).Factors() (lower, upper *Dense, err error)` | Materialize independent triangular matrices satisfying `P*A=L*U`. |
| `(*LU).Solve(b []float64) ([]float64, error)` | Solve one RHS using stored factors. |
| `(*LU).SolveMatrix(b *Dense) (*Dense, error)` | Solve `A*X=B` with one RHS per column of `B`. |
| `(*LU).Det() (float64, error)` | Evaluate the determinant from the factors. |
| `(*LU).LogDet() (sign int, logAbs float64, err error)` | Evaluate its sign and natural log of absolute value. |
| `(*LU).Inverse() (*Dense, error)` | Solve `A*X=I` using the stored factors. |
| `Solve(a *Dense, b []float64) ([]float64, error)` | One-shot default factorization and vector solve. |
| `SolveMatrix(a, b *Dense) (*Dense, error)` | One-shot default factorization and multi-RHS solve. |
| `Det(a *Dense) (float64, error)` | One-shot default factorization and determinant. |
| `Inverse(a *Dense) (*Dense, error)` | One-shot default factorization and inverse. |
| `FactorizeQR(a *Dense) (*QR, error)` | Column-pivoted Householder QR with the default rank threshold. |
| `FactorizeQRWithOptions(a *Dense, opt QROptions) (*QR, error)` | QR with an explicit computed-rank policy. |
| `(*QR).Dimensions() (int, int)` | Read the original coefficient shape. |
| `(*QR).Pivots() []int` | Copy the final **column** permutation. |
| `(*QR).Factors() (q, r *Dense, err error)` | Independent thin Q and square R with `A*P=Q*R`. |
| `(*QR).Solve(b []float64) (x []float64, residual float64, err error)` | Least-squares fit and actual residual 2-norm. |
| `(*QR).SolveMatrix(b *Dense) (x *Dense, residuals []float64, err error)` | An n-by-r fit and one actual residual norm per RHS column. |
| `(*QR).SolveWithOptions(b []float64, opt LeastSquaresOptions)` | Same return values as Solve, with an optional residual limit. |
| `(*QR).SolveMatrixWithOptions(b *Dense, opt LeastSquaresOptions)` | Same return values as SolveMatrix, with per-column residual limits. |
| `LeastSquares(a *Dense, b []float64)` | One-shot default QR; same return values as QR.Solve. |
| `LeastSquaresMatrix(a, b *Dense)` | One-shot default QR; same return values as QR.SolveMatrix. |
| `FactorizeCholesky(a *Dense) (*Cholesky, error)` | Default symmetry and positive-pivot validation, followed by factorization. |
| `FactorizeCholeskyWithOptions(a *Dense, opt CholeskyOptions) (*Cholesky, error)` | Explicit symmetry and positive-pivot policies. |
| `(*Cholesky).Size() int` | Read the square coefficient size. |
| `(*Cholesky).Lower() (*Dense, error)` | Copy the lower-triangular factor L. |
| `(*Cholesky).Solve(b []float64) ([]float64, error)` | Solve `L*Lᵀ*x=b` using the stored factors. |
| `(*Cholesky).SolveMatrix(b *Dense) (*Dense, error)` | Solve one RHS per column without refactorization. |
| `SolveSPD(a *Dense, b []float64) ([]float64, error)` | One-shot default Cholesky vector solve. |
| `SolveSPDMatrix(a, b *Dense) (*Dense, error)` | One-shot default Cholesky multi-RHS solve. |
| `FactorizeEigenSym(a *Dense) (*EigenSym, error)` | Real symmetric cyclic Jacobi eigendecomposition with default options. |
| `FactorizeEigenSymWithOptions(a *Dense, opt EigenSymOptions) (*EigenSym, error)` | Explicit symmetry, convergence tolerance and sweep budget. |
| `(*EigenSym).Size() int` | Read the square input size. |
| `(*EigenSym).Values() []float64` | Copy eigenvalues in descending algebraic order. |
| `(*EigenSym).Vectors() (*Dense, error)` | Copy orthonormal eigenvector columns V, with `A=V*diag(values)*Vᵀ`. |
| `FactorizeSchur(a *Dense) (*Schur, error)` | Bounded general real square Schur decomposition with default options. |
| `FactorizeSchurWithOptions(a *Dense, opt SchurOptions) (*Schur, error)` | Explicit size, total QR iterations, scalar work, deflation and final error policies. |
| `(*Schur).Factors() (q, t *Dense, err error)` | Independent real orthogonal Q and upper quasi-triangular T, `A≈Q*T*Qᵀ`. |
| `(*Schur).Values() []complex128` | Copy complex eigenvalues in unsorted block order, positive-imaginary member first in each conjugate pair. |
| `(*Schur).Size()` / `Iterations()` | Input order and total implicit QR sweeps; direct 2×2 rotations are not iterations. |
| `(*Schur).RelativeReconstructionError()` / `OrthogonalityError()` | Checked relative Frobenius reconstruction residual and `||QᵀQ-I||F`. |
| `FactorizeEigen(a *Dense) (*Eigen, error)` | General real square input; bounded complex right eigenvectors, with default policies. |
| `FactorizeEigenWithOptions(a *Dense, opt EigenOptions) (*Eigen, error)` | Explicit Schur, combined-work, residual, numerical-rank and required-independence policies. |
| `(*Eigen).Values() []complex128` | Copy values in unsorted Schur block order. |
| `(*Eigen).RightVector(column int) ([]complex128, error)` | Copy one normalized right eigenvector in matching Values order. |
| `(*Eigen).Residuals() []float64` | Copy every checked original-A relative eigenpair residual. |
| `(*Eigen).BasisRank()` / `BasisTolerance()` | Numerical rank of the returned columns and its relative threshold; not exact diagonalizability. |
| `(*Eigen).RegularizedColumns() []bool` | Copy flags for singular backsolves that needed an explicit pivot perturbation. |
| `(*Eigen).Size()` / `SchurIterations()` | Input order and underlying total implicit QR sweeps. |
| `FactorizeSVD(a *Dense) (*SVD, error)` | Economy one-sided Jacobi SVD for tall, square or wide inputs, of any numerical rank. |
| `FactorizeSVDWithOptions(a *Dense, opt SVDOptions) (*SVD, error)` | Explicit orthogonality, reconstruction, numerical rank and sweep policies. |
| `(*SVD).Dimensions() (int, int)` | Read the original m-by-n input shape. |
| `(*SVD).Values() []float64` | Copy k=min(m,n) nonnegative descending singular values, including zeros; independent of the rank cutoff. |
| `(*SVD).Factors() (u, v *Dense, err error)` | Independent m-by-k U and n-by-k V with `A≈U*diag(values)*Vᵀ`; tall input shapes are unchanged. Returns V, not Vᵀ. |
| `(*SVD).RelativeReconstructionError() float64` | Measured relative Frobenius reconstruction error against original A. |
| `(*SVD).DeflatedColumns() int` | Count unresolved residual columns explicitly set to zero under the reconstruction policy. |
| `(*SVD).Rank() int` | Numerical rank using the stored relative cutoff. |
| `(*SVD).RankWithTolerance(tol float64) (int, error)` | Numerical rank under an explicit finite cutoff in [0,1). |
| `(*SVD).PseudoInverse() (*Dense, error)` | Independent n-by-m truncated Moore-Penrose inverse under the stored cutoff. |
| `(*SVD).PseudoInverseWithTolerance(tol float64) (*Dense, error)` | Pseudoinverse under an explicit relative cutoff. |
| `(*SVD).Solve(b []float64) (x []float64, residual float64, err error)` | Minimum-norm fit for the retained subspace and actual original-matrix residual. |
| `(*SVD).SolveWithTolerance(b []float64, tol float64)` | Same return values as Solve, using an explicit rank cutoff. |
| `(*SVD).SolveMatrix(b *Dense) (x *Dense, residuals []float64, err error)` | Independent n-by-r minimum-norm fits, one actual original-matrix residual norm per RHS column. |
| `(*SVD).SolveMatrixWithTolerance(b *Dense, tol float64)` | Same return values as SolveMatrix, using an explicit rank cutoff. |

`Dense` has private dimensions and storage, with no mutation or resize API.
Constructors copy caller slices; all data, permutation, and factor exports are
copies. Arithmetic and solves leave their inputs unchanged, including on error.
Both matrices and completed factorizations can be shared by concurrent readers.
Callers must not concurrently mutate input slices while a call reads them.

Nonnegative zero dimensions are supported. The zero value of `Dense` is a valid
0-by-0 matrix. An `m`-by-0 matrix times a 0-by-`n` matrix is the zero `m`-by-`n`
matrix. The empty square determinant is 1; its log determinant is `(1,0)` and
its inverse is empty. Empty RHS columns retain their shape. Negative dimensions,
entry-count mismatches, and integer overflow of entry/byte counts are errors.
Dense arithmetic and the earlier factorizations have no configured memory quota
or out-of-memory recovery: representable but excessive allocations can exhaust
memory. Schur/general Eigen and the separate complex matrix/LU entry points
additionally enforce explicit order/work bounds.

A nil `*Dense` is invalid for numerical operations. Its `Dimensions`/`Data`
introspection returns `(0,0)`/nil. Nil or zero-value `LU`, `QR`, `Cholesky`,
`EigenSym`, `Schur`, `Eigen`, and `SVD` factors are uninitialized; numerical operations return `ErrInvalid`, and
dimension/size/permutation introspection returns zero/nil. Use the appropriate
factorization constructor even for valid empty factorizations.

## Factor once, solve repeatedly

```go
a, err := linalg.NewDense(2, 2, []float64{0, 2, 1, 3})
if err != nil {
    return err
}
lu, err := linalg.Factorize(a)
if err != nil {
    return err
}
x, err := lu.Solve([]float64{4, 7}) // [1, 2]; input remains unchanged
if err != nil {
    return err
}
b, err := linalg.NewDense(2, 2, []float64{4, 2, 7, 4})
if err != nil {
    return err
}
xs, err := lu.SolveMatrix(b) // RHS are columns; X = [[1,1],[2,1]]
if err != nil {
    return err
}
// Consume x and xs in the application; the same lu remains reusable.
```

Use solves to apply an inverse to data. Forming the full inverse costs more
work and storage and is unnecessary for solving a system. Factorization costs
O(n³) time and O(n²) storage. Each vector solve costs O(n²) time and O(n) new
storage; `r` RHS columns cost O(n²r) time and O(nr) new storage. Inversion costs
O(n³) time and O(n²) new storage; determinant evaluation costs O(n) time and no
heap allocation in the recorded benchmark.

## Pivot policy and numerical behavior

LU selects the largest absolute entry in the active column: ordinary partial
pivoting, not scaled pivot selection. Entire rows, including previously stored
lower-triangular multipliers, are swapped. If `p := lu.Pivots()`, then row `i`
of `P*A` is row `p[i]` of the original matrix. The lower factor has unit diagonal.

`LUOptions.RelativePivotTolerance` must be finite and in `[0,1)`.
`DefaultLUOptions()` sets it to **1e-14**. At each step let `s` be the maximum
absolute entry in the selected row of the **original** matrix; that scale moves
with row swaps and is not recomputed after elimination. A computed pivot `v`
is rejected if:

```text
v == 0, or (tolerance > 0 and abs(v)/s <= tolerance).
```

The ratio avoids an absolute cutoff or an underflowing `tolerance*s` product.
Uniformly scaling a system normally preserves the decision, provided rounding,
underflow, and overflow do not change the represented system. A diagonal matrix
containing both `1e-200` and `1e200` is accepted because each row has its own
scale. Nonzero subnormal pivots are supported: multipliers are computed by direct
division, rather than by first computing a potentially overflowing reciprocal.

For exact-zero checking, explicitly use:

```go
lu, err := linalg.FactorizeWithOptions(a, linalg.LUOptions{
    RelativePivotTolerance: 0,
})
```

The zero value of `LUOptions` has this exact-zero meaning; it does not select the
default tolerance. Exact-zero mode does not reject a nonzero pivot just because
its ratio to the row scale underflows to zero. It still rejects nonfinite
arithmetic. It checks **computed float64 pivots**, not exact mathematical rank:
rounding may make a nonsingular matrix appear singular or leave a tiny nonzero
pivot for a mathematically singular input.

The default threshold is a documented heuristic, not a condition-number or
rank estimate and not a proof of numerical singularity. It may reject solvable
systems with disparate entries within a row, while accepting systems whose
overall condition number is large. Row-wise rescaling can also change ordinary
partial-pivot choices. Use an explicit policy when the application has a reason
to accept small computed pivots; it does not restore accuracy lost to rounding.

Trailing LU updates, dot products, and substitution updates use `math.FMA`.
They still use float64 sequential accumulation without compensated summation,
automatic equilibration, complete pivoting, iterative refinement, pivot-growth
reporting, condition estimation, or extended precision. Partial pivoting can
have substantial element growth. Arithmetic that becomes nonfinite returns an
error even if a different evaluation order or rescaling could produce a finite
answer. Ordinary finite underflow rounds according to float64 arithmetic and
can lose information, including rounding small products or solutions to zero.
Successful operations guarantee finite outputs, not correct digits for every
input or mathematical invertibility. Fused operations may differ in last bits
from implementations that round multiplication and addition separately.

Check the residual appropriate to the application, for example
`||A*X-B||∞ / (||A||∞*||X||∞ + ||B||∞)`. Even a small residual can coexist with
large solution error for an ill-conditioned matrix. The Hilbert test exercises
this distinction; the package does not calculate a condition estimate.

Determinants include the row-swap parity. Their diagonal product accumulates
normalized mantissas and powers of two so an intermediate product does not
overflow or underflow when the final determinant is representable. A final
overflow or a nonzero determinant rounding to zero returns `ErrRange`; a
nonzero subnormal result is allowed. `LogDet` remains usable when the ordinary
determinant is out of range. Both describe the rounded LU factors, not an exact
arithmetic determinant. If factorization rejects a pivot, the one-shot `Det`
returns `(0, error)` rather than silently treating the threshold as proof that
the determinant is mathematically zero.

## Householder QR and least squares

For an m-by-n coefficient matrix, `FactorizeQR` requires **m >= n** and full
column rank under the selected policy. It stores compact Householder vectors,
their weights, a final column permutation, and an independent original matrix
copy for residual evaluation. It never forms `Aᵀ*A` or solves normal equations.
The factorization identity is `A*P=Q*R`, where Q is thin (m-by-n), Qᵀ*Q=I, and
R is n-by-n upper triangular. Column `j` of `A*P` is original column
`Pivots()[j]`; solves undo that permutation before returning coefficients.
`Factors` materializes independent Q/R matrices; the full m-by-m Q is not stored
or exported. Each solve applies the compact reflectors directly to the RHS.

Pivot selection recomputes every remaining column's tail norm at each step,
rather than relying on repeatedly downgraded norm estimates. Norms divide by
their largest entry before accumulating with `math.Hypot`, avoiding unscaled
squares that could overflow or erase a subnormal column. Reflectors are built
in those normalized units, so a finite column norm does not require forming a
possibly overflowing `alpha-beta`. Target columns are scaled during reflector
application, avoiding unnecessary reflector-dot overflow when output entries
remain representable. A coefficient column norm exceeding MaxFloat64 is an
arithmetic error because the corresponding R pivot cannot be stored.

`DefaultQROptions()` sets `RelativeRankTolerance` to **1e-14**. The option must
be finite and in `[0,1)`. Let `s` be the largest original column 2-norm and `v`
the largest remaining column-tail 2-norm. A step rejects when:

```text
v == 0, or (RelativeRankTolerance > 0 and v/s <= RelativeRankTolerance).
```

Failure returns nil with `*RankError`, matching `ErrRankDeficient`. `Rank` is
the count of columns accepted before this rejection, **not an exact rank**;
the error also records column count, `Norm`, `Scale`, and the requested
tolerance. `QROptions{}` explicitly disables relative rejection and checks only
computed zero tails. It does not make a rank-deficient problem mathematically
solvable: roundoff can leave a tiny nonzero tail for a truly dependent column,
or destroy one that was mathematically independent. No dependent columns are
silently dropped, and no minimum-norm fallback is selected.

This global column-norm policy differs deliberately from LU's original-row
pivot policy. For example, `diag(1e-300,1e300)` is rejected by default QR even
though LU's local row check accepts it. Zero QR tolerance permits factorization
when the computed tails are nonzero, but does not promise accurate coordinates
for that scale disparity. Uniform scaling normally preserves a rank decision
within float64 range; scaling different columns can change the decision. The
fixed threshold does not automatically account for dimension, measurement
noise, units, singular values, or the condition number. Column-pivoted QR is a
useful rank heuristic, not an SVD or a guaranteed rank-revealing method for
every adversarial matrix. Near dependence can leave a tiny residual and large
coefficient errors. There is no condition estimator, refinement, equilibration,
or extended precision.

`Solve` returns `(x, residual2Norm, error)`; `SolveMatrix` returns an n-by-r
matrix, r residual norms, and an error. Norms are **actual computed**
`||A*x-b||₂` against the stored original coefficients in the original column
order, not just norms of the trailing transformed RHS. This also detects, for
example, a square-system solution rounding to zero while a nonzero RHS remains.
Residual entries use FMA accumulation and their norm uses `math.Hypot`, without
squaring entries. This is a float64 residual measurement, not a rigorous bound
on the exact real residual. A large residual is valid for an inconsistent
least-squares problem; a small residual alone does not certify forward accuracy
or optimality for an ill-conditioned problem.

Enable an application-specific fit limit explicitly:

```go
qr, err := linalg.FactorizeQR(a)
if err != nil {
    return err
}
x, residual, err := qr.SolveWithOptions(b, linalg.LeastSquaresOptions{
    CheckResidual: true,
    RelativeResidualTolerance: 1e-6,
})
if err != nil {
    return err // errors.Is(err, linalg.ErrResidual) identifies a rejected fit
}
// Consume x and residual. A nonzero residual within the limit is accepted.
```

The check requires `||A*x-b||₂/||b||₂ <= RelativeResidualTolerance` independently
for each RHS; the tolerance must be finite and nonnegative even if checking is
disabled. Zero tolerance requires a computed zero residual, checking the norm
directly so ratio underflow cannot silently pass it. With a zero RHS the
computed zero solution has zero residual and passes. The default
`LeastSquaresOptions{}` disables rejection; residual norms are still returned.
The normalized RHS norm is used for comparison even when `||b||₂` itself
overflows. A rejected fit returns nil result(s) and `*ResidualError` matching
`ErrResidual`; it records the first failing zero-based RHS index, absolute
`Norm`, `Relative` residual, and `Tolerance`. A relative value beyond float64
range may be +Inf in that error diagnostic, never in a successful result.

RHS transformation, triangular division, actual residual evaluation, or a
residual norm exceeding float64 range returns `ErrNonFinite`. A failed call
never damages the reusable factors. Nonzero subnormal norms are supported, but
finite underflow remains ordinary float64 rounding. In particular, normalization
can lose tiny coordinates of a vector spanning more than the available exponent
range, and cancellation can lose significant digits. Even with exact-zero rank
checking, recovering every coordinate of an extremely ill-scaled system is not
guaranteed. An underflowed mathematical residual entry may also round to zero;
the residual check is on computed values, not exact arithmetic.

Empty designs with n=0 are supported: x is empty and the residual is `||b||₂`.
A 0-by-0 coefficient matrix supports empty vector solves and zero residuals for
0-by-r RHS matrices. A nonempty factorization accepts m-by-0 RHS matrices and
returns n-by-0 results with an empty residual slice. Wide matrices (m<n) and
rank-deficient inputs are errors, including underdetermined inputs for which a
minimum-norm solution might otherwise be defined.

QR factorization costs O(mn²) time and O(mn+n) storage, including **two** owned
m-by-n copies (packed factors and original coefficients). Each r-column solve
costs O(mnr+n²r) time, including original-matrix residual evaluation, and
O(mr+nr+r) new storage. No explicit inverse or normal matrix is needed.

## Cholesky and symmetric positive definite solves

`FactorizeCholesky` requires square input and validates **both** triangles. Its
default policy requires exact elementwise symmetry and sets
`RelativePivotTolerance` to **1e-14**. It never silently trusts or discards the
upper triangle. The resulting private lower factor satisfies `A=L*Lᵀ`, with a
positive diagonal. `Lower` returns a copy; `Solve` and `SolveMatrix` perform
forward and backward triangular substitution, allocating their own workspace.
`SolveSPD`/`SolveSPDMatrix` are one-shot default-policy entry points.

`CholeskyOptions.RelativeSymmetryTolerance` and `RelativePivotTolerance` must
both be finite and in `[0,1)`. For each off-diagonal pair `(a,b)`, exact equality
always passes. Otherwise let `s=max(abs(a),abs(b))`; the symmetry check requires
`abs(a/s-b/s) <= RelativeSymmetryTolerance`. Zero symmetry tolerance rejects
every unequal pair. Scaling before subtraction prevents overflow for pairs
such as `(MaxFloat64,-MaxFloat64)` and avoids an absolute tolerance hiding a
large relative discrepancy near zero.

Approximate symmetry is an explicit change of the matrix being solved:

```go
opt := linalg.DefaultCholeskyOptions()
opt.RelativeSymmetryTolerance = 1e-14
chol, err := linalg.FactorizeCholeskyWithOptions(a, opt)
if err != nil {
    return err
}
// chol describes the symmetric matrix formed by averaging accepted pairs.
```

Accepted unequal pairs are averaged before factoring. Thus reconstruction and
solves describe that **averaged symmetric matrix**, which is not necessarily
the original nonsymmetric input. Equal entries, including subnormals, are kept
exactly. Small unequal entries are summed before halving to preserve subnormal
midpoints; huge entries are halved before summing to avoid an overflowing sum.
The midpoint and factors remain rounded float64 values. Input storage is never
modified. A rejected pair returns nil and `*SymmetryError` matching
`ErrNotSymmetric`, with row/column, `Lower`, `Upper`, `Relative`, and `Tolerance`.

Let `v` be a computed diagonal Schur pivot before square root and `d` the
original diagonal at that position. Rejection occurs if:

```text
d <= 0, or v <= 0, or
(RelativePivotTolerance > 0 and v/d <= RelativePivotTolerance).
```

`*PositiveDefiniteError`, matching `ErrNotPositiveDefinite`, reports the
zero-based pivot, signed `Value`, original diagonal `Scale`, and tolerance.
No square root of a nonpositive value is attempted. A positive computed pivot
below the default threshold can belong to a mathematically SPD matrix; rejection
is a numerical policy, not a proof that a matrix is indefinite. Conversely,
rounding can make a mathematically semidefinite input appear slightly positive.
`CholeskyOptions{}` requires exact symmetry and merely strictly positive
computed pivots, with no relative cutoff. The policy is not a condition-number
estimate or an eigenvalue test. Local diagonal scales allow disparate positive
diagonal matrices without using an inappropriate global absolute cutoff.

Updates and substitution use FMA. Uniformly tiny/huge SPD systems and nonzero
subnormal diagonal values are supported within float64 representability.
Nonfinite updates or divisions return `ErrNonFinite`; a rounded nonpositive
Schur pivot returns `ErrNotPositiveDefinite`. An indefinite input can overflow
during an intermediate computation before its negative Schur pivot is found;
that returns the arithmetic error rather than a false SPD success. Ordinary
finite product/solution underflow remains allowed, just as in Dense/LU. There
is no automatic scaling, pivoted/modified Cholesky, diagonal regularization,
semidefinite solver, condition estimator, or accuracy guarantee for nearly
singular input. Reconstruction and solve residuals should be checked against
the matrix actually factored, especially when approximate symmetry is enabled.

The empty 0-by-0 matrix has an empty valid factor and supports empty/zero-column
RHS shapes. Factorization costs O(n³) time and O(n²) storage; each r-column solve
costs O(n²r) time and O(nr) new storage. Completed factors support concurrent
read-only reuse, including after a previous failed solve.

## Symmetric eigenvalue decomposition

`FactorizeEigenSym` computes descending **algebraic** eigenvalues and matching
orthonormal eigenvector columns. Real symmetric indefinite, repeated-eigenvalue,
rank-deficient and zero matrices are supported, including the empty 0-by-0
matrix. It uses cyclic plane rotations on a private symmetric work matrix and
accumulates V independently. For general nonsymmetric input, use the separate
Schur (real factors/complex eigenvalues) or Eigen (complex right eigenvectors) API.

`DefaultEigenSymOptions()` selects `MaxSweeps: 64`, `RelativeTolerance: 1e-14`
and exact input symmetry. Approximate symmetry must be explicitly enabled with
`RelativeSymmetryTolerance`; it uses the same pairwise scaled difference and
overflow-safe averaging policy as Cholesky. The returned decomposition then
describes the averaged matrix, which can differ from the supplied matrix.

The convergence metric is the maximum absolute off-diagonal entry divided by
the maximum absolute entry of the current work matrix. Convergence is checked
before iteration and after each complete sweep. Entries already below the
relative threshold can remain in the work matrix and are excluded from the
diagonal representation returned to callers. This is an explicit normwise
stopping policy; it is not a relative bound on each eigenvalue. An n-by-n
remaining off-diagonal matrix can have a Frobenius norm up to
`sqrt(n*(n-1))*RelativeTolerance*maxEntry`, before floating-point rotation error.
Zero relative tolerance requests computed exact diagonalization and can exhaust
the budget even when the default normwise policy succeeds.

## One-sided Jacobi SVD

`FactorizeSVD` computes an economy decomposition `A≈U*diag(s)*Vᵀ` with
`k=min(m,n)`, m-by-k U, n-by-k V and k nonnegative descending singular values.
Tall/square input factor shapes and value counts are unchanged; wide input
uses the same one-sided column-rotation core on `Aᵀ`, then swaps U and V. No
Gram matrix (`Aᵀ*A` or `A*Aᵀ`) is constructed or diagonalized. Rank-deficient,
zero and empty matrices are valid. A zero matrix has k zero values with
orthonormal economy columns. With either dimension zero, k=0 and exports
retain the specified empty shapes.

Exact zero columns receive orthogonal complement vectors after all nonzero
columns have converged. Completion tries at most the core's row count of
coordinate axes per zero column, with two modified Gram-Schmidt passes per
candidate. It rejects vanishing candidates, normalizes viable ones and checks
their dot products against existing columns at `1e-14`. Exhaustion returns an
error matching `ErrConvergence`; it never substitutes a zero or nonorthogonal
vector. Bases in zero/repeated singular/eigenvalue spaces and vector signs are
unspecified. Compare reconstruction, subspaces and orthogonality, not literal
vector coordinates. Economy V does not expose an entire right null-space
basis for wide matrices.

`DefaultSVDOptions()` selects 64 sweeps and three **distinct** policies:

| Option | Default | Meaning |
| --- | --- | --- |
| `RelativeTolerance` | `1e-14` | Largest absolute normalized dot product between distinct nonzero working columns after a complete sweep. |
| `RelativeReconstructionTolerance` | `1e-12` | Measured `||A-U*diag(s)*Vᵀ||F/||A||F` against original A before success. |
| `RelativeRankTolerance` | `1e-14` | Retain `s>0` with `s/sMax>tol` for rank, pseudoinverse and solve operations only. |

Convergence is checked on current columns after each whole sweep; later
rotations cannot invalidate an earlier decision. For wide input the working
columns are `Aᵀ*U`. Correlations use separately scaled column norms (reusing
QR's helper) and compensated summation. Angles use norm ratios instead of
unscaled squared norms. Nonzero vectors are normalized with column scale and
scaled norm, avoiding overflowing reciprocals.

In rank-deficient inputs an unresolved numerical residual column can shrink
until its next required rotation is unrepresentable. A positive reconstruction
tolerance permits setting the smaller column to zero only if its norm ratio to
the paired column is within that tolerance. This event increments
`DeflatedColumns()`. It is a disclosed decomposition approximation, independent
of rank selection; a small but already orthogonal column is preserved. Every
successful factorization additionally measures reconstruction against original
A. Excess error returns `*ReconstructionError` matching `ErrReconstruction`,
with `Relative` and `Tolerance`; there are no partial factors. Setting the
reconstruction tolerance to zero forbids residual-column deflation and requires
a computed zero reconstruction residual. An unrepresentable angle then returns
`ErrRange`. The reconstruction check is itself float64: an extremely small
relative error can underflow to zero, and it is not an exact-arithmetic proof.

`RelativeReconstructionError()` reports the measured Frobenius ratio (zero for
an all-zero input). It describes the **untruncated decomposition**, not the
error of a rank-truncated pseudoinverse. The rank cutoff never zeroes values in
`Values()` or removes columns from `Factors()`. For example a diagonal input
`diag(1,1e-6)` with rank tolerance `1e-4` still returns both singular values and
reconstructs both directions; its rank is 1 and its pseudoinverse is
`diag(1,0)`. In that case `A*P*A-A` has norm `1e-6`, a truncation approximation
error, regardless of a much tighter decomposition reconstruction residual.

All tolerances must be finite and in `[0,1)`. Zero options are literal policies,
not aliases for defaults: no sweeps, exact computed orthogonality and
reconstruction, no deflation and zero-only numerical rank cutoff. Start from
`DefaultSVDOptions()` to customize one policy. In particular setting
`RelativeRankTolerance=0` retains all computed positive values, including tiny
roundoff artifacts of a mathematically rank-deficient input; it does not reveal
the exact rank. Rank deficiency no longer returns `RankError` from SVD; that
error remains part of the unchanged full-column-rank QR contract.

Both spectral algorithms return `*ConvergenceError` when the sweep budget is
exhausted. `Algorithm`, `Sweeps`, `MaxSweeps`, `Residual` and `Tolerance` expose
their iteration diagnostics. Orthogonal completion has its own finite candidate
budget and returns an error matching the same sentinel if it fails. Negative
sweep budgets return `ErrBudget`. Nonfinite input/norms/results return
`ErrNonFinite`. All errors leave the input unchanged and return nil factors.

## Truncated pseudoinverse and minimum-norm solves

`Rank()` counts computed values retained by the stored rank cutoff.
`RankWithTolerance(tol)` queries another cutoff without modifying the factors.
`PseudoInverse()` returns independent n-by-m `P=V_r*diag(1/s_r)*U_rᵀ` under the
stored cutoff; `PseudoInverseWithTolerance(tol)` selects another cutoff. For a
zero-rank matrix P is zero. The four Penrose identities describe the retained
matrix `A_r=U_r*diag(s_r)*V_rᵀ`. If a positive value is discarded, the first
identity `A*P*A=A` need not hold for original A. For the computed orthogonal
decomposition, truncation adds a spectral error of the largest discarded
singular value and a Frobenius error of the tail's Euclidean norm, in addition
to decomposition and arithmetic error.

`Solve` and `SolveMatrix` apply the retained factors directly, without forming
P or normal equations. The returned x minimizes `||A_r*x-b||2` and has minimum
Euclidean norm among those least-squares minimizers. With no discarded nonzero
directions, this is the usual minimum-norm least-squares result for A, within
decomposition and rounding error. Nonzero truncation can increase the original
least-squares residual and need not satisfy `Aᵀ*(A*x-b)=0` for original A.
Every returned residual is recomputed as `||A*x-b||2` with the stored **original
input**, so truncation error is visible. `SolveMatrix` returns an n-by-r X and
one norm per RHS column; `SolveWithTolerance` and `SolveMatrixWithTolerance`
override only the rank cutoff for that call. Inputs and outputs are independent.

Products/quotients in pseudoinverse and solve use binary mantissa/exponent
scaling with compensated accumulation, avoiding an overflowing `1/s` or
projection when the final entries fit. Unrepresentable final entries or actual
residual norms return `ErrNonFinite`, with nil outputs. Ordinary underflow and
loss of very small separated contributions still follow float64 rounding;
the accumulator is not arbitrary precision. The implementation avoids squaring
the condition number, but does **not** promise condition-independent relative
accuracy, accurate tiny values for arbitrary ill-conditioned inputs, or exact
rank recovery. Scaling cannot restore information lost in input rounding.
Subnormal rotations may lose relative accuracy or fail to converge. There is
no arbitrary-precision or alternate-algorithm fallback.

Each eigen sweep costs O(n³). Each SVD sweep costs O(max(m,n)*k²), with an
additional O(m*n*k) reconstruction check. Up to k zero columns each try at most
max(m,n) coordinate candidates with two passes against earlier columns; this
completion work is bounded separately from the sweep count. Factors retain
O(m*n+(m+n)*k) storage, including a private original-input copy for residuals.
Pseudoinverse construction costs O(m*n*r), where r is retained rank, and O(m*n)
output storage. For q RHS columns, solve costs O((m+n)*r*q) plus O(m*n*q) to
measure actual residuals, with O(n*q+r+q) new storage. Kernels are unblocked and
serial; the sweep budget does not bound elapsed time. Completed factors allow
concurrent exports, pseudoinverses and solves. Independent factorizations may
read the same immutable Dense input concurrently.

## Errors and validation

All errors support `errors.Is` against package sentinels:

| Sentinel | Meaning |
| --- | --- |
| `ErrDimension` | Negative, mismatched, nonsquare, out-of-bounds, or storage-overflowing dimensions. |
| `ErrNonFinite` | NaN/Inf input or nonfinite arithmetic, including factorization, solve, inverse, or residual-norm overflow. |
| `ErrSingular` | A computed LU pivot was zero or rejected by the selected policy. |
| `ErrTolerance` | A pivot, rank, symmetry, convergence, reconstruction or residual tolerance is outside its documented finite range. |
| `ErrInvalid` | Nil matrix or nil/uninitialized factorization. |
| `ErrRange` | A nonzero determinant, required rotation, Schur scaling/output or normalized eigenvector component cannot be represented within its float64 policy. |
| `ErrRankDeficient` | QR rejected a column tail, or general Eigen with RequireIndependent rejected its numerical column rank; SVD accepts rank deficiency. |
| `ErrResidual` | An enabled least-squares fit limit or a general right-eigenvector residual check failed. |
| `ErrNotSymmetric` | A Cholesky or symmetric eigendecomposition pair failed its symmetry policy. |
| `ErrNotPositiveDefinite` | A Cholesky Schur pivot was nonpositive or rejected by its relative policy. |
| `ErrBudget` | A Jacobi/Schur iteration count is negative or a Schur MaxSize option is invalid. |
| `ErrConvergence` | Jacobi/Schur iteration or SVD orthogonal completion exhausted its finite budget. |
| `ErrReconstruction` | Measured SVD reconstruction or Schur reconstruction/orthogonality error exceeds its tolerance. |
| `ErrSchurLimit` | The Schur matrix order or cumulative scalar-work budget was exceeded. |
| `ErrEigenLimit` | The general-eigenvector order/combined-work bound was exceeded; Schur-stage limits also match ErrSchurLimit. |
| `ErrComplexLimit` | A separate complex matrix/LU dimension, element or per-call work limit was exceeded. |

`errors.As(err, &singular)` with a `*linalg.SingularError` exposes the zero-based
elimination step, signed pivot, original row scale, and requested tolerance.
No partially computed factorization, matrix, or vector escapes on failure.
Scalar numerical errors return zero with a nonnil error; callers must check it.
Nonfinite values cannot be inserted through the public matrix API. Arithmetic
and solve outputs are checked before return; rejected data never silently
becomes a successful NaN result.

Focused tests include rectangular and zero-sized arithmetic, ownership of all
public data exports, multiple successive row pivots, `P*A=L*U` reconstruction,
known solutions, repeated and concurrent factor reuse, multi-RHS solves,
left/right inverse residuals, and randomized diagonally dominant systems at
sizes 1, 2, 7, 31, and 64. They also check singular and near-singular policies,
invalid tolerance/shape/index/nil/nonfinite inputs, row scales after swaps,
uniform scaling by `1e-300` and `1e300`, ill-scaled diagonal and subnormal
systems, Hilbert residuals, determinant signs against independent permutation
expansion, determinant range handling, and arithmetic-overflow rejection.
The new QR tests independently check reconstruction with column permutations,
Qᵀ*Q, residual norms, and least-squares first-order stationarity; consistent and
inconsistent multi-RHS fits, reflected near dependence, rank boundaries,
nil/shape/tolerance errors, empty designs, residual-limit diagnostics, input and
factor-export ownership, scaled and subnormal norms, avoidable and unavoidable
overflow, and solution underflow are covered. Cholesky tests include a known
exact lower factor, independent random SPD fixtures at sizes 1 through 64,
reconstruction and solve residuals, repeated/multi-RHS and concurrent reuse,
indefinite/semidefinite and nearly positive pivots, strict/approximate symmetry,
tiny/huge midpoint handling, and arithmetic/solution range limits. Executable
examples demonstrate the direct factor-and-solve APIs. Spectral tests include
analytic indefinite/repeated/zero and Toeplitz eigenvalue spectra, analytic
rectangular SVD, deterministic random reconstruction and orthogonality, sine
transform bases with known spectra spanning condition numbers through `1e12`,
structured near dependence down to `1e-18`, strict/approximate symmetry, explicit
rank thresholds, exhausted/negative budgets, zero/empty shapes, finite/range
rejection, uniformly scaled inputs, subnormals and unrepresentable angles.
Private overlay tests verify actual independent factor storage and nonfinite
input rejection; concurrency tests exercise reuse and independent simultaneous
factorizations. Economy SVD tests independently check tall/wide/zero/empty
shapes, bounded zero-vector completion, known integer ranks, all four Penrose
conditions, analytic pseudoinverses and minimum-norm multi-RHS solutions,
least-squares stationarity and null-space orthogonality, truncation versus
decomposition error, reconstruction diagnostics, retained original residuals,
concurrent inverse/solve reuse and representability limits. The focused package
suite covers 96.4% of statements as measured
on 2026-11-04; this is not a numerical accuracy or capability-completeness metric.

All test and benchmark source lives in `tests/_cases/linalg/`. The central
runner overlays those files into the original package during compilation,
including private tests, without copying test files into production directories.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go vet -tags noasm ./linalg
```

## Allocation and timing samples

Recorded 2026-11-03 on Apple M2, darwin/arm64, Go 1.27.3; module language version
is Go 1.26.0. Benchmarks use the normal default GOMAXPROCS (reported suffix `-8`),
`CGO_ENABLED=0`, and `-tags noasm`. No rendering is involved, so image size and
frame count are not applicable. These are observations on a shared development
machine with concurrent work, not comparisons with another numerical library
or universal performance guarantees.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -tags noasm -p 1 -run '^$' -bench . -benchmem -benchtime=150ms -count=3
```

The following original dense/LU samples use deterministic, row-permuted,
strictly diagonally dominant dense matrices. Multiplication computes `A*A`.
Vector RHS has `b[i]=i+1`; multi-RHS
has eight columns with small signed integer entries. All three observed times
are shown in run order, in **ns/op**. Bytes and allocation counts are per
operation; tiny B/op variations between runs are retained as ranges.

| Operation | n / RHS | Three samples (ns/op) | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| Multiply | 8 | 650.9 / 440.3 / 362.7 | 560 | 2 |
| Multiply | 32 | 12948 / 12927 / 12900 | 8240 | 2 |
| Multiply | 128 | 860119 / 870521 / 862102 | 131120 | 2 |
| Multiply | 256 | 6354985 / 6312728 / 6330449 | 524336 | 2 |
| Factorize | 8 | 789.2 / 663.7 / 635.6 | 720 | 4 |
| Factorize | 32 | 18616 / 18470 / 18639 | 8784 | 4 |
| Factorize | 128 | 772092 / 782329 / 772415 | 133200–133201 | 4 |
| Factorize | 256 | 5267121 / 5379183 / 5782878 | 528464–528467 | 4 |
| Reused solve | 8 / 1 | 267.7 / 214.9 / 214.1 | 64 | 1 |
| Reused solve | 32 / 1 | 2984 / 5050 / 3485 | 256 | 1 |
| Reused solve | 128 / 1 | 53261 / 52422 / 52130 | 1024 | 1 |
| Reused solve | 256 / 1 | 234708 / 274596 / 219169 | 2048 | 1 |
| Reused multi-RHS solve | 8 / 8 | 448.7 / 445.7 / 447.0 | 560 | 2 |
| Reused multi-RHS solve | 32 / 8 | 5280 / 5244 / 5251 | 2096 | 2 |
| Reused multi-RHS solve | 128 / 8 | 175887 / 257923 / 98072 | 8240 | 2 |
| Reused multi-RHS solve | 256 / 8 | 303710 / 301318 / 303670 | 16432 | 2 |
| Reused inverse | 8 | 426.2 / 426.7 / 424.4 | 560 | 2 |
| Reused inverse | 32 | 14616 / 18292 / 18092 | 8240 | 2 |
| Reused inverse | 128 | 939901 / 926868 / 1234346 | 131120 | 2 |
| Reused determinant | 32 | 197.7 / 172.5 / 174.1 | 0 | 0 |

Fixture construction is excluded from timing. Factorize includes its input
copy, scale/permutation workspaces, and output factor object. Reused solve,
inverse, and determinant benchmarks exclude initial factorization. All timed
matrix/vector result allocations, substitution workspaces, and finite-result
checks are included. There is no per-call shared scratch storage, allocation
pool, destination-buffer API, or hidden runtime fallback. Constructor,
transpose, and elementwise-operation performance is not measured by this suite.

### QR and Cholesky samples

These additional measurements use the same 2026-11-03 Apple M2 / darwin-arm64 /
Go 1.27.3 environment and pure-Go flags. They are observations during concurrent
development, not a comparison or a claim of BLAS-level performance. QR uses
deterministic random m-by-n matrices with an added dominant diagonal in the
leading n rows, with m=2n. Cholesky uses independently generated symmetric
`G*Gᵀ+I` SPD fixtures. These Gram matrices are **test/benchmark inputs only**;
the QR least-squares implementation does not form normal equations.
RHS entries repeat small signed integers (`i%17-8`); multi-RHS cases have eight
columns. No rendering occurs, so image size and frame count are not applicable.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -tags noasm -p 1 -run '^$' -bench 'Benchmark(QR|Cholesky)' -benchmem -benchtime=150ms -count=3
```

All three observed samples are in **ns/op**, in run order. Fixture generation
is excluded; factorization includes owned copies/workspaces and validation.
Reused solves exclude factorization and include all output/workspace allocation,
finite checks, and (for QR) actual residual evaluation. Optional residual-limit
checking is disabled in these timing samples. Exporting Q/R or L is not timed.

| Operation | m / n / RHS | Three samples (ns/op) | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| QR factorize | 16 / 8 | 28796 / 21215 / 14057 | 2304 | 5 |
| QR factorize | 64 / 32 | 876757 / 876845 / 873584 | 33408 | 5 |
| QR factorize | 256 / 128 | 54494000 / 57959070 / 66428153 | 526464 | 5 |
| Reused QR solve | 16 / 8 / 1 | 1387 / 1370 / 1625 | 248 | 4 |
| Reused QR solve | 64 / 32 / 1 | 20677 / 19533 / 19473 | 824 | 4 |
| Reused QR solve | 256 / 128 / 1 | 305270 / 308504 / 329784 | 3128 | 4 |
| Reused QR multi-RHS | 16 / 8 / 8 | 10706 / 9468 / 9460 | 1648 | 4 |
| Reused QR multi-RHS | 64 / 32 / 8 | 150092 / 154097 / 154295 | 6256 | 4 |
| Reused QR multi-RHS | 256 / 128 / 8 | 2486849 / 2663841 / 3392341 | 24688 | 4 |
| Cholesky factorize | 8 / 8 | 316.6 / 283.4 / 267.4 | 608 | 3 |
| Cholesky factorize | 32 / 32 | 6031 / 9742 / 5519 | 8288 | 3 |
| Cholesky factorize | 128 / 128 | 321389 / 323879 / 327757 | 131168 | 3 |
| Reused Cholesky solve | 8 / 8 / 1 | 235.7 / 231.8 / 280.3 | 64 | 1 |
| Reused Cholesky solve | 32 / 32 / 1 | 3293 / 3107 / 3083 | 256 | 1 |
| Reused Cholesky solve | 128 / 128 / 1 | 52297 / 52224 / 52290 | 1024 | 1 |
| Reused Cholesky multi-RHS | 8 / 8 / 8 | 478.8 / 480.2 / 488.3 | 560 | 2 |
| Reused Cholesky multi-RHS | 32 / 32 / 8 | 5363 / 6323 / 5871 | 2096 | 2 |
| Reused Cholesky multi-RHS | 128 / 128 / 8 | 77977 / 79103 / 78734 | 8240 | 2 |

QR's fresh pivot-norm recomputation and robust per-column scaling cost repeated
column passes; it is an unblocked correctness-oriented implementation. No
parallel/blocked kernel or performance equivalence to optimized native QR is
claimed. Cholesky validates both triangles and retains a full zero-upper dense
lower factor rather than packed triangular storage.

### Spectral factorization samples

The following table records the earlier tall/full-column-rank implementation,
before the economy/solve extension. Its SVD figures exclude the original-input
copy and final reconstruction check added by that extension; they are historical
samples, not measurements of the current SVD implementation. These old
benchmarks were not repeated for the economy extension.

Recorded 2026-11-04 on Apple M2, darwin/arm64, Go 1.27.3, module language
version Go 1.26.0, with default GOMAXPROCS (`-8`), `CGO_ENABLED=0` and
`-tags noasm`. Package builds are serialized with `-p 1`; benchmark iterations
are serial. Input generation is excluded. Eigen inputs are deterministic
Gaussian symmetric matrices; SVD inputs are deterministic Gaussian rectangular
matrices with m=2n. Timings include all factorization allocations, sweeps,
sorting and stored factors, and exclude factor exports, reconstruction and
input creation. No rendering is involved (image size/frame count: not applicable).
The shared machine was running concurrent development work; these are timing
samples, not speedup or parity claims.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -tags noasm -p 1 -run '^$' -bench 'Benchmark(EigenSym|SVD)Factorize' -benchmem -benchtime=150ms -count=3
```

| Operation | m / n | ns/op, three samples | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| Symmetric eigen factorize | 8 / 8 | 15024 / 14820 / 14565 | 1896 | 10 |
| Symmetric eigen factorize | 32 / 32 | 548711 / 552588 / 552329 | 25512 | 10 |
| One-sided Jacobi SVD | 16 / 8 | 277739 / 284542 / 276565 | 3464 | 11 |
| One-sided Jacobi SVD | 64 / 32 | 22904774 / 23535893 / 23013619 | 50120 | 11 |

SVD recomputes scaled column norms for every candidate pair and separately
checks convergence after each sweep. This prioritizes explicit convergence
and scaling behavior over caching or blocked-kernel performance.

### Economy SVD and inverse/solve samples

Recorded 2026-11-04 on Apple M2, darwin/arm64, Go 1.27.3, module language
version Go 1.26.0. `CGO_ENABLED=0`, `-tags noasm`, default GOMAXPROCS (`-8`),
serial iterations and `-p 1` were used. The common input is a deterministic
16-by-32 integer matrix formed from rank-eight factors; the solve uses four
RHS columns. Input construction is excluded. The economy factorization includes
transpose handling, sweeps, any zero-column completion, the original-matrix
reconstruction check and owned storage. Pseudoinverse/solve reuse an existing
factorization; their timings exclude factorization and factor exports. Solve
timings include actual residual evaluation against original A. Image size and
frame count do not apply. No comparison with native code or the earlier
full-rank workload is implied; the development machine was shared.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -tags noasm -p 1 -run '^$' -bench 'BenchmarkSVD(EconomyWideRankDeficient|PseudoInverseReuse|SolveMatrixReuse)$' -benchmem -benchtime=150ms -count=3
```

| Operation | m / n / rank / RHS | ns/op, three samples | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| Wide rank-deficient economy factorization | 16 / 32 / 8 / — | 3978331 / 3935800 / 3857804 | 21160 | 14 |
| Reused SVD pseudoinverse | 16 / 32 / 8 / — | 73992 / 118569 / 75059 | 4144 | 2 |
| Reused SVD four-RHS solve | 16 / 32 / 8 / 4 | 48003 / 45881 / 54544 | 1233 / 1232 / 1232 | 4 |

Solve uses exponent-carrying projections, preserving separable RHS coordinates
even when their magnitudes span `1e-300` to `1e300`; this does not guarantee
accuracy for arbitrary ill-conditioned, mixed coordinates.

## General real Schur decomposition

```go
a, err := linalg.NewDense(3, 3, []float64{
    0, -2, 1,
    2,  0, 3,
    0,  0, 5,
})
if err != nil { return err }
opt := linalg.DefaultSchurOptions()
opt.MaxSize = 64
opt.MaxIterations = 2048
f, err := linalg.FactorizeSchurWithOptions(a, opt)
if err != nil { return err }
q, t, err := f.Factors() // A ≈ Q*T*Qᵀ; independent real Dense matrices
if err != nil { return err }
values := f.Values() // complex128 copy: ±2i and 5, in Schur block order
_, _, _ = q, t, values
```

This implements **real Schur factors and complex eigenvalues**, including
nonsymmetric, singular, repeated and defective square matrices. The Schur API
does not compute general eigenvectors; use the separate Eigen API below.
Columns of Q are orthonormal **Schur vectors**:
for a nonnormal matrix they generally are not individual eigenvectors. Empty
input returns valid empty Q/T and an empty eigenvalue slice. Zero input returns
Q=I and T=0. Constructors, factor exports and eigenvalue exports own independent
storage; input is preserved on success and error. A completed factor and its
input matrix can be read/exported/factorized concurrently.

T is exactly zero below the first subdiagonal. Nonzero subdiagonals occupy
disjoint 2×2 blocks with complex-conjugate roots; real pairs are reduced to 1×1
blocks by an orthogonal plane rotation. A complex block's two diagonals need
not be equal: this API does not promise LAPACK's additional canonical 2×2
normalization. Values follow the resulting blocks without sorting; each complex
pair has its positive imaginary value first. Block order, basis and signs are
unspecified, particularly for repeated roots. There is no balancing, spectral
reordering or generalized matrix pencil API. Right eigenvector recovery is a
separate factorization against the original matrix, described below.

### Algorithm and numerical policy

1. Check square/finite input and option/resource bounds, then copy and scale by
   an exact power of two derived from the largest entry. Reject a scale that
   cannot be reversed without losing a nonzero entry's precision.
2. Reduce to upper Hessenberg form using two-sided Householder similarities,
   accumulating the same right transformations in Q. Axis-aligned reflectors
   that are signed permutations use exact swaps/signs, preserving strict
   nilpotent/Jordan structures without artificial roundoff roots.
3. Apply implicit Francis double-shift QR to each trailing unreduced block,
   chasing length-three Householder bulges (length two at the end). Every
   twentieth stalled sweep uses a real exceptional shift; an identically zero
   starting polynomial uses an implicit single-shift step. All steps are
   orthogonal similarities. There is no explicit polynomial matrix or dense QR
   factorization at each iteration.
4. Deflate a subdiagonal only after a local relative test and a conservative
   Ahues–Tisseur product criterion. Product comparison carries mantissas and
   exponents, so underflow cannot make two positive products look like zero.
   There is no absolute tiny-entry floor that discards a meaningful small
   conjugate pair. Zero tolerance requires exact zero subdiagonals.
5. Finish 2×2 blocks using scaled discriminants, compensated products near
   repeated roots, and square-root/Hypot formulas that avoid squaring tiny
   off-diagonals. Real pairs are triangularized; complex pairs stay real 2×2.
6. Round T to its actual output float64 scale **before** checking
   `||A-Q*T*Qᵀ||F/||A||F` and `||QᵀQ-I||F`. Checks themselves use scaled
   arithmetic. This catches inaccurate subnormal output even when the internal
   scaled decomposition was accurate. Check failures return nil factors.

`RelativeReconstructionError` and `OrthogonalityError` report those measured
checks. The norm for zero A is defined to have zero relative error. T and each
real/imaginary eigenvalue component must fit float64; irreversible global/local
scaling, nonzero output underflow to zero, or overflow returns `ErrRange`.
Subnormal rounding that stays nonzero must still pass the actual-output
reconstruction check. Extremely mixed coordinate scales may therefore be
rejected even when the mathematical eigenvalues would fit. Homogeneous scales
`1e-300..1e300` and exact minimum-subnormal rotation blocks are tested.

These are backward reconstruction/orthogonality policies, **not a forward
relative eigenvalue accuracy guarantee**. Defective or nearly coalescent roots
can move substantially under small float64 perturbations. No exact multiplicity,
Jordan form, diagonalizability or arbitrary relative accuracy of tiny roots is
claimed. The selected deflation tolerance can affect those roots; final residual
checks remain independent and may reject an overly loose deflation policy.

### Options, resource bounds and errors

| SchurOptions field | Default constructor | WithOptions semantics |
| --- | --- | --- |
| `MaxIterations` | 4096 | Total implicit QR sweeps across all blocks; negative invalid, zero permits only reduction/deflation/direct 1×1/2×2 finishing. |
| `RelativeTolerance` | `8*2^-52` | Finite `[0,1)` local deflation tolerance; zero is literal. |
| `ReconstructionTolerance` | `1e-12` | Finite `[0,1)` relative Frobenius bound; zero requires exact computed reconstruction. |
| `OrthogonalityTolerance` | `1e-12` | Finite `[0,1)` absolute Frobenius bound on QᵀQ−I; zero is literal. |
| `MaxSize` | 256 | Zero selects 256; otherwise 1..1024. Input order above the selected bound is rejected. |
| `MaxWork` | `256<<20` | Zero selects the default; uint64 cumulative scalar-work budget. |

Start from `DefaultSchurOptions()` before changing individual policies. An
otherwise zero `SchurOptions{}` retains literal zero iteration/tolerances;
it is not shorthand for the default numerical policy.

The budget is a deterministic operation-count model, not a wall-time or CPU
instruction guarantee. Before allocating factors, reserve `3*n³+8*n²` units for
three final cubic products and scans. Each nonzero length-m reflector (including
an exact signed permutation) costs `6*n*m+8*m`; each direct real 2×2 rotation
costs `12*n+32`. Every active-block scan costs `n+1`. Zero Hessenberg vectors
require no matrix update. The charge is checked before each corresponding
transform; iteration exhaustion and work exhaustion are independent errors.
Hessenberg reduction is O(n³), each full active QR sweep O(n²), and final
validation O(n³). Scalar work uses uint64 with order capped at 1024, preventing
count-product overflow. Raising order generally requires raising work as well.

Reachable internal matrix storage peaks at three n×n float64 arrays (Q, T and
one check product), plus O(n) reflector/eigenvalue storage; input remains owned
by the caller. Default order 256 bounds these three arrays to 1.5 MiB; the hard
order 1024 bounds them to 24 MiB. GC, allocator overhead, the caller's input and
exported factor copies are additional; there is no operating-system memory quota
or out-of-memory recovery. Returned factors retain two n×n arrays and n complex
values. There is no retained original matrix and no runtime dependency beyond
Go's standard library.

| Failure | Error contract |
| --- | --- |
| nil / nonsquare / nonfinite input | Existing `ErrInvalid` / `ErrDimension` / `ErrNonFinite`. |
| invalid tolerances / negative iteration or invalid MaxSize option | Existing `ErrTolerance` / `ErrBudget`. |
| order or cumulative work bound exceeded | `ErrSchurLimit`. |
| QR iteration budget exhausted | `*SchurConvergenceError`, matching existing `ErrConvergence`, with total counts and inclusive active indices. |
| output precision/range cannot be represented | Existing `ErrRange`. |
| reconstruction or orthogonality check failed | `*SchurValidationError`, matching existing `ErrReconstruction`, with Check, ErrorValue and Limit. |

Every failure returns nil factors; none exposes a partial Schur form or switches
to another runtime. Existing error sentinels and other factorization APIs are
preserved. Nil/zero-value Schur factors return nil Values, zero diagnostics and
ErrInvalid from Factors; a valid empty factorization must use the constructor.

This is an original GoMath implementation under the repository MIT license.
Algorithmic references are Francis implicit double-shift QR and Ahues/Tisseur,
*A New Deflation Criterion for the QR Algorithm* (LAPACK Working Note 122).
No LAPACK or other implementation source is copied or linked; existing
repository licenses/notices are preserved.

### Independent checks and focused commands

`tests/reference/linalg_schur.py` generated 56 checked-in cases with NumPy 2.3.5
and SciPy 1.17.1 (seed 9639). Ordinary tests read only
`tests/data/linalg/schur_scipy.json`; they never import or launch Python/SciPy.
Orders 0..64 cover random, mixed real/complex, repeated/defective, cyclic,
nilpotent, Grcar and companion matrices and scales through `1e±300`.
Go tests independently verify all Q/T entries' structure, orthogonality and
reconstruction against original A, and match the full complex eigenvalue
multiset without assuming order. Analytical tests additionally require exact
strict Jordan roots, minimum-subnormal conjugate roots, compensated near-real
discriminants, and tiny nonzero imaginary parts on nonzero real centers.

For these 56 cases, measured maximum relative reconstruction error was
`3.06238e-14` and maximum `||QᵀQ-I||F` was `7.68068e-14`. Root comparison
tolerances are explicit per case; nearly defective references use conditioned
tolerances rather than asserting a tight forward error. These figures describe
the tested matrix set, not every matrix allowed by the API.

Ownership/concurrency, exact budget boundaries, invalid options, nonconvergence,
range/underflow and post-check failures are also tested. The central test entry
point puts fuzz failures under `tests/data/linalg/testdata/fuzz/`. Initial
20-second fuzzing executed 1,104,166 cases; a 10-second run after output-rounding
validation changes executed 444,826 cases, both passing. Tests use bounded orders
1..8, up to 256 QR iterations and 2M work units per fuzz input.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -run='^$' -fuzz='^FuzzSchur$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 go vet -tags noasm ./linalg
GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -tags noasm ./linalg
python3 tests/reference/linalg_schur.py # development-only reference generation
```

Full focused tests/race (including `-vet=all`), package vet and the 32-bit
cross-build passed; final whole-linalg statement coverage was 96.2%. Dependency
inspection reports only `gomath/linalg` outside the standard library. No other
production package, root/CLI/export integration or Python reference was edited.

### Schur benchmark samples

Recorded 2026-11-04, Go 1.27.3, darwin/arm64, Apple M2, default GOMAXPROCS=8,
`CGO_ENABLED=0`, runner `noasm`, serial iterations and `-p=1`:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -run='^$' -bench='^BenchmarkSchur$' -benchmem -benchtime=300ms -count=3 -p=1
```

Inputs are independent seeded Gaussian n×n matrices, PCG seeds (9639,n).
Timing includes copying/scaling, Hessenberg reduction, all QR steps, eigenvalue
extraction, output range handling, final cubic reconstruction/orthogonality
checks and stored factors. It excludes input creation, factor exports, program
startup, disk and NumPy/SciPy. Image dimensions/video frame counts do not apply.

| Order | Three ns/op samples | B/op | allocs/op |
| --- | --- | --- | --- |
| 8 | 19438 / 21534 / 19411 | 1888 | 7 |
| 32 | 803326 / 810705 / 805053 | 25504 | 7 |
| 64 | 5745829 / 5694714 / 5735605 | approximately 100000 | 7 |

The development machine was shared. These are measured cost samples, without
claims of native-LAPACK speed, universal convergence or relative root accuracy.

## General real-matrix complex right eigenvectors

```go
a, err := linalg.NewDense(3, 3, []float64{
    0, -2, 1,
    2,  0, 3,
    0,  0, 5,
})
if err != nil { return err }
opt := linalg.DefaultEigenOptions()
opt.Schur.MaxSize = 64
opt.RequireIndependent = true // numerical rank policy, not an exact proof
f, err := linalg.FactorizeEigenWithOptions(a, opt)
if err != nil { return err }
values := f.Values()
v, err := f.RightVector(0) // A*v ≈ values[0]*v, complex128 column copy
if err != nil { return err }
_, _, _ = values, v, f.Residuals()
```

`FactorizeEigen` accepts general square real Dense matrices, including empty,
zero, singular, real/complex spectra and repeated/defective roots. It builds
and validates real Schur factors internally, then computes **right eigenvectors**.
These are distinct from the orthogonal Schur columns Q. Nonnormal-matrix right
eigenvectors generally are not orthogonal, and no inverse-V or diagonalization
API is provided. There is no left/generalized eigenvector API or external runtime.
Input storage is never modified; all slice exports are independent copies.
Completed factors support concurrent reads/exports and parallel factorizations
of the same immutable Dense. Nil/zero-value Eigen is invalid; introspection
returns zero/nil and RightVector returns ErrInvalid. An empty factor is valid,
has order/rank zero and no vector columns. Zero n×n produces unit coordinate
columns, not zero vectors.

### Column ordering, phase and dependency

Eigenvalues are in the internally computed Schur block order, without sorting.
For every complex pair, the positive-imaginary member comes first, followed by
its exact complex conjugate. The second vector column is constructed by exact
complex conjugation of the first. Real-root columns are real-valued. Every
column has Euclidean norm one to float64 rounding; the first largest-magnitude
component selected before phase rotation is made real and nonnegative. Ties
and rounding can change which component appears largest after rotation.

Signs/phases, block order and the basis within a repeated-root eigenspace are
not unique. Do not compare repeated-root columns to a uniquely prescribed
basis. Every returned column passes the specified residual check; this does
not guarantee relative forward accuracy of tiny roots or vectors of an
ill-conditioned matrix.

`BasisRank()` measures **the numerical rank of the exported columns** under
`RelativeBasisTolerance`, using column-pivoted two-pass modified Gram-Schmidt.
Pivots at or below tolerance times the first/largest pivot are discarded. This
is not the true geometric multiplicity, a singular-value condition number, or
an exact mathematical test for diagonalizability. Even full numerical rank
cannot prove that the original finite input is exactly diagonalizable: a
perturbation of a defective matrix can split repeated roots. A highly
unbalanced but mathematically diagonalizable conjugate pair can have numerical
column rank one under the default cutoff.

By default dependent columns are returned **with this rank diagnostic**, without
claiming a complete independent eigenbasis. For instance `[[1,1],[0,1]]` has
rank-one output under the default policy. `RequireIndependent: true` rejects
rank below n with ErrRankDeficient and returns nil. This option imposes only
the documented numerical policy; it is not an exact algebraic certificate.
No generalized eigenvectors or guaranteed complete basis of each defective
eigenspace are supplied. With zero basis tolerance, roundoff artifacts may be
counted as independent columns, as in other numerical rank APIs.

### Algorithm, range and error checks

1. Reserve the bounded vector-stage work before allocating factors. Internally
   call the existing Schur factorization with a capped remaining work budget.
   Its square/finite, reversible power-of-two scaling, QR convergence, output
   representability, original-A reconstruction and Q orthogonality policies all
   remain active. This API does not weaken Schur tolerances implicitly.
2. For each real 1×1 block, seed its coordinate vector. For a complex 2×2 block,
   construct a complex null vector from the stronger shifted row. Blocks may
   have unequal diagonals; LAPACK-style equal diagonals are not assumed.
3. Solve preceding quasi-triangular blocks backwards. Complex 2×2 solves use
   complete pivoting and elimination, avoiding a cancellation-prone explicit
   determinant quotient. Each real and imaginary scalar carries its own
   float64 mantissa and integer exponent; a tiny imaginary shift is preserved
   when the real diagonal cancels. Products/quotients can have exponents beyond
   the float64 range without prematurely overflowing or underflowing.
4. A zero scalar pivot/zero final 2×2 pivot with zero right side selects a zero
   free component. An **exactly zero** pivot with a nonzero right side is replaced
   by `2^-52 * max(abs(T entries))`, carried in exponent form even below subnormal
   range. `RegularizedColumns()` explicitly flags every affected column and its
   conjugate. Nonzero tiny pivots are not floored: e.g. `1±1e-300 i` remains a
   complex pair. These are approximate eigenvectors subject to the same final
   residual policy. Regularization does not create a missing eigenbasis.
5. Apply Q with compensated exponent-carrying dot sums, normalize and fix phase.
   A computed nonzero component that cannot fit the normalized complex128 output
   returns ErrRange; it is not silently dropped. A long exact Jordan chain can
   therefore fail explicitly rather than exporting underflowed artificial tails.
   Mantissas are still float64: separated contributions can round away and
   cancellation can cause loss of relative precision. This is not arbitrary
   precision eigenvector recovery.
6. Independently check **every actual output column**, including conjugates,
   against the original input A. The relative residual is

   `||A*v - lambda*v||2 / ((||A||F + |lambda|) * ||v||2)`.

   An exact zero numerator gives zero; a nonzero relative residual too tiny
   for float64 is retained as the minimum positive subnormal. The zero-input
   exact equation has error zero. The criterion is normwise backward residual,
   not eigenvalue/vector forward error or relative accuracy of each matrix entry.
7. Compute numerical column rank; enforce RequireIndependent when selected.
   All errors return nil, with no partial columns/factors.

For step 6 only, each real/imaginary row residual is summed with a **fixed
4352-bit** integer-bucket accumulator (`math/bits`, four arrays of
68 uint64 words for real/imaginary positive/negative sums). Products and sums of the finite binary64
inputs are exact at this precision for the maximum order 1024: possible product
bits span at most 4196 binary positions, with fewer than 2×1024+2 terms. The
residual norm and final ratio are then rounded through exponent-carrying
float64 arithmetic. This bounded accumulator prevents large cancellation from
concealing an extremely tiny nonzero residual under a literal zero tolerance.
It does not increase eigenvalue or backsolve precision, invokes no native code,
and has no input-selected precision or unbounded convergence loop.

| EigenOptions field | Default | Explicit semantics |
| --- | --- | --- |
| Schur | DefaultSchurOptions() | All existing Schur policies, including MaxSize=256 and MaxIterations=4096. |
| MaxWork | 512M | Combined model below; zero selects default. |
| RelativeResidualTolerance | 1e-12 | Finite [0,1); zero requires an exactly zero computed numerator. |
| RelativeBasisTolerance | 1e-10 | Finite [0,1); relative numerical rank pivot cutoff. |
| RequireIndependent | false | true rejects numerical rank below n. |

Start from `DefaultEigenOptions()` before changing policies. A zero
`EigenOptions{}` retains literal zero Schur iterations/tolerances and literal
zero eigen residual/rank tolerances, while zero size/work fields select their
documented defaults.

The vector stage conservatively reserves `128*n³ + 64*n² + 32*n` work units,
including backsolves, transformation, normalization, rank and residual checks.
A unit covers a bounded arithmetic kernel/loop term, including the fixed-size
checker; it is a deterministic resource model, **not a FLOP or wall-time count**.
Schur additionally charges its existing scalar-update model. Its budget is capped
by the smaller of its explicit/default MaxWork and the combined amount remaining
after the reserve. The reserve is intentionally conservative for zero/diagonal
matrices and for conjugate pairs. MaxSize and work limits apply before factor
allocation; increasing size without increasing work does not promise acceptance.
For example order 256 exceeds the default combined reserve alone, while order
128 can fit it if QR work stays within the remainder. Maximum configurable order
remains 1024; changing it requires changing the nested Schur policy. All cubic
bounds are evaluated after the size bound, safely on 32-bit architectures.

Storage is O(n²), with copied complex columns, Schur factors and a rank workspace;
fixed-precision residual scratch is reused across rows. Exports allocate fresh
copies. There is no OOM recovery or universal convergence guarantee.

| Failure | Result |
| --- | --- |
| nil, nonsquare, nonfinite input | ErrInvalid, ErrDimension or ErrNonFinite. |
| invalid tolerances / iteration or size policy | ErrTolerance / ErrBudget. |
| combined reserve or order exceeded | ErrEigenLimit; order also matches ErrSchurLimit. |
| Schur-stage work exhausted | Both ErrEigenLimit and ErrSchurLimit. |
| Schur nonconvergence / factor post-check failure | Existing SchurConvergenceError / SchurValidationError. |
| unrepresentable scaling, output or normalized vector | ErrRange. |
| original-A eigenpair residual above tolerance | EigenResidualError matching ErrResidual, with Column, Residual and Limit. |
| RequireIndependent and rank<n | ErrRankDeficient. |
| invalid vector column index | ErrDimension, nil column. |

### Independent references and validation

`tests/reference/linalg_eigen.py` creates the checked-in
`tests/data/linalg/eigen_scipy.json`: 30 cases from NumPy 2.3.5 / SciPy 1.17.1,
NumPy RNG seed 39521. Distinct, well-conditioned roots compare normalized vector
directions up to complex phase. Repeated semisimple cases compare reference
invariant-subspace projectors, eigenvalue-group dimension and full numerical
column rank, without prescribing unique columns. Cases include unequal-diagonal
complex blocks, nearly real/coalescent conjugates, nonnormal random matrices of
orders 2..32, nonorthogonal repeated-root similarities and scales 1e-300..1e300.

All 56 existing Schur fixtures additionally run through vector recovery and
original-A residual validation, including defective/Jordan/companion/Grcar and
order-64 matrices. Existing fixtures are preserved. Analytical tests separately
cover empty/zero/negative scalars, exact repeated real/complex semisimple roots,
nonorthogonal real eigenvectors, defective real/complex blocks, tiny imaginary
shifts, unbalanced pairs, minimum-subnormal through maximum-float rotations,
strict independence rejection, unrepresentable vector tails, exact work
boundaries, literal-zero residual rejection and the full binary64 product
exponent span. Ownership/concurrent exports/refactorizations are tested.

Ordinary tests read local JSON and require no Python, SciPy or subprocess.
Reference generation is development-only:

```sh
python3 tests/reference/linalg_eigen.py
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -race -count=1 -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -run='^$' -fuzz='^FuzzGeneralEigen$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 go vet -tags noasm ./linalg
GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -tags noasm ./linalg
```

The focused test suite/race, vet and 32-bit cross-build passed; whole-linalg
statement coverage was 96.5%. Initial 20-second fuzzing executed 449,592 inputs;
a second 20-second run after the fixed integer accumulator change executed
432,233 inputs, both passing. Fuzz order is bounded 1..8, QR at 256
iterations and combined work at 2M; uniform power-of-two scale exponents vary
from -1000 through 1000. Failure corpus belongs under
`tests/data/linalg/testdata/fuzz/`, never production. Dependency audit reports
only `gomath/linalg` outside the standard library. No root/CLI/export, external
Python source or other package was changed by this slice.

### General eigenvector benchmark samples

Recorded 2026-11-04, Go 1.27.3, darwin/arm64, Apple M2, default GOMAXPROCS=8,
CGO_ENABLED=0, runner noasm, serial execution and `-p=1`:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -run='^$' -bench='^BenchmarkGeneralEigen$' -benchmem -benchtime=300ms -count=3 -p=1
```

Inputs are seeded Gaussian n×n matrices, PCG seeds (39521,n). Timing includes
copying/internal Schur, original-A Schur checks, complex block backsolves,
Q transformations, normalization/phase, all exact-product eigenpair residual
checks, numerical column-rank checks and immutable stored result allocation.
It excludes input creation, result exports, startup, disk and NumPy/SciPy;
image dimensions/frame counts do not apply. Work is O(n³) beyond the bounded
Schur iteration stage; storage is O(n²), with a fixed-size exact checker.

| Order | Three ns/op samples | B/op | allocs/op |
| --- | --- | --- | --- |
| 8 | 87408 / 86760 / 89603 | approximately 5432 | 20 |
| 32 | 4028946 / 3960913 / 3970918 | 70992 | 34 |
| 64 | 28387160 / 27983406 / 28205139 | 272752 | 50 |

These samples were collected on a shared development machine and describe the
full checked computation, without native-LAPACK performance claims or a promise
that every matrix converges or has a well-conditioned eigenbasis.

## Immutable complex128 matrices and partial-pivot LU

`ComplexDense` and `ComplexLU` are independent types and entry points. They do
not change Dense, real LU or Eigen signatures, error types, or numerical policies.
There is no automatic real/complex promotion, complex Schur/eigenvalue solver,
Hermitian eigensolver or BLAS/native/subprocess dependency. In particular the
Eigen API above continues to require a **real Dense**, not a general complex
matrix. The following implementation is new Go source with no copied external
library implementation or changes to existing license notices.

```go
a, err := linalg.NewComplexDense(2, 2, []complex128{
    1+2i, 3-4i,
    2i,   7+1i,
})
if err != nil { return err }
adjoint, err := a.ConjTranspose() // explicit conjugation; Transpose alone does not
if err != nil { return err }
f, err := linalg.FactorizeComplex(a)
if err != nil { return err }
x, relativeResidual, err := f.Solve([]complex128{2, 4-1i})
if err != nil { return err }
phase, logAbs, err := f.LogDet() // det(A) / |det(A)|, log(|det(A)|)
if err != nil { return err }
_, _, _, _, _ = adjoint, x, relativeResidual, phase, logAbs
```

| Entry point | Contract |
| --- | --- |
| `NewComplexDense(rows, cols int, data []complex128)` | Return `(*ComplexDense,error)`; copy exactly rows×cols finite real/imaginary components in row-major order. |
| `NewComplexDenseWithLimits(rows, cols, data, limits)` | Same, with explicit ComplexLimits. |
| `ComplexZeros(rows, cols)` / `ComplexIdentity(n)` | Bounded independent complex zero/identity matrices, including empty. |
| `ComplexZerosWithLimits` / `ComplexIdentityWithLimits` | Explicit per-call allocation/work policy. |
| `(*ComplexDense).Dimensions()` / `At(row,col)` / `Data()` | Immutable shape, checked complex scalar, independent row-major copy. |
| `Transpose()` / `ConjTranspose()` | Independent ordinary transpose or conjugate transpose; no shared views. |
| `Add(b)` / `Sub(b)` | Same-shape complex elementwise arithmetic. |
| `Mul(b)` / `MulVec(x)` | Ordinary **bilinear** matrix product/dot sums; no implicit conjugation or Hermitian assumption. |
| Arithmetic/transpose `WithLimits` variants | The same arguments followed by ComplexLimits; independently bound inputs/output/work. |
| `FactorizeComplex(a)` / `FactorizeComplexWithOptions(a,opt)` | Return `(*ComplexLU,error)` for square complex input under default/explicit ComplexLUOptions. |
| `(*ComplexLU).Size()` / `Pivots()` | Input order and a copy of the final row permutation, not a swap log. Row i of P·A is original row Pivots()[i]. |
| `Factors()` / `FactorsWithLimits(limits)` | Return independent `(lower,upper *ComplexDense,err error)` with P·A≈L·U and unit diagonal L. |
| `RelativeReconstructionError()` | Checked original-input `||P·A-L·U||F/||A||F`. |
| `Solve(b []complex128)` | Return `(x []complex128,relativeResidual float64,err error)` for one RHS. |
| `SolveMatrix(b *ComplexDense)` | Return `(x *ComplexDense,relativeResiduals []float64,err error)`; one RHS per column, preserving shape. |
| `SolveWithOptions` / `SolveMatrixWithOptions` | Additional ComplexSolveOptions as last argument. |
| `LogDet()` | Return `(phase complex128,logAbs float64,err error)`, without constructing determinant magnitude. |
| `Inverse()` / `InverseWithOptions(opt)` | Checked identity multi-RHS solve; return `(*ComplexDense,error)`. Prefer solves when applying an inverse. |

All input and exported storage is independent. There is no mutation/resize API.
Nil matrices are invalid for computation; introspection gives (0,0)/nil. A
nonnil zero-value ComplexDense is a valid 0×0 matrix. Nil/zero-value ComplexLU is
invalid, including for LogDet/Factors/solves. Empty factorization must go through
the constructor and is valid: empty L/U, empty pivots, determinant phase 1 and
log magnitude 0. Empty multi-RHS systems preserve 0×r shape and return r zero
residuals; n×0 RHS preserves n×0 shape with no residuals.

Constructors and arithmetic reject nonfinite input components. A finite complex
number whose **modulus** exceeds MaxFloat64 remains valid; norm/pivot comparison
does not require materializing that modulus as float64. Operations and factor
exports return independent allocations, including transposes. Matrices and
completed factors can be shared by concurrent readers/solvers; callers must not
mutate input slices concurrently while a call reads them.

### Complex arithmetic and finite range

Dot sums, matrix products, LU trailing updates, forward-substitution sums and
original-input residuals use a separate, fixed **4352-bit integer accumulator**
for exact products of finite binary64 components. Positive/negative real and
imaginary contributions use four 68-word uint64 arrays. Complex dimension is
at most 4096, so at most 8194 product terms per signed component (plus bounded
carry) fit this fixed storage. Precision/allocation never depends on an input
exponent. This code is independent of the frozen Eigen implementation.

The sum is reduced to a float64 mantissa plus integer exponent, then materialized
when a stored complex128 component is required. This prevents intermediate
product overflow from concealing a finite cancellation result. For example the
first two enormous terms of a dot sum may cancel exactly, leaving a small third
term. Subnormal materialization can involve additional rounding; this is not a
promise of correctly rounded arbitrary-precision matrix arithmetic.

Complex division computes finite-input real/imaginary numerator products and
squared modulus without first forming an overflowing reciprocal or denominator.
Back-substitution keeps the summed numerator in exponent form through division,
so that numerator need not itself fit complex128. Norms and wide quotients carry
separate real/imaginary exponents. Mantissas remain float64; wide operations and
factorization remain subject to rounding/conditioning, not arbitrary-precision
linear algebra. The stored forward-substitution vector and packed L/U entries
must fit complex128. No balancing, iterative refinement or alternate solver is
used automatically.

A required computed nonzero stored/result component that rounds to zero gives
ErrRange, as does component overflow. Nonzero subnormal components are allowed.
This strict policy also applies to LU multipliers, forward workspace, solutions,
inverse and determinant phase; a highly unbalanced scalar can have a phase
component too small to export. There is no clipping, silent zero substitution,
pseudoinverse, rank truncation or native fallback. Ordinary cancellation to exact
zero is allowed. Extremely separated low contributions in float64 mantissa
operations may still round away; residual policies do not certify individual
component accuracy.

### LU pivot, reconstruction and solve policies

This implementation uses **ordinary partial pivoting by complex modulus**:
choose the first maximum-modulus remaining entry in each column. It does not
scale pivot selection by row norms. Multipliers have magnitude at most one to
rounding; raw pivot paths need not equal SciPy/LAPACK, which may use a different
complex pivot norm. Correctness is checked by reconstruction/solutions, not by
requiring identical packed factors or permutations.

For the selected pivot row, retain its original maximum entry modulus, following
row swaps. A pivot is rejected when zero, or when

`|pivot| / max_j |original pivot-row entry_j| <= RelativePivotTolerance`.

The ratio is evaluated with exponent-carrying arithmetic. Zero tolerance rejects
only exactly zero pivots; default is 1e-14. This policy is not a rank/condition
number estimate or an absolute cutoff: a tiny independent diagonal row can be
accepted, while a moderate pivot in a row with huge off-diagonals can be rejected.
`ComplexSingularError` matches existing ErrSingular and reports Pivot, complex
Value, Relative, RelativePivotTolerance, and original row magnitude represented
as ScaleMantissa×2^ScaleExponent. Singular inputs produce no factor; LogDet does
not return NumPy's singular `(phase=0, logAbs=-Inf)` tuple for a failed factor.

After elimination, verify original P·A against stored L/U using exact-product
residual sums. Default ReconstructionTolerance=1e-12; zero is literal exact-zero
product sum. Zero input is singular unless order zero. Failed reconstruction
returns nil with ComplexReconstructionError matching ErrReconstruction.

Each solve checks the **actual exported column** against the stored original A:

`relative = ||A*x-b||2 / (||A||F*||x||2 + ||b||2)`.

The numerator uses exact binary64 product sums before exponent-carrying norm
rounding. An exact zero numerator gives zero, including an exact zero system
with zero RHS. A nonzero residual ratio too tiny for float64 is retained as the
smallest positive subnormal; literal zero tolerance therefore cannot accept it
because the final ratio underflowed. Default RelativeResidualTolerance=1e-12.
The error is normwise backward residual, not forward solution accuracy for a
near-singular matrix. No condition number is supplied. `ComplexResidualError`
identifies the first rejected RHS with RHS, Relative and Limit, matching
ErrResidual. A failure in any RHS returns nil solution, nil residual slice
(vector solve returns nil x and scalar zero). No partial outputs escape.

LogDet multiplies normalized pivot phases with row-swap parity and accumulates
log modulus with compensated summation. It avoids directly forming an overflowing
or underflowing determinant magnitude, but requires finite representable phase
components. Phase is unit modulus to rounding; logAbs is real, with no branch
choice for a multivalued complex logarithm. Numerical phase and small solutions
have no universal relative accuracy guarantee near cancellation/singularity.

| Options | Defaults | Explicit zero semantics |
| --- | --- | --- |
| `ComplexLUOptions.Limits` | DefaultComplexLimits() | Zero fields select limits defaults. |
| RelativePivotTolerance | 1e-14 | Reject only zero pivots. |
| ReconstructionTolerance | 1e-12 | Require exact-zero product residual. |
| `ComplexSolveOptions.Limits` | DefaultComplexLimits() | Zero fields select limits defaults. |
| RelativeResidualTolerance | 1e-12 | Require exact-zero product residual. |

Every numerical tolerance is finite in [0,1). Start from
`DefaultComplexLUOptions()` / `DefaultComplexSolveOptions()` before adjusting
individual policies. These are distinct from real LUOptions and other residual
option types; existing types and errors were not modified.

### Explicit complex resource bounds

| ComplexLimits field | Zero/default | Allowed range/meaning |
| --- | --- | --- |
| MaxDimension | 1024 | 1..4096, applied to both dimensions even when storage is empty. |
| MaxElements | 1M | At most 16M complex entries per matrix/vector buffer. |
| MaxWork | 256M | Per-call conservative bounded arithmetic-kernel units, not FLOPs/time. |

Dimension multiplication and 16-byte element storage are checked against int
arithmetic before indexing or allocation, including 32-bit architectures.
Invalid policies match ErrBudget. Negative/storage-overflow shapes match
ErrDimension; configured size/element/work exhaustion matches ErrComplexLimit.
All policies and work reserves are checked before result/factor allocation.

Policies are **per call**, not inherited from an operand or retained in a
factor's mutable budget. Default methods use defaults; with a matrix/factor
larger than those defaults, call its explicit WithLimits/WithOptions variant.
This avoids silently choosing one operand's policy for a binary operation.
At/Data/Dimensions and factor diagnostics are bounded by the immutable object's
already-checked hard shape. Data/Pivots always copy, without an implicit work
account. LogDet has at most 4096 fixed steps. Its scalar results do not allocate
an unbounded result buffer.

Work reserves (dimensions have already been capped, so uint64 products cannot
overflow):

| Call | Work reserve |
| --- | --- |
| Constructor, zeros, identity, either transpose; c output entries | c bounded copy/write kernels. |
| Add/Sub; c entries | 3c. |
| Mul; a=m×k, b=k×n | mn(k+1)+mk+kn. |
| MulVec; a=m×k | m(k+1)+k. |
| LU factorization; n×n | 4n³+16n²+4n, including full reconstruction. |
| L/U export | 2n². |
| Solve; n×r RHS | 8n²r+16nr+4n²+4r, including original-system residuals. |
| Inverse | Same solve reserve with r=n, plus n² for identity setup. |

The model counts bounded kernels; exact accumulators have a constant maximum
68-word carry scan per term. There are no iterative convergence loops. Raising
MaxDimension alone does not waive element/work limits. For example order 1024
LU exceeds default work; explicitly supply a larger work budget if needed.

Matrix operations use O(input/output size) storage plus fixed accumulator
scratch. LU retains two n² complex buffers (packed and original), permutation
and row scales; peak workspace is O(n²). Multi-RHS solve adds an n×r solution
and r residuals; inverse also adds an n² identity. Factor export adds two n²
buffers. MaxElements bounds each buffer, not total process RAM or cumulative
allocations. The hard dimensional cap bounds their sum, but there is no OOM
recovery; explicitly requesting large allowed limits still requires sufficient
memory.

### Complex validation and independent references

`tests/reference/linalg_complex.py` generated the checked-in
`tests/data/linalg/complex_scipy.json` with NumPy 2.3.5 / SciPy 1.17.1, seed 20841.
There are 30 LU/multi-RHS/LogDet references and four rectangular arithmetic
references. The development generator positively equilibrates each row before
SciPy LU/solve/slogdet to avoid platform safe-min behavior on mixed/tiny scales;
solutions are for the same original equation, and log determinant adds the row
scale logs. This development-only normalization is not a runtime fallback.
Ordinary tests read JSON and never invoke Python, SciPy or a native process.

Cases cover orders 0..32, complex and real scalar pivots, Hermitian and deliberately
non-Hermitian matrices, row swaps, complex conjugation, rectangular/empty products,
near-singular and independent mixed 1e-300/1e300 diagonal scales. Go analytical
tests additionally cover maximum-float complex modulus, minimum subnormal pivots,
finite cancellation after individually overflowing products, singular and rejected
near-singular systems, exact-pivot opt-in, required multiplier/update/solution/
phase/inverse range errors, exact budget boundaries, literal-zero reconstruction
and residual rejection, later-RHS failure without partial results, immutable
exports and concurrent arithmetic/factorization/solves.

A Go development test compares the new fixed integer accumulator to a separate
4352-bit math/big.Float oracle over random finite product exponents, plus carries
from 8194 maximal products followed by cancellation down to the smallest possible
binary64 product. No math/big precision is used by the production complex path.

Validation commands (runner supplies noasm; all test/bench sources are central):

```sh
python3 tests/reference/linalg_complex.py # development only
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -race -count=1 -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -run='^$' -fuzz='^FuzzComplexLU$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 go vet -tags noasm ./linalg
GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -tags noasm ./linalg
```

Full focused linalg tests/race, vet and 32-bit cross-build passed. Whole-package
statement coverage is 96.4%. The 20-second complex-LU fuzz run executed 619,966
inputs, passing, with order bounded 1..8 and uniform power-of-two scales from
exponents -1000..1000. The central TestMain keeps failure corpus under
`tests/data/linalg/testdata/fuzz/`. Dependency audit lists only `gomath/linalg`
outside the standard library. Production additions are only complex_dense.go,
complex_numeric.go and complex_lu.go; accepted Dense/real LU/Schur/Eigen sources
are frozen. No root/CLI/export/examples, Python reference source or other docs
were changed by this slice.

### Complex matrix/LU benchmark samples

Recorded 2026-11-04, Go 1.27.3, darwin/arm64, Apple M2, GOMAXPROCS=8,
CGO_ENABLED=0, runner noasm, serial execution with `-p=1`:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=linalg -run='^$' -bench='^BenchmarkComplex$' -benchmem -benchtime=300ms -count=3 -p=1
```

Inputs are deterministic n×n Gaussian complex matrices with 2n added to the
real diagonal, PCG seeds (20841,n). Solve4 uses four independent complex Gaussian
RHS columns. Multiplication includes exact dot sums and output range handling.
LU includes copies, modulus pivoting, complex division/trailing updates and
complete original-input reconstruction. Solve4 excludes factorization and
includes input copying, substitutions, output range handling and all four
original-system residual checks. All include result allocation; input creation,
factor exports, disk, startup, NumPy/SciPy, inverse and LogDet are excluded.
Image dimensions/frame counts do not apply.

| Operation/order | Three ns/op samples | B/op | allocs/op |
| --- | --- | --- | --- |
| Mul/8 | 33511 / 34088 / 33591 | 1072 | 2 |
| LU/8 | 83066 / 82871 / 82885 | 2352 | 5 |
| Solve4/8 | 48388 / 48042 / 48570 | 592 | 3 |
| Mul/32 | 1443086 / 1459562 / 1453407 | 16432 | 2 |
| LU/32 | 4116930 / 4106430 / 4158680 | 33648 | 5 |
| Solve4/32 | 443097 / 442957 / 441019 | 2128 | 3 |
| Mul/64 | 10475824 / 10512302 / 10498731 | 65584 | 2 |
| LU/64 | 31107221 / 31651800 / 31133517 | 132720 | 5 |
| Solve4/64 | 1529441 / 1533194 / 1534111 | 4176 | 3 |

These costs include deliberate correctness checks on a shared development
machine. There is no claim of BLAS speed, universal forward accuracy, or support
for an independent eigenbasis of a general complex matrix.
