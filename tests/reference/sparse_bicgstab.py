"""Development-only SciPy BiCGSTAB/direct references. No copied implementation.
Run from repository root: python3 tests/reference/sparse_bicgstab.py
Extreme cases solve an explicitly scaled equivalent reference system to avoid
SciPy binary64 norm underflow/overflow; Go tests use the original stored system.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy import sparse
from scipy.sparse import linalg

rng = np.random.default_rng(20261104)
cases = []

def record(name, a, scale=1., jacobi=False):
    a = a.tocsr()
    a.sum_duplicates(); a.sort_indices(); a.eliminate_zeros()
    known = rng.uniform(-1, 1, a.shape[0])
    b = a @ known
    m = sparse.diags(1/a.diagonal()) if jacobi else None
    x, info = linalg.bicgstab(a, b, M=m, rtol=2e-13, atol=0, maxiter=2000)
    assert info == 0, (name, info)
    direct = linalg.spsolve(a, b)
    assert np.linalg.norm(b-a@x)/np.linalg.norm(b) < 1e-12
    cases.append(dict(name=name, rows=a.shape[0], rowptr=a.indptr.tolist(),
                      colind=a.indices.tolist(), values=(a.data*scale).tolist(),
                      b=(b*scale).tolist(), solution=x.tolist(), direct=direct.tolist(),
                      reference_scale=scale, jacobi=jacobi,
                      original_relative_residual=float(np.linalg.norm(b-a@x)/np.linalg.norm(b))))

for n in (7, 29, 73):
    a = sparse.random(n, n, density=.12, random_state=rng, data_rvs=rng.standard_normal)
    a = a + sparse.diags(np.asarray(abs(a).sum(axis=1)).ravel()+2)
    record(f'nonsymmetric{n}', a)
    record(f'jacobi{n}', a, jacobi=True)
a = sparse.diags([-np.ones(30), 4*np.ones(31), -2*np.ones(30)], [-1,0,1])
record('convection31', a)
record('large_scale', a, 1e300)
record('small_scale', a, 1e-300, True)
record('negative_diagonal', -a, jacobi=True)
a = sparse.diags([np.linspace(2,4,15), 3*np.ones(14)], [0,1])
record('nonnormal_upper', a, jacobi=True)
root=Path(__file__).resolve().parents[1]/'data'/'sparse'
(root/'bicgstab.json').write_text(json.dumps(dict(numpy=np.__version__,scipy=scipy.__version__,cases=cases),indent=2)+'\n')
