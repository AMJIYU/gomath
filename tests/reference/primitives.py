"""Pinned development-only Manim primitive raster; no Go runtime dependency."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
from manim import Annulus, AnnularSector, Ellipse, RoundedRectangle, Sector, Camera, WHITE, BLACK
from PIL import Image
import numpy as np

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
out = Path(sys.argv[1])
meta = out.with_suffix(".json")
assert not out.exists() and not meta.exists()
shapes = [
    Ellipse(width=2.5, height=1.2).shift(np.array([-3.,1.,0.])),
    Annulus(inner_radius=.4, outer_radius=.9).shift(np.array([0.,1.,0.])),
    AnnularSector(inner_radius=.35, outer_radius=1, start_angle=.3, angle=-4.2).shift(np.array([3.,1.,0.])),
    RoundedRectangle(width=3, height=1, corner_radius=.25).shift(np.array([-2.,-1.1,0.])),
    Sector(radius=.9, start_angle=.2, angle=2.3).shift(np.array([2.,-1.2,0.])),
]
for shape in shapes:
    shape.set_fill(WHITE, opacity=1).set_stroke(width=0)
camera = Camera(pixel_width=640, pixel_height=360, frame_width=10, frame_height=5.625, background_color=BLACK)
camera.capture_mobjects(shapes)
pixels = np.array(camera.pixel_array, copy=True)
Image.fromarray(pixels).save(out)
result = {"source":{"version":manim.__version__,"commit":COMMIT,"generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
          "png_sha256":hashlib.sha256(out.read_bytes()).hexdigest(),"width":640,"height":360,"frame_width":10,
          "limits":"Five static white filled shapes on black; no strokes or animation. Go uses four cubic arcs per full turn, Manim uses a different subdivision. Bounded raster comparison, not exact control-point or API parity."}
with meta.open("x") as f:
    json.dump(result,f,indent=2)
    f.write("\n")
