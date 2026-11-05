"""Pinned Manim scalar scaling and oriented NumberLine mapping; development only.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/scales.py tests/data/graph/manim-scales-0.22.1.json
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import LinearBase, LogBase, NumberLine, ORIGIN
import numpy as np

COMMIT="16ea443e2a5f1d79db0a721161411ce916d6e733"
source=Path(__file__).resolve().parents[3]/"manim"
assert subprocess.check_output(["git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==COMMIT
assert manim.__version__=="0.22.1"
output=Path(sys.argv[1]);assert not output.exists()
cases=[]
for kind,parameter in [("linear",1),("linear",2.5),("log",2),("log",np.e),("log",10)]:
    for angle in [0,.6]:
        scaling=LinearBase(parameter) if kind=="linear" else LogBase(parameter,custom_labels=False)
        origin=np.array([1.2,-.7,0.])
        line=NumberLine(x_range=[-2,3,1],length=8,scaling=scaling,include_numbers=False,include_tip=False)
        line.rotate(angle,about_point=ORIGIN).shift(origin)
        normal=np.array([-np.sin(angle),np.cos(angle),0.])
        samples=[]
        for coordinate in [-3,-2,-1.5,-1,0,.5,2,3,4]:
            value=scaling.function(coordinate)
            point=line.n2p(value)
            projected=point+normal*.3
            samples.append({"coordinate":coordinate,"value":value,"point":point.tolist(),
                            "projected_point":projected.tolist(),"inverse":float(line.p2n(projected))})
        cases.append({"kind":kind,"parameter":parameter,"angle":angle,"origin":origin.tolist(),
                      "range":[-2,3,1],"length":8,"samples":samples,
                      "ticks":[{"value":float(value),"center":line.n2p(value).tolist()} for value in line.get_tick_range()]})
output.parent.mkdir(parents=True,exist_ok=True)
with output.open("x") as stream:
    json.dump({"source":{"version":manim.__version__,"commit":COMMIT,
                         "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope":"Ten scalar/NumberLine configurations,90 mappings/inverses and60 tick centers. Identity/positive linear and log base>1 only; no glyphs, Manim Axes placement, transformed mapping or animation defaults.",
               "cases":cases},stream,indent=2);stream.write("\n")
