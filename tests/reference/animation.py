"""Development-only Manim reference generator. Never imported by GoMath.

Run from gomath with the pinned sibling Manim source in an isolated uv env:
uv run --quiet --isolated --no-project --with ../manim python \
    tests/reference/animation.py tests/data/root/manim-animation-0.22.1.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import MovingCamera, ReplacementTransform, Rotate, Scene, Transform
from manim import ValueTracker, VMobject, linear, tempconfig

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
actual_commit = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
assert actual_commit == COMMIT, (actual_commit, COMMIT)
assert manim.__version__ == "0.22.1"


def square(side, center):
    x, y = center
    corners = np.array([[x-side/2,y-side/2,0], [x+side/2,y-side/2,0],
                        [x+side/2,y+side/2,0], [x-side/2,y+side/2,0]])
    points = []
    for i in range(4):
        a, b = corners[i], corners[(i+1) % 4]
        points.extend([a, a*2/3+b/3, a/3+b*2/3, b])
    return VMobject().set_points(np.array(points))


def capture(kind, alpha):
    with tempconfig({"dry_run": True, "media_dir": "/tmp/gomath-manim-behavior", "verbosity": "ERROR"}):
        if kind == "tracker":
            obj = ValueTracker(-2)
            anim = Transform(obj, ValueTracker(6), rate_func=linear)
        elif kind == "camera":
            camera = MovingCamera(frame_width=12, frame_height=6.75)
            obj = camera.frame.move_to(np.array([-1., .5, 0.]))
            target = obj.copy().scale(.5).move_to(np.array([2., -1., 0.]))
            anim = Transform(obj, target, rate_func=linear)
        else:
            obj = square(2, (-2, 0))
            if kind == "shift":
                anim = Transform(obj, obj.copy().shift(np.array([4., 1., 0.])), rate_func=linear)
            elif kind == "rotate":
                anim = Rotate(obj, angle=2*np.pi, rate_func=linear)
            elif kind == "replacement":
                target = square(1, (1, -1))
                scene = Scene()
                scene.add(obj)
                anim = ReplacementTransform(obj, target, rate_func=linear)
            else:
                raise ValueError(kind)
        anim.begin()
        anim.interpolate(alpha)
        result = {"kind": kind, "alpha": alpha}
        if kind == "tracker":
            result["value"] = float(obj.get_value())
        elif kind == "camera":
            result.update(center=obj.get_center().tolist(), width=float(obj.width))
        else:
            result["points"] = obj.points.tolist()
        if kind == "replacement":
            if alpha == 1:
                anim.finish()
                anim.clean_up_from_scene(scene)
            result.update(source_present=obj in scene.mobjects, target_present=target in scene.mobjects)
        return result


records = [capture(kind, a) for kind in ["shift", "rotate", "replacement", "tracker", "camera"]
           for a in [0., .125, .25, .5, .75, .875, 1.]]
result = {"source": {"version": manim.__version__, "commit": COMMIT,
                     "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
          "cases": records,
          "limits": "Linear rates; one equal-topology square; full-turn rotation; root replacement; scalar tracker; camera pan/zoom. No default-rate, child alignment or general parity claim."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result, out, indent=2)
    out.write("\n")
