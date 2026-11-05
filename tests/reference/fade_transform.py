"""Development-only pinned two-layer FadeTransform rectangle geometry/opacity."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import FadeTransform, VMobject, linear

COMMIT="16ea443e2a5f1d79db0a721161411ce916d6e733"
source=Path(__file__).resolve().parents[3]/"manim"
assert subprocess.check_output(["git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==COMMIT
assert manim.__version__=="0.22.1"


def rectangle(w,h,x,y):
    corners=np.array([[-w/2+x,-h/2+y,0.],[w/2+x,-h/2+y,0.],[w/2+x,h/2+y,0.],[-w/2+x,h/2+y,0.]])
    points=[]
    for i in range(4):
        a,b=corners[i],corners[(i+1)%4]
        points.extend([a,a+(b-a)/3,a+2*(b-a)/3,b])
    return VMobject().set_points(np.array(points)).set_fill(opacity=1)


def capture(kind,alpha):
    a,b=rectangle(4,2,-2,0),rectangle(2,4,2,1)
    anim=FadeTransform(a,b,stretch=kind=="stretch",dim_to_match=0 if kind=="width" else 1,rate_func=linear)
    anim.begin()
    anim.interpolate(alpha)
    layers=[{"points":o.points.tolist(),"opacity":o.get_fill_opacity()} for o in anim.mobject]
    return {"kind":kind,"alpha":alpha,"layers":layers}


records=[capture(kind,alpha) for kind in ["stretch","width","height"]
         for alpha in [0.,.03125,.125,.25,.375,.5,.625,.75,.875,.96875,1.]]
result={"source":{"version":manim.__version__,"commit":COMMIT,"generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
        "cases":records,"limits":"33 two-layer equal-topology rectangle geometry/opacity states before cleanup. Does not prove arbitrary nested opacity, differing bitmap assets, actual source/target scene-ID timing, group cleanup or full FadeTransform parity."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result,out,indent=2)
    out.write("\n")
