"""Pinned Manim complex-coordinate mappings, with rigid 3D intermediate poses.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/complexplane.py tests/data/graph/manim-complexplane-0.22.1.json

No font/TeX execution is used. Grid defaults and arbitrary shears are outside
this reference; GoMath's dual-basis inverse for shears is tested independently.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import ComplexPlane
import numpy as np

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
cases = []
for xr, yr, width, height in [([-3, 5, 1], [-2, 4, 1], 8, 3), ([-4, 4, 2], [-3, 3, 1], 6, 5)]:
    for alpha in [0, .125, .25, .5, .75, .875, 1]:
        a, b = .8 * alpha, -.6 * alpha
        rz = np.array([[np.cos(a), -np.sin(a), 0], [np.sin(a), np.cos(a), 0], [0, 0, 1]])
        rx = np.array([[1, 0, 0], [0, np.cos(b), -np.sin(b)], [0, np.sin(b), np.cos(b)]])
        linear = rz @ rx
        shift = np.array([1.2 * alpha, -.7 * alpha, .9 * alpha])
        matrix = np.eye(4)
        matrix[:3, :3], matrix[:3, 3] = linear, shift
        plane = ComplexPlane(x_range=xr, y_range=yr, x_length=width, y_length=height,
                             faded_line_ratio=0, tips=False)
        plane.shift(-plane.n2p(complex((xr[0]+xr[1])/2, (yr[0]+yr[1])/2)))
        plane.apply_matrix(linear).shift(shift)
        samples = []
        for z in [0j, 1+0j, 1j, -2+3j, 10-7j]:
            point = plane.n2p(z)
            off_plane = point + linear[:, 2] * 1.7
            back = plane.p2n(off_plane)
            samples.append({"number": [z.real, z.imag], "point": point.tolist(),
                            "off_plane": off_plane.tolist(), "inverse": [back.real, back.imag]})
        cases.append({"x": xr, "y": yr, "width": width, "height": height,
                      "alpha": alpha, "matrix": matrix.flatten().tolist(), "samples": samples})
out.parent.mkdir(parents=True, exist_ok=True)
with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": COMMIT,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "14 rigid poses; 70 n2p points and 70 off-plane p2n projections. No defaults/fonts/shear or complete animation timing parity.",
               "cases": cases}, stream, indent=2)
    stream.write("\n")
