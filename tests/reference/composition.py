"""Development-only pinned Manim animation-group reference.

uv run --quiet --offline --isolated --no-project --with ../manim python \
    tests/reference/composition.py artifacts/reference/composition-rerun.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import AnimationGroup, LaggedStart, Transform, ValueTracker
from manim import linear, smooth, there_and_back

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"

def capture(kind, alpha):
    objects = [ValueTracker(x) for x in [-2., 1., 4.]]
    def leaf(i, duration, rate=linear):
        return Transform(objects[i], ValueTracker([2., 7., -2.][i]), run_time=duration, rate_func=rate)
    if kind == "parallel":
        anim = AnimationGroup(leaf(0, 2), leaf(1, 1), leaf(2, .5))
    elif kind == "long_first":
        anim = LaggedStart(leaf(0, 10), leaf(1, 1), leaf(2, 2), lag_ratio=.1)
    elif kind == "nested":
        inner = LaggedStart(leaf(0, 3, smooth), leaf(1, .5), lag_ratio=.2, run_time=2, rate_func=smooth)
        anim = LaggedStart(inner, leaf(2, 1), lag_ratio=.5, run_time=4, rate_func=smooth)
    elif kind == "returning":
        anim = LaggedStart(leaf(0, 1), leaf(1, 2), leaf(2, .5), lag_ratio=.3, rate_func=there_and_back)
    else:
        raise ValueError(kind)
    anim.begin()
    anim.interpolate(alpha)
    # Manim's finish recursively completes children independently of the parent rate.
    if alpha == 1:
        anim.finish()
    return {"kind": kind, "alpha": alpha, "values": [o.get_value() for o in objects]}

records = [capture(kind, t) for kind in ["parallel", "long_first", "nested", "returning"]
           for t in [0., .03125, .125, .25, .375, .5, .625, .75, .875, .96875, 1.]]
result = {"source": {"version": manim.__version__, "commit": COMMIT,
                     "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
          "cases": records,
          "limits": "Distinct scalar tracker transforms, unequal durations, recursive group clocks and finish. No shared-object Succession, cleanup/introducer timing or arbitrary composition parity."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result, out, indent=2)
    out.write("\n")
