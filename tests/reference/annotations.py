"""Pinned straight-path shape-matchers and equal-topology Transform interiors.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/annotations.py tests/data/root/manim-annotations-0.22.1.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import (VMobject, VGroup, SurroundingRectangle, BackgroundRectangle,
                   Underline, Cross, Transform, ORIGIN, linear, BLACK)

COMMIT="16ea443e2a5f1d79db0a721161411ce916d6e733"
source=Path(__file__).resolve().parents[3]/"manim"
assert subprocess.check_output(["git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==COMMIT
assert manim.__version__=="0.22.1"
out=Path(sys.argv[1]);assert not out.exists()
rectangle=np.array([[-1.,-.5,0.],[1.,-.5,0.],[1.,.5,0.],[-1.,.5,0.]])
rotation=np.array([[np.cos(.31),-np.sin(.31),0.],[np.sin(.31),np.cos(.31),0.],[0.,0.,1.]])
targets=[
    [{"points":rectangle.tolist(),"closed":True}],
    [{"points":(rectangle@rotation.T+np.array([.6,-.4,.2])).tolist(),"closed":True}],
    [{"points":[[-1.2,-.4,.3],[1.7,-.6,.3],[.4,1.3,.3]],"closed":True}],
    [{"points":(rectangle+np.array([-1.,.4,.2])).tolist(),"closed":True},
     {"points":[[1.,-1.,-.2],[2.,1.,.4]],"closed":False}],
]
cases=[]
for parts in targets:
    objects=[]
    for part in parts:
        points=list(part["points"])
        if part["closed"]:points=points+[points[0]]
        objects.append(VMobject().set_points_as_corners(np.array(points)))
    target=VGroup(*objects)
    for variant in range(3):
        px,py=[(0.,0.),(.1,.2),(.4,.05)][variant]
        buffer=[0.,.1,.25][variant]
        scale=[.6,1.,1.3][variant]
        for kind in ["surrounding","background","underline","cross"]:
            if kind=="surrounding":annotation=SurroundingRectangle(target,buff=(px,py),corner_radius=0.)
            elif kind=="background":annotation=BackgroundRectangle(target,buff=(px,py),corner_radius=0.,color=BLACK)
            elif kind=="underline":annotation=Underline(target,buff=buffer)
            else:annotation=Cross(target,scale_factor=scale)
            states=[]
            for alpha in [0.,.125,.25,.5,.75,.875,1.]:
                obj=annotation.copy()
                destination=annotation.copy().scale(1.1,about_point=ORIGIN).rotate(-.4,about_point=ORIGIN).shift(np.array([.6,-.4,.2]))
                animation=Transform(obj,destination,rate_func=linear)
                animation.begin();animation.interpolate(alpha)
                if alpha==1:animation.finish()
                states.append({"alpha":alpha,"points":obj.get_all_points().tolist()})
            cases.append({"kind":kind,"parts":parts,"padding_x":px,"padding_y":py,
                          "buffer":buffer,"scale":scale,"states":states})
with out.open("x") as stream:
    json.dump({"source":{"version":manim.__version__,"commit":COMMIT,
                          "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope":"48 straight-path shape-matcher constructions;336 linear Transform states. Compare full cubic-control multisets, disregarding rectangle contour starting corner/winding. Not curved-control-hull bounds, rounded corner defaults, special BackgroundRectangle creation, style/raster/full API parity.",
               "cases":cases},stream,indent=2)
    stream.write("\n")
