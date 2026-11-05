#!/usr/bin/env python3
"""Development-only camera-to-world SciPy Rotation/Slerp reference.

Camera local axes: right +X, up +Y, forward -Z. View coordinates equal
diag(1,1,-1) @ R.T @ (world-eye); this is not a look-at implementation.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy.spatial.transform import Rotation, Slerp

root = Path(__file__).resolve().parents[1] / "data"
rng = np.random.default_rng(20261105)
rotations = [Rotation.identity()]
for axis in np.eye(3):
    for angle in [np.pi/2, -np.pi/2, np.pi]:
        rotations.append(Rotation.from_rotvec(axis*angle))
rotations += list(Rotation.random(8, random_state=rng))
cases = []
eye = np.array([.37, -.19, .83])
for i, r in enumerate(rotations):
    q = r.as_quat()[[3, 0, 1, 2]]
    if i == 10:
        q *= 1e308
    if i == 11:
        q *= 1e-300
    points = []
    for depth in [-1, .05, .1, .7, 2.3, 20, 21]:
        # Construct independent world points; recompute depth from those actual
        # rounded world values so clipping comparisons can avoid exact ties.
        world = eye + r.apply([.31, -.27, -depth])
        view = r.inv().apply(world-eye) * [1, 1, -1]
        points.append(dict(world=world.tolist(), view=view.tolist()))
    cases.append(dict(q=q.tolist(), eye=eye.tolist(), matrix=r.as_matrix().ravel().tolist(), points=points))
target = root / "three" / "camera_orientation_scipy.json"
target.parent.mkdir(parents=True, exist_ok=True)
meta = dict(scipy=scipy.__version__, numpy=np.__version__, seed=20261105)
target.write_text(json.dumps(dict(**meta, cases=cases), indent=2)+"\n")

times = [.25, 1, 1.75]
rs = Rotation.from_euler("xyz", [[.05,-.2,-.3],[.3,.12,.8],[-.18,.2,1.45]])
eyes = np.array([[0,0,5],[.3,-.1,4.9],[.6,.2,4.7]])
fovs, spans = [.9,1.1,.8], [4.7,5.2,4.4]
sample_times = sorted(set([0,2]+np.linspace(.25,1.75,33).tolist()))
clipped = np.clip(sample_times,times[0],times[-1])
samples = []
for t, tc, r in zip(sample_times,clipped,Slerp(times,rs)(clipped)):
    eye = [float(np.interp(tc,times,eyes[:,i])) for i in range(3)]
    samples.append(dict(time=t,eye=eye,matrix=r.as_matrix().ravel().tolist(),
                        fov=float(np.interp(tc,times,fovs)),span=float(np.interp(tc,times,spans))))
target_scene = root / "scene3d" / "camera_orientation_scipy.json"
target_scene.write_text(json.dumps(dict(**meta,
    keys=[dict(time=t,eye=eye.tolist(),q=r.as_quat()[[3,0,1,2]].tolist(),fov=fov,span=span)
          for t,eye,r,fov,span in zip(times,eyes,rs,fovs,spans)],samples=samples),indent=2)+"\n")
print(f"SciPy {scipy.__version__}, NumPy {np.__version__}: {len(cases)} camera poses / {sum(len(c['points']) for c in cases)} world-view samples; {len(samples)} dense timeline frames")
