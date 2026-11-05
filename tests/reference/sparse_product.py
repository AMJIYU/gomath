"""Development-only SpGEMM references: SciPy and exact Python Fraction.
Run from repository root: python3 tests/reference/sparse_product.py
No copied third-party implementation; Go tests consume only the generated JSON.
"""
import json
import math
import struct
from fractions import Fraction
from pathlib import Path
import numpy as np
import scipy
from scipy import sparse


def matrix(a):
    a = a.tocsr(copy=True)
    a.sort_indices()
    return dict(rows=a.shape[0], cols=a.shape[1], rowptr=a.indptr.tolist(),
                colind=a.indices.tolist(), values=a.data.tolist())


def from_rows(rows, columns):
    p, ci, v = [0], [], []
    for row in rows:
        for col, value in sorted(row):
            ci.append(col); v.append(float(value))
        p.append(len(v))
    return sparse.csr_matrix((v, ci, p), shape=(len(rows), columns))


cases = []
rng = np.random.default_rng(20261104)
for name, m, k, n in [('rectangular', 7, 11, 5), ('tall_wide', 17, 3, 29),
                      ('float_random', 12, 23, 9), ('empty_rows', 0, 5, 9),
                      ('empty_inner', 4, 0, 7)]:
    a = sparse.random(m, k, density=.29, random_state=rng,
                      data_rvs=rng.standard_normal).tocsr()
    b = sparse.random(k, n, density=.31, random_state=rng,
                      data_rvs=rng.standard_normal).tocsr()
    if name != 'float_random':
        a.data = np.rint(a.data * 4); b.data = np.rint(b.data * 4)
    a.sort_indices(); b.sort_indices()
    expected = (a @ b).tocsr(); expected.sort_indices(); expected.eliminate_zeros()
    cases.append(dict(name=name, oracle='scipy', a=matrix(a), b=matrix(b), expected=matrix(expected)))


def fraction_case(name, a, b):
    ad, bd = matrix(a), matrix(b)
    rowptr, colind, values, bits, rationals = [0], [], [], [], []
    for row in range(ad['rows']):
        cells = {}
        for q in range(ad['rowptr'][row], ad['rowptr'][row+1]):
            inner = ad['colind'][q]
            for s in range(bd['rowptr'][inner], bd['rowptr'][inner+1]):
                col = bd['colind'][s]
                cells[col] = cells.get(col, Fraction()) + Fraction(ad['values'][q])*Fraction(bd['values'][s])
        for col, exact in sorted(cells.items()):
            if not exact:
                continue
            try:
                value = float(exact)
            except OverflowError:
                value = math.inf
            if not math.isfinite(value) or value == 0:
                cases.append(dict(name=name, oracle='fraction', a=ad, b=bd, error='range'))
                return
            colind.append(col); values.append(value)
            bits.append(struct.pack('>d', value).hex())
            rationals.append(str(exact))
        rowptr.append(len(values))
    cases.append(dict(name=name, oracle='fraction', a=ad, b=bd,
                      expected=dict(rows=ad['rows'], cols=bd['cols'], rowptr=rowptr,
                                    colind=colind, values=values), bits=bits, rationals=rationals))

fraction_case('overflowing_products_cancel',
              from_rows([[(0, 1e308), (1, -1e308), (2, 1e-300)]], 3),
              from_rows([[(0, 1e308), (2, -1e308)], [(0, 1e308), (2, -1e308)],
                         [(0, 1), (2, 2)]], 4))
fraction_case('underflowing_products_accumulate',
              from_rows([[(0, 2.0**-538), (1, 2.0**-538), (2, 2.0**-538)]], 3),
              from_rows([[(0, 2.0**-538)], [(0, 2.0**-538)], [(0, 2.0**-538)]], 1))
fraction_case('exact_cancel_zero', from_rows([[(0, 1e308), (1, -1e308)]], 2),
              from_rows([[(0, 1e308)], [(0, 1e308)]], 1))
fraction_case('mixed_scale_and_rounding',
              from_rows([[(0, 1), (1, 2.0**-53), (2, 2.0**-54)], [(0, 1e-300), (2, 1e300)]], 3),
              from_rows([[(0, 1), (1, 1e300)], [(0, 1)], [(0, 1), (1, 1e-300)]], 2))
fraction_case('explicit_zeros', from_rows([[(0, 0), (1, 2)], [(0, 0)]], 2),
              from_rows([[(0, 1e308), (1, 1e308)], [(0, 0), (1, 3)]], 2))
fraction_case('overflow_reject', from_rows([[(0, 1)], [(1, 1e308)]], 2),
              from_rows([[(0, 1)], [(1, 2)]], 2))
fraction_case('nonzero_underflow_reject', from_rows([[(0, 1e-300)]], 1),
              from_rows([[(0, 1e-300)]], 1))

out = Path(__file__).resolve().parents[1] / 'data' / 'sparse' / 'product.json'
out.write_text(json.dumps(dict(numpy=np.__version__, scipy=scipy.__version__, cases=cases), indent=2)+'\n')
