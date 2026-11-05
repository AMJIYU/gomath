"""Development-only pinned Manim growing and indication reference.

uv run --quiet --offline --isolated --no-project --with ../manim python \
    tests/reference/indication.py artifacts/reference/indication-rerun.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import VMobject, GrowFromPoint, GrowFromCenter, GrowFromEdge
from manim import SpinInFromNothing, Indicate, Wiggle, ApplyWave, linear

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"

def capture(kind, alpha):
    corners=np.array([[-2.,-1.,0.],[2.,-1.,0.],[2.,1.,0.],[-2.,1.,0.]])+np.array([1.,.5,0.])
    points=[]
    for i in range(4):
        a,b=corners[i],corners[(i+1)%4]
        points.extend([a,2*a/3+b/3,a/3+2*b/3,b])
    obj=VMobject().set_points(np.array(points))
    if kind=="point": anim=GrowFromPoint(obj,np.array([-3.,2.,0.]))
    elif kind=="center": anim=GrowFromCenter(obj)
    elif kind=="edge": anim=GrowFromEdge(obj,np.array([1.,1.,0.]))
    elif kind=="spin": anim=SpinInFromNothing(obj,angle=2*np.pi)
    elif kind=="indicate": anim=Indicate(obj)
    elif kind=="wiggle": anim=Wiggle(obj)
    elif kind=="wave": anim=ApplyWave(obj)
    elif kind=="wave_ripples": anim=ApplyWave(obj,direction=np.array([1.,2.,0.]),amplitude=.4,time_width=.5,ripples=3,rate_func=linear)
    else: raise ValueError(kind)
    anim.begin()
    anim.interpolate(alpha)
    return {"kind":kind,"alpha":alpha,"points":obj.points.tolist()}

records=[capture(kind,t) for kind in ["point","center","edge","spin","indicate","wiggle","wave","wave_ripples"]
         for t in [0.,.03125,.125,.25,.375,.5,.625,.75,.875,.96875,1.]]
result={"source":{"version":manim.__version__,"commit":COMMIT,
                  "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
        "cases":records,
        "limits":"One equal-topology vector rectangle, default easing and selected wave settings. Control points only; no arbitrary family bounds, style, GrowArrow, or raster parity claim."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result,out,indent=2)
    out.write("\n")
