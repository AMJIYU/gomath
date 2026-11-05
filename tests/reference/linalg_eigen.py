#!/usr/bin/env python3
"""Development-only SciPy right-eigenvector reference. No runtime dependency.

Directions are stored only for distinct, well-conditioned roots. Repeated
semisimple roots store their full invariant subspace for projector comparison;
a unique repeated-root eigenbasis is deliberately not prescribed.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy import linalg

rng = np.random.default_rng(39521)
cases = []


def add(name, a, mode="directions", tolerance=2e-10):
    a = np.asarray(a, dtype=np.float64)
    n = len(a)
    scale = float(np.max(np.abs(a))) if n else 1.0
    scale = scale or 1.0
    values, vectors = linalg.eig(a / scale)
    groups = []
    if mode == "subspaces":
        remaining = set(range(n))
        while remaining:
            first = min(remaining)
            group = [j for j in remaining if abs(values[j]-values[first]) < 1e-10]
            remaining.difference_update(group)
            basis, _ = linalg.qr(vectors[:, group], mode="economic")
            projector = basis @ basis.conj().T
            groups.append(dict(value=[float(values[first].real), float(values[first].imag)],
                               dimension=len(group),
                               projector=[[[float(x.real), float(x.imag)] for x in row]
                                          for row in projector]))
    cases.append(dict(name=name, n=n, data=a.ravel().tolist(), scale=scale,
                      values=[[float(x.real), float(x.imag)] for x in values],
                      vectors=[[[float(x.real), float(x.imag)] for x in vectors[:, j]] for j in range(n)],
                      mode=mode, tolerance=tolerance, groups=groups,
                      referenceMaxResidual=max((float(linalg.norm((a/scale)@vectors[:,j]-values[j]*vectors[:,j]))
                                                for j in range(n)), default=0.0)))


add("unequal_complex_block", [[4, 10], [-1, 2]])
add("nonnormal_mixed", [[0, 2, 7], [-3, 0, -1], [0, 0, 5]])
add("near_real_conjugate", [[1, 1], [-2.**-52, 1]], tolerance=1e-8)
add("near_coalescent", [[1, 1], [-np.nextafter(1., 2.), -1]], tolerance=1e-8)
for n in [2, 3, 5, 8, 16, 32]:
    for j in range(3):
        add(f"gaussian_{n}_{j}", rng.normal(size=(n, n)))
base = rng.normal(size=(7, 7))
for scale in [1e-300, 1e-250, 1., 1e250, 1e300]:
    add(f"scaled_{scale}", base*scale)
for n in [4, 8, 12]:
    # Similarities are well-conditioned but deliberately nonorthogonal. Repeated
    # eigenvectors themselves are not unique; compare their invariant subspaces.
    q, _ = linalg.qr(rng.normal(size=(n, n)))
    s = q @ np.diag(np.linspace(1., 3., n)) @ q.T
    diagonal = np.diag(np.repeat([1., 3.], n//2))
    add(f"repeated_semisimple_{n}", s@diagonal@linalg.inv(s), "subspaces", 1e-9)

target = Path(__file__).resolve().parents[1]/"data"/"linalg"/"eigen_scipy.json"
target.write_text(json.dumps(dict(numpy=np.__version__, scipy=scipy.__version__, seed=39521,
                                  cases=cases), indent=2)+"\n")
print(f"{len(cases)} cases -> {target}")
