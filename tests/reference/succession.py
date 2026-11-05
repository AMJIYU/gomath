"""Development-only pinned shared-object Succession control-point reference.

uv run --quiet --offline --isolated --no-project --with ../manim python \
    tests/reference/succession.py <fresh-output.json>
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import ApplyMethod, Rotating, Scene, Succession, VMobject, linear

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"


def capture(kind, alpha):
    corners = np.array([[-2., -1., 0.], [2., -1., 0.], [2., 1., 0.], [-2., 1., 0.]])
    points = []
    for i in range(4):
        a, b = corners[i], corners[(i+1) % 4]
        points.extend([a, a+(b-a)/3, a+2*(b-a)/3, b])
    obj = VMobject().set_points(np.array(points))

    def shift(x, duration=1):
        # ApplyMethod creates its target at begin, including in a Succession.
        # Animation builders that precompute targets have different semantics.
        return ApplyMethod(obj.shift, np.array([x, 0., 0.]), run_time=duration, rate_func=linear)

    if kind == "shift":
        anim = Succession(shift(2), shift(4, 2), ApplyMethod(obj.move_to, np.array([-2., 0., 0.]), rate_func=linear))
    elif kind == "nested":
        anim = Succession(shift(2), Succession(shift(4), ApplyMethod(obj.scale, 2., run_time=3, rate_func=linear), run_time=2))
    elif kind == "rotate":
        anim = Succession(shift(3), Rotating(obj, angle=np.pi, about_edge=None, rate_func=linear, run_time=1),
                          Rotating(obj, angle=np.pi, about_edge=None, rate_func=linear, run_time=1))
    else:
        raise ValueError(kind)
    anim._setup_scene(Scene())
    anim.begin()
    anim.interpolate(alpha)
    if alpha == 1:
        anim.finish()
    return {"kind": kind, "alpha": alpha, "points": obj.points.tolist()}


records = [capture(kind, alpha) for kind in ["shift", "nested", "rotate"]
           for alpha in [0., .025, .0625, .125, .2, .25, 1/3, .375, .5, .625, 2/3, .7, .75, .8, .875, .975, 1.]]
result = {"source": {"version": manim.__version__, "commit": COMMIT,
                     "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
          "cases": records,
          "limits": "51 control-point states for shared relative motion, deferred centers, unequal and nested linear sequence timing. No warped sequence clock, parallel-contained Succession, introducer/remover/replacement cleanup parity or arbitrary TeX/API coverage. GoMath performs cleanup at each step boundary; Manim Succession delays some scene cleanup until the outer animation cleanup."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result, out, indent=2)
    out.write("\n")
