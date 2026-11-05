#!/usr/bin/env python3
"""Independent development-only SciPy Rotation/Slerp fixtures; not a runtime.

Run from any directory. Raw inputs are WXYZ, converted explicitly to SciPy's
XYZW convention. NumPy rescales extreme finite inputs before SciPy construction.
"""
import json
from pathlib import Path
import numpy as np
import scipy
from scipy.spatial.transform import Rotation, Slerp

ROOT = Path(__file__).resolve().parents[1] / "data"
rng = np.random.default_rng(20261104)


def rotation(q):
    a = np.asarray(q, dtype=np.float64)
    a = a / np.max(np.abs(a))
    a = a / np.linalg.norm(a)
    return Rotation.from_quat(a[[1, 2, 3, 0]])


pairs = [
    ("identity", [1, 0, 0, 0], [1, 0, 0, 0]),
    ("antipodal", [1, 2, -3, 4], [-1, -2, 3, -4]),
    ("near_parallel", [1, 0, 0, 0], [1, 1e-8, -2e-8, 3e-8]),
    ("half_turn_positive", [1, 0, 0, 0], [0, 0, 0, 1]),
    ("half_turn_negative", [1, 0, 0, 0], [0, 0, 0, -1]),
    ("near_half_turn", [1, 0, 0, 0], [1e-9, 2, 1, -3]),
    ("extreme_finite", [1e308, -1e308, 1e308, -1e308], [1e-308, 2e-308, 3e-308, 4e-308]),
    ("subnormal", [5e-324, 1e-323, -1.5e-323, 2e-323], [-2e-323, 1.5e-323, 1e-323, 1e-323]),
    ("relative_half_turn", [1, 1, 0, 0], [0, 0, -1, -1]),
]
for i in range(11):
    pairs.append((f"random_{i}", rng.normal(size=4).tolist(), rng.normal(size=4).tolist()))

cases = []
times = np.linspace(0, 1, 65)
vector = np.array([.37, -1.21, 2.43])
for name, a, b in pairs:
    ra, rb = rotation(a), rotation(b)
    interpolated = Slerp([0, 1], Rotation.concatenate([ra, rb]))(times)
    cases.append(dict(name=name, a=a, b=b, vector=vector.tolist(),
                      normalizedA=ra.as_quat()[[3, 0, 1, 2]].tolist(),
                      product=(ra*rb).as_matrix().ravel().tolist(),
                      samples=[dict(t=float(t), matrix=r.as_matrix().ravel().tolist(),
                                    rotated=r.apply(vector).tolist())
                               for t, r in zip(times, interpolated)]))
target = ROOT / "geom" / "quaternion_scipy.json"
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(dict(scipy=scipy.__version__, numpy=np.__version__,
                                 seed=20261104, cases=cases), indent=2) + "\n")

# A separate dense timeline fixture exports world rotation matrices without
# using any Go quaternion, timeline, projection, or raster code.
a, b, c = [.73, .21, -.39, .17], [-.14, .51, .23, -.69], [.31, -.62, .41, .27]
key_times = [.25, 1.0, 1.75]
rotations = Rotation.concatenate([rotation(q) for q in [a, b, c]])
sample_times = sorted(set([0, 2] + np.linspace(.25, 1.75, 33).tolist()))
sampled = Slerp(key_times, rotations)(np.clip(sample_times, .25, 1.75))
target_scene = ROOT / "scene3d" / "orientation_scipy.json"
target_scene.parent.mkdir(parents=True, exist_ok=True)
target_scene.write_text(json.dumps(dict(scipy=scipy.__version__, numpy=np.__version__,
    keys=[dict(time=t, q=q) for t, q in zip(key_times, [a, b, c])],
    samples=[dict(time=t, matrix=r.as_matrix().ravel().tolist()) for t, r in zip(sample_times, sampled)]), indent=2) + "\n")
print(f"SciPy {scipy.__version__}, NumPy {np.__version__}: {len(cases)} x {len(times)} quaternion samples -> {target}; {len(sample_times)} timeline samples -> {target_scene}")
