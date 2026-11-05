"""Pinned PolarPlane explicit mappings, not its labels/default grid metrics.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/polarplane.py tests/data/graph/manim-polarplane-0.22.1.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import PolarPlane
import numpy as np

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
cases = []
for maximum, step, world in [(3, 1, 2.5), (4, .5, 3)]:
    for alpha in [0, .125, .25, .5, .75, .875, 1]:
        angle = .8 * alpha
        matrix = np.array([[np.cos(angle), -np.sin(angle), 0, 1.2*alpha],
                           [np.sin(angle), np.cos(angle), 0, -.7*alpha],
                           [0, 0, 1, .3*alpha], [0, 0, 0, 1]])
        plane = PolarPlane(radius_max=maximum, radius_step=step, size=2*world,
                           azimuth_step=12, faded_line_ratio=0)
        plane.apply_matrix(matrix[:3, :3]).shift(matrix[:3, 3])
        samples = []
        for radius in [0, .5, maximum, 2*maximum]:
            for azimuth in [0, .7, np.pi/2, np.pi, 4.9, -.8]:
                point = plane.polar_to_point(radius, azimuth)
                inverse = plane.point_to_polar(point)
                samples.append({"radius": radius, "azimuth": azimuth,
                                "point": point.tolist(), "inverse": list(inverse)})
        cases.append({"maximum": maximum, "step": step, "world": world,
                      "matrix": matrix.flatten().tolist(), "samples": samples})
out.parent.mkdir(parents=True, exist_ok=True)
with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": COMMIT,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "14 rigid poses, 336 explicit polar mappings and inverse checks; offsets/CW/defaults/fonts/grid control points and animation timing unverified.",
               "cases": cases}, stream, indent=2)
    stream.write("\n")
