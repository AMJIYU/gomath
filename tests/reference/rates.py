"""Development-only source-pinned easing and transform reference.

uv run --quiet --isolated --no-project --with ../manim python \
    tests/reference/rates.py artifacts/reference/rates-rerun.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import Transform, VMobject
from manim.utils import rate_functions as rf

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
actual = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
assert actual == COMMIT
assert manim.__version__ == "0.22.1"

names = ["linear", "smooth", "smoothstep", "smootherstep", "smoothererstep",
         "rush_into", "rush_from", "slow_into", "double_smooth", "there_and_back",
         "there_and_back_with_pause", "running_start", "wiggle", "lingering", "exponential_decay"]
names += ["ease_" + mode + "_" + curve
          for curve in ["sine", "quad", "cubic", "quart", "quint", "expo", "circ", "back", "elastic", "bounce"]
          for mode in ["in", "out", "in_out"]]
functions = {name: getattr(rf, name) for name in names}
functions.update({
    "smooth_3": lambda t: rf.smooth(t, 3),
    "there_and_back_3": lambda t: rf.there_and_back(t, 3),
    "rush_into_3": lambda t: rf.rush_into(t, 3),
    "rush_from_3": lambda t: rf.rush_from(t, 3),
    "pause_half": lambda t: rf.there_and_back_with_pause(t, .5),
    "wiggle_3": lambda t: rf.wiggle(t, 3),
    "running_start_quarter": lambda t: rf.running_start(t, -.25),
    "decay_quarter": lambda t: rf.exponential_decay(t, .25),
    "partial": rf.not_quite_there(),
    "squish": rf.squish_rate_func(rf.smooth),
    "squish_equal": rf.squish_rate_func(rf.smooth, .4, .4),
})
times = [-.25, 0., .03125, .125, .25, .375, .5, .625, .75, .875, .96875, 1., 1.25]
records = [{"name": name, "alpha": t, "value": float(function(t))}
           for name, function in functions.items() for t in times]

# Verify that the animation engine itself preserves the selected rates.
animations = []
for name in ["smooth", "ease_in_back", "ease_out_back", "ease_out_elastic", "wiggle"]:
    for t in times[1:-1]:
        obj = VMobject().set_points_as_corners([np.array([0., 0., 0.]), np.array([1., 0., 0.])])
        anim = Transform(obj, obj.copy().shift(np.array([4., 0., 0.])),
                         **({} if name == "smooth" else {"rate_func": functions[name]}))
        anim.begin()
        anim.interpolate(t)
        animations.append({"name": name, "alpha": t, "x": float(obj.get_center()[0])})

result = {"source": {"version": manim.__version__, "commit": COMMIT,
                     "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
          "rates": records, "animations": animations,
          "limits": "Finite real scalar samples; default and selected parameterized rates, one equal-topology translated line. No general composition, style-overshoot or whole-API claim."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result, out, indent=2)
    out.write("\n")
