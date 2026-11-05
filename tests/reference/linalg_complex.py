#!/usr/bin/env python3
"""Development-only NumPy/SciPy complex matrix/LU/solve reference generator."""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy import linalg

rng = np.random.default_rng(20841)
cases = []


def packed(a):
    return [[float(z.real), float(z.imag)] for z in np.asarray(a).ravel()]


def add(name, a, x=None):
    a = np.asarray(a, dtype=np.complex128)
    n = len(a)
    if x is None:
        x = rng.normal(size=(n, 3)) + 1j*rng.normal(size=(n, 3))
    x = np.asarray(x, dtype=np.complex128)
    b = a@x
    if n:
        # Normalize the equation before LAPACK so the reference does not depend
        # on the platform's safe-min scaling policy for subnormal pivots.
        scale = np.max(np.maximum(abs(a.real), abs(a.imag)), axis=1)[:, None]
        reference = linalg.solve(a/scale, b/scale)
        p, lower, upper = linalg.lu(a/scale)
        lu_residual = linalg.norm(a/scale-p@lower@upper)/linalg.norm(a/scale)
        phase, log_abs = np.linalg.slogdet(a/scale)
        log_abs += np.log(scale).sum()
    else:
        reference = x
        lu_residual, phase, log_abs = 0., 1.+0j, 0.
    cases.append(dict(name=name, n=n, rhs=x.shape[1], data=packed(a), b=packed(b),
                      solution=packed(reference), phase=packed([phase])[0], logAbs=float(log_abs),
                      scipyLUResidual=float(lu_residual), tolerance=2e-11))


add("empty", np.empty((0, 0)), np.empty((0, 2)))
add("imag_scalar", [[2j]], [[1-2j, -3+1j]])
add("row_swap", [[0, 2j], [3-2j, 1j]])
add("nonhermitian", [[1+2j, 3-4j, 2j], [-2+1j, 4+3j, 5], [2+3j, -1j, 7-1j]])
for n in [2, 3, 8, 16, 32]:
    for j in range(3):
        a = rng.normal(size=(n, n)) + 1j*rng.normal(size=(n, n))
        a += 2*n*np.eye(n)
        add(f"random_{n}_{j}", a)
        if j == 0:
            add(f"hermitian_{n}", a+a.conj().T)
base = rng.normal(size=(5, 5))+1j*rng.normal(size=(5, 5))+10*np.eye(5)
for scale in [1e-300, 1e-200, 1e200, 1e300]:
    add(f"scale_{scale}", base*scale)
add("mixed_diagonal_scales", np.diag([1e-300*(1+1j), 1e300*(1-1j), -2j]))
add("near_singular", [[1, 1j], [1, 1j+2.**-36]], [[1+1j, 2-1j], [-1+2j, 1j]])
# Rectangular arithmetic references use NumPy without LU/Hermitian assumptions.
arithmetic = []
for m, k, n in [(0, 3, 2), (3, 0, 4), (2, 3, 4), (5, 7, 3)]:
    a = rng.normal(size=(m, k))+1j*rng.normal(size=(m, k))
    b = rng.normal(size=(k, n))+1j*rng.normal(size=(k, n))
    arithmetic.append(dict(m=m, k=k, n=n, a=packed(a), b=packed(b),
                           product=packed(a@b), transpose=packed(a.T), adjoint=packed(a.conj().T)))
target = Path(__file__).resolve().parents[1]/"data"/"linalg"/"complex_scipy.json"
target.write_text(json.dumps(dict(numpy=np.__version__, scipy=scipy.__version__, seed=20841,
                                  cases=cases, arithmetic=arithmetic), indent=2)+"\n")
print(f"{len(cases)} LU/solve + {len(arithmetic)} arithmetic cases -> {target}")
