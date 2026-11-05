"""Pinned ArcBetweenPoints controls and equal-topology Transform interiors.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/arc_connections.py tests/data/root/manim-arc-connections-0.22.1.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import ArcBetweenPoints, TangentialArc, Line, Transform, ORIGIN, linear

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
records = []
for start, end in [([-2., -.3, 0.], [1.5, .7, 0.]),
                   ([.2, .4, 0.], [.7, -1.1, 0.]),
                   ([-1.2, 0., .5], [1.1, .2, .5])]:
    options = [{"angle": angle} for angle in [-4.8, -np.pi, -np.pi/2, -.001, 0., .001, np.pi/2, np.pi, 4.8]]
    options += [{"radius": radius} for radius in [3., -3.]]
    for spec in options:
        arc = ArcBetweenPoints(start, end, **spec)
        states = []
        for alpha in [0., .125, .25, .5, .75, .875, 1.]:
            obj = arc.copy()
            target = arc.copy().scale(1.1, about_point=ORIGIN).rotate(-.4, about_point=ORIGIN).shift(np.array([.6, -.4, .2]))
            animation = Transform(obj, target, rate_func=linear)
            animation.begin()
            animation.interpolate(alpha)
            if alpha == 1:
                animation.finish()
            states.append({"alpha": alpha, "points": obj.points.tolist()})
        records.append({"start": start, "end": end, "options": spec,
                        "sweep": arc.angle, "states": states})
tangential = []
for a, b, c, d in [([-2., 0., 0.], [2., 0., 0.], [0., -2., 0.], [0., 2., 0.]),
                    ([-2., -.4, 0.], [2., .4, 0.], [-.4, -2., 0.], [.4, 2., 0.])]:
    for corner in [(1, 1), (1, -1), (-1, 1), (-1, -1)]:
        for radius in [.3, 1.2]:
            arc = TangentialArc(Line(a, b), Line(c, d), radius=radius, corner=corner)
            states = []
            for alpha in [0., .125, .25, .5, .75, .875, 1.]:
                obj = arc.copy()
                target = arc.copy().scale(1.1, about_point=ORIGIN).rotate(-.4, about_point=ORIGIN).shift(np.array([.6, -.4, .2]))
                animation = Transform(obj, target, rate_func=linear)
                animation.begin()
                animation.interpolate(alpha)
                if alpha == 1:
                    animation.finish()
                states.append({"alpha": alpha, "points": obj.points.tolist()})
            tangential.append({"a": a, "b": b, "c": c, "d": d, "radius": radius,
                               "corner": corner, "states": states})
with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": COMMIT,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "33 explicit sweep/radius/line constructions and231 linear Transform states, plus16 tangential fillets and112 Transform states. XY chords including constant nonzero Z. Every cubic control compared; not arbitrary 3D plane defaults, tips, updater, raster or complete API parity.",
               "cases": records, "tangential_cases": tangential}, stream, indent=2)
    stream.write("\n")
