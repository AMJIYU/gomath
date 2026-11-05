"""Pinned Manim signed-bar geometry, including numeric-update intermediate states.

uv run --quiet --offline --isolated --no-project --with ../manim python \
 tests/reference/barchart.py tests/data/graph/manim-bars-0.22.1.json

No text or external TeX is used. Updates call change_bar_values on explicitly
interpolated data; this does not validate Manim Transform's contour matching.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import BarChart, ORIGIN
import numpy as np

COMMIT="16ea443e2a5f1d79db0a721161411ce916d6e733"
source=Path(__file__).resolve().parents[3]/"manim"
assert subprocess.check_output(["git","-C",str(source),"rev-parse","HEAD"],text=True).strip()==COMMIT
assert manim.__version__=="0.22.1"
out=Path(sys.argv[1]);assert not out.exists()
cases=[]
start=np.array([-2.,0.,1.,4.]);end=np.array([3.,2.,-1.,0.])
for angle in [0,.42]:
    for alpha in [0,.125,.25,.5,.75,.875,1]:
        chart=BarChart(start.tolist(), y_range=[-3,5,1],x_length=8,y_length=4,
                       bar_width=.6,bar_stroke_width=0,bar_fill_opacity=.7,
                       axis_config={"include_ticks":False},
                       y_axis_config={"include_numbers":False},tips=False)
        values=(1-alpha)*start+alpha*end
        chart.change_bar_values(values.tolist())
        chart.shift(-chart.c2p(0,0))
        chart.rotate(angle,about_point=ORIGIN).shift([1.2,-.7,0])
        cases.append({"alpha":alpha,"angle":angle,"shift":[1.2,-.7,0],
                      "start":start.tolist(),"end":end.tolist(),"values":values.tolist(),
                      "vertices":[bar.get_vertices().tolist() for bar in chart.bars]})
out.parent.mkdir(parents=True,exist_ok=True)
with out.open("x") as stream:
    json.dump({"source":{"version":manim.__version__,"commit":COMMIT,
                         "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope":"14 four-bar states: explicit numeric update at seven alpha values, then two rigid poses. Baseline normalized to c2p(0,0). No labels, automatic range/defaults or Transform contour matching.",
               "cases":cases},stream,indent=2);stream.write("\n")
