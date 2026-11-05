"""Pinned vector pointwise mappings and seven interior Transform captures.

Development only. Use the pinned offline uv Manim environment and a new output
path. Does not read Go output or provide a runtime mapping fallback.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import VMobject, VGroup, Transform, linear, ORIGIN

root = Path(__file__).resolve().parents[2]
source = root.parent / "manim"
commit = "16ea443e2a5f1d79db0a721161411ce916d6e733"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == commit
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
assert not out.exists()
parts = [
    [[-1., -.2, .3], [-.8, .7, .3], [.2, -.9, .3], [.5, .3, .3],
     [.5, .3, .3], [.7, .8, .3], [1.3, .5, .3], [1.5, -.1, .3]],
    [[-.6, .1, -.2], [-.5, .9, -.2], [.5, .8, -.2], [.6, .1, -.2],
     [.6, .1, -.2], [.4, -.8, -.2], [-.4, -.7, -.2], [-.6, .1, -.2]],
]
cases = []
for kind in ["quadratic", "square", "exponential", "affine"]:
    for handle in [.01, .25, 1.]:
        for count in [1, 2]:
            children = [VMobject(pre_function_handle_to_anchor_scale_factor=handle).set_points(np.array(p)) for p in parts[:count]]
            obj = children[0] if count == 1 else VGroup(children[0], VGroup(children[1]))
            if kind == "quadratic":
                obj.apply_function(lambda p: np.array([p[0]+.15*p[1]**2, p[1]+.25*p[0]**2, p[2]+.1*p[0]*p[1]]))
            elif kind == "affine":
                obj.apply_function(lambda p: np.array([1.2*p[0]+.3*p[1]+.4, -.2*p[0]+.7*p[1]-.1, p[2]+.2]))
            elif kind == "square":
                obj.apply_complex_function(lambda z: z*z)
            else:
                obj.apply_complex_function(lambda z: np.exp(.4*z))
            # VGroup.apply_function contracts its whole family with its own
            # factor. Set the group factor explicitly before regenerating.
            if count == 2 and handle != .01:
                children = [VMobject().set_points(np.array(p)) for p in parts[:count]]
                obj = VGroup(children[0], VGroup(children[1]), pre_function_handle_to_anchor_scale_factor=handle)
                if kind == "quadratic":
                    obj.apply_function(lambda p: np.array([p[0]+.15*p[1]**2, p[1]+.25*p[0]**2, p[2]+.1*p[0]*p[1]]))
                elif kind == "affine":
                    obj.apply_function(lambda p: np.array([1.2*p[0]+.3*p[1]+.4, -.2*p[0]+.7*p[1]-.1, p[2]+.2]))
                elif kind == "square":
                    obj.apply_complex_function(lambda z: z*z)
                else:
                    obj.apply_complex_function(lambda z: np.exp(.4*z))
            states = []
            for alpha in [0., .125, .25, .5, .75, .875, 1.]:
                current = obj.copy()
                target = obj.copy().scale(1.1, about_point=ORIGIN).rotate(-.4, about_point=ORIGIN).shift(np.array([.6, -.4, .2]))
                animation = Transform(current, target, rate_func=linear)
                animation.begin()
                animation.interpolate(alpha)
                if alpha == 1:
                    animation.finish()
                states.append({"alpha": alpha, "points": current.get_all_points().tolist()})
            cases.append({"kind": kind, "handle": handle, "parts": parts[:count], "states": states})
out.parent.mkdir(parents=True, exist_ok=True)
with out.open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": commit,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "24 explicit cubic-family point/complex mappings at three handle scales and168 ordered linear Transform states. Tests origin mapping and preserved complex Z; not adaptive curve subdivision, arbitrary paint/clip deformation, updater or every apply_function semantic.",
               "cases": cases}, stream, indent=2)
    stream.write("\n")
