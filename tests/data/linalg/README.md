# Independent real Schur references

`schur_scipy.json` contains 56 development-generated NumPy/SciPy cases.
Reproduce with `python3 tests/reference/linalg_schur.py` from the repository root;
the generator requires NumPy/SciPy only during development. Ordinary Go tests
read the checked-in JSON and never invoke Python, SciPy, LAPACK or a subprocess.

Recorded versions: NumPy 2.3.5, SciPy 1.17.1; NumPy RNG seed 9639.
The generator computes real Schur factors and complex eigenvalues independently,
and records its own reconstruction/orthogonality residuals. The Go tests verify
the complete Go Q/T factors against original A and match the eigenvalue multiset
to the independent reference, without assuming block order or eigenvector signs.

Cases cover orders 0..64, zero/triangular matrices, repeated/defective roots,
nilpotent Jordan blocks, real and complex pairs, Grcar/companion/cyclic matrices,
orthogonal similarities, and homogeneous scales 1e-300..1e300. Root tolerances
are explicitly stored per case. They are normally 3e-11 relative to the largest
reference root; nearly defective cases use larger tolerances because a small
backward error need not imply accurate individual eigenvalues. The near-coalescent
2x2 SciPy reference also has a ~3e-17 spurious real center; a separate analytic
Go test requires the exact zero center and discriminant-derived imaginary part.

Separate Go analytical tests cover minimum-subnormal complex pairs, highly
unbalanced off-diagonals, tiny imaginary parts, exact permutations for strict
nilpotents, output rounding, budgets and error paths. All fuzz failure corpus
files are centralized under this directory's `testdata/fuzz/` by the overlay
test entry point, never under the production `linalg/` package.

## Complex right eigenvector references

`eigen_scipy.json` adds 30 independently generated NumPy/SciPy cases, preserving
all Schur fixtures above. Reproduce with `python3 tests/reference/linalg_eigen.py`.
Recorded versions: NumPy 2.3.5 / SciPy 1.17.1; RNG seed 39521. Values are stored
relative to each matrix's scale; complex vectors are stored by column as
`[real,imaginary]` pairs. Simple roots compare directions up to phase. Repeated
semisimple cases compare invariant-subspace projectors and dimension, with full
numerical rank, rather than requiring one nonunique eigenbasis. Explicit per-case
tolerances account for the two nearly coalescent/conjugate 2x2 references.

Ordinary Go tests use checked-in data only. They also recover/validate right
vectors from every existing Schur fixture and use self-contained analytical
checks for defective and extreme-scale cases, resources, ownership and
concurrency. `math/big.Float` is used only in a Go test as an independent oracle
for the production fixed-size integer-bucket residual accumulator. It is not
used by the runtime solver. No external Python, SciPy, native program or
subprocess is called by ordinary tests or by the production implementation.

## Independent complex matrix / LU references

`complex_scipy.json` adds 30 complex LU/multi-RHS/LogDet cases and four rectangular
NumPy arithmetic cases. Reproduce with `python3 tests/reference/linalg_complex.py`.
NumPy 2.3.5 / SciPy 1.17.1, RNG seed 20841. Every complex entry is a `[real,imag]`
pair in row-major order. The development generator positively scales each row
of A and B before SciPy solve/LU/slogdet, adds row log scales to LogDet, and stores
the solution to the original system. This lets mixed 1e-300/1e300 diagonal scales
be compared without losing a row to global reference scaling. Ordinary tests
read only checked-in JSON and never execute SciPy or a subprocess.

The Go tests independently reconstruct P*A from exported L/U and recompute scaled
A*X-B, compare solutions and determinant phase/log magnitude to reference, and
compare ordinary/conjugate transposes and rectangular products. Pivot paths need
not match LAPACK because the Go complex pivot policy explicitly uses complex
modulus. Singular, near-singular, extreme/subnormal/overflow-cancellation, budget,
range, immutable/concurrent and no-partial-result checks are analytical Go tests.
All previous Schur/Eigen data is preserved. Any fuzz failure corpus remains under
this directory's `testdata/fuzz/`, never production directories.
