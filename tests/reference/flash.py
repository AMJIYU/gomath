"""Development-only pinned ShowPartial endpoint and FadeOut cleanup evidence."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import VMobject, ShowPassingFlash, Uncreate, FadeOut, Scene, linear

COMMIT="16ea443e2a5f1d79db0a721161411ce916d6e733"
source=Path(__file__).resolve().parents[3]/"manim"
assert subprocess.check_output(["git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==COMMIT
assert manim.__version__=="0.22.1"

def rectangle():
    corners=np.array([[-2.,-1.,0.],[2.,-1.,0.],[2.,1.,0.],[-2.,1.,0.]])
    points=[]
    for i in range(4):
        a,b=corners[i],corners[(i+1)%4]
        points.extend([a,2*a/3+b/3,a/3+2*b/3,b])
    return VMobject().set_points(np.array(points))

records=[]
for kind in ["flash","flash_linear","uncreate"]:
    for alpha in [0.,.02,.1,.25,.5,.75,.9,.98,1.]:
        obj=rectangle()
        if kind=="flash": anim=ShowPassingFlash(obj,time_width=.4)
        elif kind=="flash_linear": anim=ShowPassingFlash(obj,time_width=.2,rate_func=linear)
        else: anim=Uncreate(obj)
        anim.begin()
        anim.interpolate(alpha)
        records.append({"kind":kind,"alpha":alpha,"start":obj.get_start().tolist(),"end":obj.get_end().tolist(),"active":bool(np.max(np.abs(obj.points-obj.points[0]))>1e-12)})
obj=rectangle().set_opacity(.4)
anim=FadeOut(obj)
anim.begin()
anim.finish()
anim.clean_up_from_scene(Scene())
result={"source":{"version":manim.__version__,"commit":COMMIT,"generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
        "cases":records,"fade_cleanup_opacity":obj.get_stroke_opacity(),
        "limits":"27 equal-topology rectangle path endpoint states and one FadeOut cleanup opacity. Not full partial-path control-point, family lag, uncreate author-object representation, fill raster or shared-target composition parity."}
with Path(sys.argv[1]).open("x") as out:
    json.dump(result,out,indent=2)
    out.write("\n")
