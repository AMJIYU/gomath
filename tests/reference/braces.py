"""Pinned Brace/BraceBetweenPoints construction and Transform control states.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/braces.py tests/data/root/manim-braces-0.22.1.json
Development only; no Go construction/output is read.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import Brace, BraceBetweenPoints, VMobject, VGroup, Transform, linear, ORIGIN

commit = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == commit
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
rectangle = np.array([[-1., -.5, 0.], [1., -.5, 0.], [1., .5, 0.], [-1., .5, 0.]])
angle = .31
rotation = np.array([[np.cos(angle), -np.sin(angle), 0.], [np.sin(angle), np.cos(angle), 0.], [0., 0., 1.]])
targets = [
    [{"points": rectangle.tolist(), "closed": True}],
    [{"points": (rectangle @ rotation.T + np.array([.6, -.4, .2])).tolist(), "closed": True}],
    [{"points": [[-1.2, -.4, .3], [1.7, -.6, .3], [.4, 1.3, .3]], "closed": True}],
    [{"points": (rectangle*.2 + np.array([-1., .4, .2])).tolist(), "closed": True},
     {"points": [[-.6, -.6, .2], [.1, .3, .2]], "closed": False}],
]


def capture(brace, **metadata):
    states = []
    for alpha in [0., .125, .25, .5, .75, .875, 1.]:
        obj = brace.copy()
        destination = brace.copy().scale(1.1, about_point=ORIGIN).rotate(-.4, about_point=ORIGIN).shift(np.array([.6, -.4, .2]))
        animation = Transform(obj, destination, rate_func=linear)
        animation.begin()
        animation.interpolate(alpha)
        if alpha == 1:
            animation.finish()
        states.append({"alpha": alpha, "points": obj.get_all_points().tolist()})
    return {**metadata, "tip": brace.get_tip().tolist(), "direction_result": brace.get_direction().tolist(), "states": states}


cases = []
for parts in targets:
    objects = []
    for part in parts:
        points = list(part["points"])
        if part["closed"]:
            points += [points[0]]
        objects.append(VMobject().set_points_as_corners(np.array(points)))
    target = VGroup(*objects)
    for direction in [[0., -1., 0.], [1., 0., 0.], [-.6, .8, 0.]]:
        for sharpness in [.3, 2.]:
            buffer = .12
            brace = Brace(target, direction=np.array(direction), buff=buffer, sharpness=sharpness)
            cases.append(capture(brace, kind="target", parts=parts, direction=direction, buffer=buffer, sharpness=sharpness))
for i, (a, b) in enumerate([
    ([-1., 0., 0.], [1., 0., 0.]), ([1., 0., 0.], [-1., 0., 0.]),
    ([.2, -1., .3], [.2, 1., .3]), ([.2, 1., .3], [.2, -1., .3]),
    ([-1., -.7, 0.], [.9, 1.2, 0.]), ([.9, 1.2, 0.], [-1., -.7, 0.]),
    ([0., 0., 0.], [.3, .4, 0.]), ([0., 0., 0.], [3., -1., 0.]),
]):
    direction = [0., 0., 0.] if i < 6 else [0., -1., 0.]
    buffer, sharpness = .05 + .02*i, .4 if i % 2 == 0 else 3.
    brace = BraceBetweenPoints(np.array(a), np.array(b), direction=np.array(direction), buff=buffer, sharpness=sharpness)
    cases.append(capture(brace, kind="points", start=a, end=b, direction=direction, buffer=buffer, sharpness=sharpness))

with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": commit,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "32 straight-target/endpoint brace constructions and224 equal-topology linear Transform states, ordered complete controls and tips. Does not verify curved target extrema, arbitrary plane, labels, ArcBrace or full lifecycle/style parity.",
               "cases": cases}, stream, indent=2)
    stream.write("\n")
