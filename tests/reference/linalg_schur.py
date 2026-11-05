#!/usr/bin/env python3
"""Development-only SciPy Schur/eigenvalue reference, never a Go runtime input."""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy import linalg

rng = np.random.default_rng(9639)
cases = []


def add(name, a, tolerance=3e-11):
    a = np.asarray(a, dtype=np.float64)
    n = a.shape[0]
    scale = float(np.max(np.abs(a))) if n else 1.0
    if not scale:
        scale = 1.0
    normalized = a / scale
    t, q = linalg.schur(normalized, output="real")
    values = linalg.eigvals(t)
    norm = linalg.norm(normalized)
    cases.append(dict(name=name, n=n, data=a.ravel().tolist(),
                      values=[[float(v.real), float(v.imag)] for v in values],
                      scale=scale, eigenTolerance=tolerance,
                      scipyReconstruction=float(linalg.norm(normalized-q@t@q.T)/(norm or 1.0)),
                      scipyOrthogonality=float(linalg.norm(q.T@q-np.eye(n)))))


add("empty", np.empty((0, 0)))
add("zero", np.zeros((5, 5)))
add("negative_scalar", [[-3]])
add("complex_pair", [[0, -2], [2, 0]])
add("real_pair", [[1, 4], [2, 3]])
add("defective_pair", [[1, 1], [0, 1]])
add("lower_defective_pair", [[1, 0], [1, 1]])
add("repeated_real_pair", [[2, 1], [-1, 0]], 1e-7)
add("unequal_complex_pair", [[4, 10], [-1, 2]])
add("unbalanced_complex_pair", [[0, 1e100], [-1e-100, 0]])
add("symmetric_pair", [[0, 1], [1, 0]])
add("triangular_repeated", [[1, 4, -3, 2], [0, 1, 7, -5], [0, 0, -2, 4], [0, 0, 0, -2]])
add("cyclic_permutation", np.roll(np.eye(7), 1, axis=0))
add("nilpotent_lower", np.diag(np.ones(7), -1))
add("grcar_16", sum(np.diag(np.ones(16-k),k) for k in range(4))-np.diag(np.ones(15),-1))
add("unity_companion_16", linalg.companion(np.r_[1., np.zeros(15), -1.]))
add("real_companion_8", linalg.companion(np.poly(np.arange(1.,9.))), 1e-7)
add("near_real_complex", [[1.,1.],[-2.**-52,1.]])
# The independent Schur rotation adds ~3e-17 to this zero real center.
# Its tiny roots therefore need a relative root tolerance of 1e-8. The exact
# analytical roots (trace zero, discriminant -2^-52) have a separate Go test.
add("near_repeated_complex", [[1.,1.],[-np.nextafter(1.,2.),-1.]], 1e-8)
base = rng.normal(size=(8, 8))
for scale in [1, 1e-300, 1e-250, 1e250, 1e300]:
    add(f"scaled_random_{scale}", base*scale)
for n in [3, 4, 7, 12, 24, 32]:
    for seed in range(4):
        add(f"random_{n}_{seed}", rng.normal(size=(n, n)))
add("random_64",rng.normal(size=(64,64)))
for n in [4, 8, 16]:
    q, _ = linalg.qr(rng.normal(size=(n, n)))
    blocks = np.diag(np.arange(n, dtype=float))
    for k in range(0, n, 4):
        blocks[k:k+2, k:k+2] = [[k, 3], [-2, k]]
    add(f"known_real_and_complex_{n}", q@blocks@q.T)
    repeated = np.eye(n)
    add(f"orthogonal_repeated_{n}", q@repeated@q.T)
q, _ = linalg.qr(rng.normal(size=(4, 4)))
add("orthogonal_defective_jordan4", q@(np.eye(4)+np.diag(np.ones(3), 1))@q.T, 5e-4)

target = Path(__file__).resolve().parents[1]/"data"/"linalg"/"schur_scipy.json"
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(dict(numpy=np.__version__, scipy=scipy.__version__, seed=9639, cases=cases), indent=2)+"\n")
print(f"{len(cases)} cases -> {target}")
