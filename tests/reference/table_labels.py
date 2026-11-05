"""Pinned Manim table geometry, using rectangles to exclude font differences.

uv run --quiet --offline --isolated --no-project --with ../manim python \
  tests/reference/table_labels.py tests/data/table/manim-labels-0.22.1.json
Development reference only; never used by the Go runtime or Go tests.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import ORIGIN, Rectangle, Table
import numpy as np

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
output = Path(sys.argv[1])
assert not output.exists()

def shape(size):
    return Rectangle(width=size[0], height=size[1], stroke_width=0)

data = [[[.4,.3],[1.2,.6],[.7,.4]],[[.8,.9],[.6,.4],[.3,.7]]]
rows, columns = [[1.4,.5],[.7,.8]], [[.9,1.1],[.5,.4],[1.6,.5]]
cases = []
for kind in ["none", "rows", "columns", "both", "corner"]:
    row_sizes = rows if kind in ["rows", "both", "corner"] else []
    column_sizes = columns if kind in ["columns", "both", "corner"] else []
    corner = [1.7,1.3] if kind == "corner" else None
    for angle, shift in [(0,[0,0,0]),(.7,[1.2,-.8,0]),(-.4,[-.6,1.1,0])]:
        tab = Table(
            [[shape(size) for size in row] for row in data],
            element_to_mobject=lambda item: item,
            row_labels=[shape(size) for size in row_sizes] or None,
            col_labels=[shape(size) for size in column_sizes] or None,
            top_left_entry=shape(corner) if corner else None,
            h_buff=.5, v_buff=.36, include_outer_lines=True,
        )
        tab.rotate(angle,about_point=ORIGIN).shift(np.array(shift))
        entries = []
        for i, row in enumerate(tab.mob_table):
            for j, item in enumerate(row):
                if len(item.get_all_points()) != 0:
                    entries.append({"row":i,"column":j,"center":item.get_center().tolist(),
                                    "size":[item.width,item.height]})
        rules = []
        for line in [*tab.get_horizontal_lines(),*tab.get_vertical_lines()]:
            a,b = line.get_start(),line.get_end()
            rules.append({"start":a.tolist(),"end":b.tolist()})
        cases.append({"name":kind,"data":data,"row_labels":row_sizes,
                      "column_labels":column_sizes,"corner":corner,"angle":angle,"shift":shift,
                      "entries":entries,"rules":rules})

output.parent.mkdir(parents=True,exist_ok=True)
with output.open("x") as stream:
    json.dump({"source":{"version":manim.__version__,"commit":COMMIT,
                         "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope":"15 static table geometries: rectangular data/header ink, optional corner, rigid transforms, line endpoints. No fonts, animation defaults, spans, alignment variants or complete Table parity.",
               "cases":cases},stream,indent=2)
    stream.write("\n")
