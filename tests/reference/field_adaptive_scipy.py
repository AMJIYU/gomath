"""Independent development-only SciPy sample-time reference. No copied code.
Run from repo root: python3 tests/reference/field_adaptive_scipy.py
The sample times come from SciPy RK45 (not the Go integrator); DOP853 then
computes a tighter independent solution at those times. Ordinary tests use JSON.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy.integrate import solve_ivp

cases=[]
systems=[
 ('harmonic', [[0,-1],[1,0]], [0,0], [1,0], 6.2),
 ('spiral', [[-.2,-2],[2,-.2]], [0,0], [1,.3], 3.5),
 ('linear_growth', [[.4,.2],[0,-.3]], [0,0], [.5,-.2], 2.3),
 ('affine', [[-.3,.4],[-.6,-.2]], [.15,-.1], [.7,.5], 4.),
 ('stiff_decay', [[-80,0],[0,-1]], [0,0], [1,1], .7),
 ('harmonic_backward', [[0,-1],[1,0]], [0,0], [1,0], -4.7),
]
for name, matrix, offset, seed, end in systems:
    a=np.array(matrix,dtype=float);c=np.array(offset,dtype=float)
    fn=lambda t,y:a@y+c
    pilot=solve_ivp(fn,(0,end),seed,method='RK45',rtol=2e-5,atol=1e-8,first_step=min(.05,abs(end)),max_step=.45)
    assert pilot.success
    indexes=np.unique(np.linspace(0,len(pilot.t)-1,8,dtype=int))
    times=pilot.t[indexes]
    ref=solve_ivp(fn,(0,end),seed,method='DOP853',rtol=3e-14,atol=1e-14,t_eval=times)
    assert ref.success
    cases.append(dict(name=name,matrix=matrix,offset=offset,seed=seed,end=end,
                      times=times.tolist(),points=ref.y.T.tolist()))
root=Path(__file__).resolve().parents[1]/'data'/'field'
(root/'adaptive_scipy.json').write_text(json.dumps(dict(numpy=np.__version__,scipy=scipy.__version__,cases=cases),indent=2)+'\n')
