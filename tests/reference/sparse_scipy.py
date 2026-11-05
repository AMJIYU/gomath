"""Development-only SciPy sparse reference; no product/test runtime dependency.
Regenerate from repository root: python3 tests/reference/sparse_scipy.py
Independent API calls only; this script contains no copied SciPy implementation.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy import sparse
from scipy.sparse import linalg

rng = np.random.default_rng(20261104)
cases = []
for name, n, scale in [('random12', 12, 1.), ('random48', 48, 1.),
                       ('small_scale', 9, 1e-100), ('large_scale', 9, 1e100)]:
    m = sparse.random(n, n, density=.14, random_state=rng, data_rvs=rng.standard_normal)
    a = (m.T @ m + sparse.diags(np.linspace(1, 3, n))).tocsr() * scale
    a.sum_duplicates(); a.sort_indices(); a.eliminate_zeros()
    x0 = rng.standard_normal(n)
    b = a @ x0
    x, info = linalg.cg(a, b, rtol=1e-12, atol=0, maxiter=1000)
    assert info == 0
    cases.append(dict(name=name, rows=n, cols=n, rowptr=a.indptr.tolist(),
                      colind=a.indices.tolist(), values=a.data.tolist(), b=b.tolist(),
                      solution=x.tolist(), matvec=(a @ np.arange(n)).tolist(),
                      original_residual=float(np.linalg.norm(b-a @ x)/np.linalg.norm(b))))
# Parent sparse-solve integration family: ordinary Dirichlet Poisson systems.
for n in (31, 32, 63):
    a = sparse.diags([-np.ones(n-1), 2*np.ones(n), -np.ones(n-1)],
                     [-1, 0, 1], format='csr')
    a.sort_indices()
    b = np.full(n, (1 / (n+1))**2)
    preconditioner = sparse.diags(1/a.diagonal())
    x, info = linalg.cg(a, b, M=preconditioner, rtol=1e-12, atol=0, maxiter=1000)
    assert info == 0
    cases.append(dict(name=f'poisson{n}', rows=n, cols=n, rowptr=a.indptr.tolist(),
                      colind=a.indices.tolist(), values=a.data.tolist(), b=b.tolist(),
                      solution=x.tolist(), matvec=(a @ np.arange(n)).tolist(),
                      original_residual=float(np.linalg.norm(b-a @ x)/np.linalg.norm(b)),
                      reference_preconditioner='Jacobi'))
root = Path(__file__).resolve().parents[1] / 'data' / 'sparse'
root.mkdir(parents=True, exist_ok=True)
(root/'scipy.json').write_text(json.dumps(dict(numpy=np.__version__, scipy=scipy.__version__, cases=cases), indent=2)+'\n')
