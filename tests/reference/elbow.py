"""Development-only Manim Elbow control references, including Transform interiors."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import Elbow, Transform, ORIGIN, linear

COMMIT="16ea443e2a5f1d79db0a721161411ce916d6e733"
source=Path(__file__).resolve().parents[3]/"manim"
assert subprocess.check_output(["git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==COMMIT
assert manim.__version__=="0.22.1"
out=Path(sys.argv[1]);assert not out.exists()
cases=[]
for width in [.2,1.,2.3]:
    for angle in [0.,.4,np.pi/2,5*np.pi/4,-np.pi/3]:
        elbow=Elbow(width=width,angle=angle)
        states=[]
        for alpha in [0.,.125,.25,.5,.75,.875,1.]:
            obj=elbow.copy()
            target=elbow.copy().scale(1.1,about_point=ORIGIN).rotate(-.4,about_point=ORIGIN).shift(np.array([.6,-.4,.2]))
            animation=Transform(obj,target,rate_func=linear)
            animation.begin();animation.interpolate(alpha)
            if alpha==1:animation.finish()
            states.append({"alpha":alpha,"points":obj.points.tolist()})
        cases.append({"width":width,"angle":angle,"states":states})
with out.open("x") as stream:
    json.dump({"source":{"version":manim.__version__,"commit":COMMIT,
                          "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope":"15 Elbow constructions/105 linear Transform states; every ordered control. Geometry only, no style/raster/lifecycle parity.","cases":cases},stream,indent=2)
    stream.write("\n")
