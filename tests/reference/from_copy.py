"""Development-only pinned TransformFromCopy geometry and reversed-rate evidence."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import AnimationGroup, Succession, TransformFromCopy, VMobject, Wait, linear
from manim.utils.rate_functions import ease_in_quad, ease_out_back

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"


def rectangle(w, h, x, y):
    corners = np.array([[-w/2+x,-h/2+y,0.],[w/2+x,-h/2+y,0.],[w/2+x,h/2+y,0.],[-w/2+x,h/2+y,0.]])
    points=[]
    for i in range(4):
        a,b=corners[i],corners[(i+1)%4]
        points.extend([a,a+(b-a)/3,a+2*(b-a)/3,b])
    return VMobject().set_points(np.array(points))


def capture(kind, alpha):
    source, target = rectangle(4,2,-2,0), rectangle(2,4,2,1)
    rate={"linear":linear,"quad":ease_in_quad,"back":ease_out_back,"grouped":ease_in_quad}[kind]
    anim=TransformFromCopy(source,target,rate_func=rate)
    if kind=="grouped":
        # Mirror Go's leaf window with a group clock and delayed local interval.
        # Use AnimationGroup's explicit timings to keep source/target preparation
        # independent of Succession's delayed scene setup.
        anim=AnimationGroup(anim,rate_func=ease_in_quad)
        anim.anims_with_timings["start"][:]=.2
        anim.anims_with_timings["end"][:]=.8
        anim.max_end_time=1.
    anim.begin()
    anim.interpolate(alpha)
    if alpha==1: anim.finish()
    return {"kind":kind,"alpha":alpha,"points":target.points.tolist()}


records=[capture(kind,alpha) for kind in ["linear","quad","back","grouped"]
         for alpha in [0.,.03125,.125,.25,.375,.5,.625,.75,.875,.96875,1.]]
result={"source":{"version":manim.__version__,"commit":COMMIT,"generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
        "cases":records,"limits":"44 equal-topology rectangle control-point states; nonsymmetric and overshooting rates, group clock plus leaf window. Group interval set directly for comparison because Go windows are an explicit scheduling API. No arbitrary topology, bitmap conversion or whole API parity."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result,out,indent=2)
    out.write("\n")
