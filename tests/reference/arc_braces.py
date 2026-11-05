"""Pinned Manim ArcBrace controls and interior linear Transform states.

Development only: uv run --quiet --offline --isolated --no-project --with
../manim python tests/reference/arc_braces.py NEW_OUTPUT_JSON.
No Go output or construction is read; the Python reference stays unchanged.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import Arc, ArcBrace, Transform, linear, ORIGIN

root = Path(__file__).resolve().parents[2]
source = root.parent / "manim"
commit = "16ea443e2a5f1d79db0a721161411ce916d6e733"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == commit
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
cases = []
for ri, radius in enumerate([.2, .3, .5, 1., 1.5, 3.]):
    for direction in [-1., 1.]:
        for ai, (start, sweep) in enumerate([(-1., 2.), (0., np.pi/2), (.7, -np.pi*1.5)]):
            center = np.array([.23, -.37, 0.])
            buffer, sharpness = (.08, .3) if (ri+ai)%2 else (.2, 2.)
            arc = Arc(radius=radius, start_angle=start, angle=sweep, arc_center=center)
            brace = ArcBrace(arc, direction=np.array([direction, 0., 0.]), buff=buffer, sharpness=sharpness)
            states = []
            for alpha in [0., .125, .25, .5, .75, .875, 1.]:
                obj = brace.copy()
                target = brace.copy().scale(1.1, about_point=ORIGIN).rotate(-.4, about_point=ORIGIN).shift(np.array([.6, -.4, .2]))
                anim = Transform(obj, target, rate_func=linear)
                anim.begin()
                anim.interpolate(alpha)
                if alpha == 1:
                    anim.finish()
                states.append({"alpha": alpha, "points": obj.get_all_points().tolist()})
            cases.append({"center": center.tolist(), "radius": radius, "start": start, "sweep": sweep,
                          "direction": direction, "buffer": buffer, "sharpness": sharpness,
                          "tip": brace.get_tip().tolist(), "direction_result": brace.get_direction().tolist(), "states": states})
out.parent.mkdir(parents=True, exist_ok=True)
with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": commit,
                          "brace_source_sha256": hashlib.sha256((source/"manim/mobject/svg/brace.py").read_bytes()).hexdigest(),
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "36 explicit planar ArcBrace constructions,252 ordered equal-topology linear Transform states, tips and anchor-box directions. Includes inside/outside, radius floor/scaling and negative sweeps. No arbitrary arc recovery, clip/style/lifecycle or full ArcBrace parity claim.",
               "cases": cases}, stream, indent=2)
    stream.write("\n")
