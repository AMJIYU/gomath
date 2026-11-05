"""Frozen ImageMobject uint8 oracle; development only, never a Go runtime path.

python3 tests/reference/image_pixels.py --source ../manim
Use --check to regenerate in memory and verify all frozen reference files.
Executes AST-extracted pinned Manim methods with only geometry-independent
state and a minimal color constructor. NumPy runs the original method bodies;
Pillow independently writes and reads PNGs. No installed Manim/native renderer
is needed. Raster frames below cover exact pixel-aligned affine placements,
not Manim camera/resampling parity.

Manim source is MIT, copyright The Manim Community Developers; see the pinned
repository LICENSE and GoMath THIRD_PARTY_NOTICES.md.
"""
from __future__ import annotations

import argparse
import ast
import hashlib
import io
import json
from pathlib import Path
import subprocess

import numpy as np
from PIL import Image, __version__ as pillow_version

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
ROOT = Path(__file__).resolve().parents[2]


def sha(data):
    return hashlib.sha256(data).hexdigest()


def premultiply(straight):
    p = straight.astype(np.uint32)
    p[..., :3] = ((p[..., :3] * (p[..., 3:4] * 257)) // 255) >> 8
    return p.astype(np.uint8)


def png_straight(premult):
    """8-bit canonical unpremultiplication used when exporting a Go Bitmap."""
    p = premult.astype(np.uint32)
    a = p[..., 3:4]
    p[..., :3] = np.where(a == 0, 0, (p[..., :3] * 65535 // np.maximum(a, 1)) >> 8)
    return p.astype(np.uint8)


def generate(source):
    head = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
    if head != COMMIT:
        raise SystemExit(f"reference HEAD {head} is not pinned {COMMIT}")
    paths = ["manim/mobject/types/image_mobject.py", "manim/utils/color/core.py", "manim/utils/images.py"]
    blobs = {p: subprocess.check_output(["git", "-C", str(source), "show", f"{COMMIT}:{p}"]) for p in paths}
    image_tree = ast.parse(blobs[paths[0]])
    color_tree = ast.parse(blobs[paths[1]])
    image_class = next(n for n in image_tree.body if isinstance(n, ast.ClassDef) and n.name == "ImageMobject")
    methods = {n.name: n for n in image_class.body if isinstance(n, ast.FunctionDef)}
    color_class = next(n for n in color_tree.body if isinstance(n, ast.ClassDef) and n.name == "ManimColor")
    to_rgb = next(n for n in color_class.body if isinstance(n, ast.FunctionDef) and n.name == "to_int_rgb")
    helper = next(n for n in color_tree.body if isinstance(n, ast.FunctionDef) and n.name == "color_to_int_rgb")
    init_invert = next(n for n in methods["__init__"].body if isinstance(n, ast.If) and ast.unparse(n.test) == "self.invert_image")
    init_alpha = next(n for n in methods["__init__"].body if isinstance(n, ast.Assign) and ast.unparse(n.targets[0]) == "self.orig_alpha_pixel_array")
    future = ast.ImportFrom(module="__future__", names=[ast.alias(name="annotations")], level=0)

    def execute(nodes, env):
        module = ast.fix_missing_locations(ast.Module(body=[future] + nodes, type_ignores=[]))
        exec(compile(module, "<pinned-manim-image-pixels>", "exec"), env)

    class Color:
        def __init__(self, rgb):
            self._internal_value = np.array(rgb, dtype=np.float64)

    env = {"np": np, "ManimColor": Color, "YELLOW_C": (1., .86, .29)}
    execute([to_rgb, helper, methods["set_color"], methods["set_opacity"]], env)
    Color.to_int_rgb = env["to_int_rgb"]

    class Pixels:
        set_color = env["set_color"]
        set_opacity = env["set_opacity"]

        def __init__(self, array):
            self.pixel_array = array.copy()
            self.orig_alpha_pixel_array = array[..., 3].copy()
            self.submobjects = []

        def invert(self):
            self.invert_image = True
            self.pixel_array_dtype = "uint8"
            execute([init_invert], dict(env, self=self))

    files = {}

    def put_png(name, straight):
        buffer = io.BytesIO()
        Image.fromarray(straight).save(buffer, format="PNG")
        data = buffer.getvalue()
        decoded = np.array(Image.open(io.BytesIO(data)).convert("RGBA"))
        if not np.array_equal(straight, decoded):
            raise AssertionError("Pillow PNG roundtrip changed straight bytes")
        files[name] = data

    # Every alpha/channel pair appears in R/A, G/A and B/A (65536 pixels).
    v, a = np.meshgrid(np.arange(256, dtype=np.uint16), np.arange(256, dtype=np.uint16))
    raw = np.stack([v, 255-v, (37*v+13) % 256, a], axis=-1).astype(np.uint8)
    files["input.nrgba"] = raw.tobytes()
    put_png("input.png", raw)
    canonical = raw.copy()
    canonical[canonical[..., 3] == 0] = 0
    definitions = [
        ("original", []),
        ("invert", [{"op": "invert"}]),
        ("invert_twice", [{"op": "invert"}, {"op": "invert"}]),
        ("opacity_subbyte", [{"op": "opacity", "factor": 1/512}]),
        ("opacity", [{"op": "opacity", "factor": .37}]),
        ("opacity_restore", [{"op": "opacity", "factor": 0}, {"op": "opacity", "factor": 1}]),
        ("opacity_absolute", [{"op": "opacity", "factor": .25}, {"op": "opacity", "factor": .75}]),
        ("replace", [{"op": "color", "rgb": [.1, .501, 1.]}]),
        ("replace_alpha", [{"op": "color", "rgb": [.1, .501, 1.], "alpha": .5}]),
        ("replace_restore", [{"op": "color", "rgb": [.1, .501, 1.], "alpha": .5}, {"op": "opacity", "factor": .75}]),
        ("tint", [{"op": "tint", "rgb": [.37, .8, .125]}]),
        ("zero_invert_restore", [{"op": "opacity", "factor": 0}, {"op": "invert"}, {"op": "opacity", "factor": 1}]),
        ("chain", [{"op": "invert"}, {"op": "tint", "rgb": [.37, .8, .125]}, {"op": "opacity", "factor": .37}]),
    ]

    def run(array, steps):
        obj = Pixels(array)
        # Execute pinned initialization's actual saved-alpha statement as well.
        execute([init_alpha], dict(env, self=obj))
        for step in steps:
            if step["op"] == "invert":
                obj.invert()
            elif step["op"] == "opacity":
                obj.set_opacity(step["factor"])
            elif step["op"] == "color":
                obj.set_color(step["rgb"], step.get("alpha"))
            elif step["op"] == "tint":
                obj.pixel_array[..., :3] = obj.pixel_array[..., :3] * np.array(step["rgb"])
        return obj.pixel_array

    cases = []
    for name, steps in definitions:
        p = premultiply(run(canonical, steps))
        files[f"{name}.rgba"] = p.tobytes()
        exported = png_straight(p)
        put_png(f"{name}.png", exported)
        cases.append({"name": name, "steps": steps, "rgba": f"{name}.rgba", "png": f"{name}.png"})

    # 16-bit alpha values below a byte and at the first byte boundary.
    high = [[65535, 1025, 32769, a] for a in [0, 1, 127, 255, 256, 257, 511, 512, 32767, 65535]]
    high_array = (np.array(high, dtype=np.uint16) >> 8).astype(np.uint8).reshape(1, -1, 4)
    high_array[high_array[..., 3] == 0] = 0
    files["subbyte16.rgba"] = premultiply(high_array).tobytes()

    # Pixel-aligned scene reference independent of the Go sampler/rasterizer.
    y, x = np.mgrid[:16, :16]
    scene = np.stack([(x*17), (y*17), ((x+y)*29 % 256), y*16+x], axis=-1).astype(np.uint8)
    scene[scene[..., 3] == 0] = 0
    scene_steps = [{"op": "invert"}, {"op": "tint", "rgb": [.5, 1., .25]}, {"op": "opacity", "factor": .37}]
    files["scene_input.nrgba"] = scene.tobytes()
    scene_pixels = premultiply(run(scene, scene_steps))
    scene_pixels = np.array(Image.fromarray(scene_pixels).transpose(Image.Transpose.ROTATE_90))
    states = []
    for time in [0., .25, .5, .75, 1., 1.5, 2., 2.5, 3.]:
        opacity = time if time <= 1 else (1. if time <= 2 else 3-time)
        shift = min(1., max(0., time-1))
        frame = np.zeros((64, 64, 4), dtype=np.uint8)
        frame[..., 3] = 255
        px, py = 24+int(4*shift), 24+int(2*shift)
        frame[py:py+16, px:px+16, :3] = np.floor(scene_pixels[..., :3].astype(float)*opacity+.5).astype(np.uint8)
        name = f"frame_{time:g}.png"
        put_png(name, frame)
        states.append({"time": time, "png": name, "opacity": opacity, "shift": [4*shift, -2*shift]})

    meta = {
        "source": {"commit": COMMIT, "files_sha256": {p: sha(data) for p, data in blobs.items()},
                   "generator_sha256": sha(Path(__file__).read_bytes()), "numpy": np.__version__, "pillow": pillow_version,
                   "execution": "AST-extracted original ImageMobject set_color/set_opacity, constructor inversion and saved-alpha, color_to_int_rgb/to_int_rgb; minimal state and color constructor"},
        "width": 256, "height": 256, "input": "input.nrgba", "cases": cases,
        "subbyte16": high, "scene": {"width": 16, "height": 16, "input": "scene_input.nrgba", "steps": scene_steps, "states": states},
        "limits": {"max_retained_bytes": 67108864, "max_pixels": 67108864//9},
        "scope": "uint8 pixel byte rules and independent pixel-aligned opaque-background animation frames; tint is a specified extension; hidden alpha0 RGB canonicalized at import; no full Manim camera/resampling or cross-asset morph parity",
        "files_sha256": {name: sha(data) for name, data in sorted(files.items())},
    }
    files["oracle.json"] = (json.dumps(meta, indent=2, sort_keys=True)+"\n").encode()
    return files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, default=ROOT.parent/"manim")
    parser.add_argument("--output", type=Path, default=ROOT/"tests/data/rasterimage/pixels")
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    files = generate(args.source.resolve())
    args.output.mkdir(parents=True, exist_ok=True)
    for name, data in files.items():
        path = args.output/name
        if args.check:
            if path.read_bytes() != data:
                raise SystemExit(f"frozen reference differs: {path}")
        else:
            with path.open("xb") as stream:
                stream.write(data)
    print(f"{'verified' if args.check else 'wrote'} {len(files)} pinned NumPy/Pillow reference files")


if __name__ == "__main__":
    main()
