"""Pinned Manim put_at_tip default next-to policy for rectangular ink labels."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import Brace, Rectangle

source = Path(__file__).resolve().parents[3] / "manim"
commit = "16ea443e2a5f1d79db0a721161411ce916d6e733"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == commit
assert manim.__version__ == "0.22.1"
cases = []
for direction in [[0., -1., 0.], [0., 1., 0.], [-1., 0., 0.], [1., 0., 0.], [-.6, .8, 0.], [.8, -.6, 0.]]:
    for buffer in [0., .15]:
        brace = Brace(Rectangle(width=2., height=1.), direction=np.array(direction))
        label = Rectangle(width=.8, height=.3).shift(np.array([3., -1., .4]))
        brace.put_at_tip(label, buff=buffer)
        cases.append({"direction": direction, "buffer": buffer,
                      "label_min": label.get_corner(np.array([-1., -1., -1.])).tolist(),
                      "label_max": label.get_corner(np.array([1., 1., 1.])).tolist()})
with Path(sys.argv[1]).open("x") as stream:
    json.dump({"source": {"version": manim.__version__, "commit": commit,
                          "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
               "scope": "12 rectangular label placements; no font, curved-ink, live reflow or use_next_to=False claim.",
               "cases": cases}, stream, indent=2)
    stream.write("\n")
