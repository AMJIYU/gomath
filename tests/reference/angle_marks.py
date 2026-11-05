"""Pinned Manim CE angle/elbow control points, including Transform interiors.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/angle_marks.py tests/data/root/manim-angle-marks-0.22.1.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import Angle, Line, RightAngle, Transform, ORIGIN, linear

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
specs = [
    ([-2., 0., 0.], [2., 0., 0.], [0., -2., 0.], [0., 2., 0.], 0.),
    ([-2., -.8, 0.], [2., .4, 0.], [-.6, -2., 0.], [.8, 2., 0.], .7),
    ([-.2, 0., 0.], [.35, 0., 0.], [0., -.4, 0.], [0., .2, 0.], 0.),
    ([1., 0., 0.], [2., 0., 0.], [0., 1., 0.], [0., 2., 0.], .5),
]
records = []
alphas = [0., .125, .25, .5, .75, .875, 1.]
for a, b, c, d, radius in specs:
    for quadrant in [(1, 1), (1, -1), (-1, 1), (-1, -1)]:
        for kind in ["ccw", "cw", "dot", "elbow"]:
            options = {"quadrant": quadrant}
            if radius:
                options["radius"] = radius
            if kind == "elbow":
                if "radius" in options:
                    options["length"] = options.pop("radius")
                mark = RightAngle(Line(a, b), Line(c, d), **options)
            else:
                mark = Angle(Line(a, b), Line(c, d), other_angle=kind == "cw", dot=kind == "dot", **options)
            record = {"a": a, "b": b, "c": c, "d": d, "radius": radius,
                      "quadrant": quadrant, "kind": kind}
            if kind != "elbow":
                record["sweep"] = mark.get_value()
            states = []
            for alpha in alphas:
                obj = mark.copy()
                target = mark.copy().scale(1.3, about_point=ORIGIN).rotate(.65, about_point=ORIGIN).shift(np.array([1.2, -.7, 0.]))
                animation = Transform(obj, target, rate_func=linear)
                animation.begin()
                animation.interpolate(alpha)
                if alpha == 1:
                    animation.finish()
                states.append({"alpha": alpha, "points": obj.points.tolist(),
                               "children": [child.points.tolist() for child in obj.submobjects]})
            record["states"] = states
            records.append(record)
with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": COMMIT,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "64 arc/dot/elbow constructions, four quadrants, auto/explicit radii, intersection outside segments; 448 equal-topology linear Transform states. XY only; not full API, styles, updater or raster equivalence.",
               "cases": records}, stream, indent=2)
    stream.write("\n")
