"""Development-only independent Manim geometry and Cairo frames; exclusive output.

uv run --quiet --isolated --no-project --with ../manim python \
    tests/reference/transform_paths.py tests/data/root/transform-paths
Output directory may exist, but reference files must not: old fixtures are frozen.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import manim
import numpy as np
from manim import (Camera, ClockwiseTransform, CounterclockwiseTransform,
                   ReplacementTransform, Scene, Transform, TransformFromCopy,
                   VMobject, linear, tempconfig)
from PIL import Image

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
source = Path(__file__).resolve().parents[3] / "manim"
assert subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert manim.__version__ == "0.22.1"
source_hashes = {}
for name in ["utils/paths.py", "utils/space_ops.py", "animation/transform.py",
             "animation/animation.py", "animation/composition.py", "mobject/mobject.py",
             "mobject/types/vectorized_mobject.py", "camera/camera.py"]:
    blob = subprocess.check_output(["git", "-C", str(source), "show", f"{COMMIT}:manim/{name}"])
    assert blob == (Path(manim.__file__).parent / name).read_bytes(), name
    source_hashes["manim/" + name] = hashlib.sha256(blob).hexdigest()

out = Path(sys.argv[1])
out.mkdir(parents=True, exist_ok=True)


def rectangle(w, h, x, y, z=0):
    corners = np.array([[-w/2+x, -h/2+y, z], [w/2+x, -h/2+y, z],
                        [w/2+x, h/2+y, z], [-w/2+x, h/2+y, z]])
    points = []
    for i in range(4):
        a, b = corners[i], corners[(i+1) % 4]
        points.extend([a, a+(b-a)/3, a+2*(b-a)/3, b])
    return VMobject().set_points(np.array(points)).set_fill("#FFFFFF", opacity=1).set_stroke(width=0)


def capture(kind, alpha, arc, axis):
    a = rectangle(2, 1, -2, 0)
    b = rectangle(1, 2, 2, .5)
    # Oblique cases deliberately include axial displacement: upstream's path
    # preserves its starting axial coordinate even at alpha=1.
    rate = (lambda t: t*t) if kind == "copy_square_rate" else linear
    if kind == "overshoot":
        rate = lambda t: 1.5*t
    cls = {"replacement": ReplacementTransform, "copy": TransformFromCopy,
           "copy_square_rate": TransformFromCopy, "clockwise": ClockwiseTransform,
           "counterclockwise": CounterclockwiseTransform}.get(kind, Transform)
    options = {"rate_func": rate, "path_arc_axis": np.array(axis, dtype=float)}
    if kind not in ["clockwise", "counterclockwise"]:
        options["path_arc"] = arc
    anim = cls(a, b, **options)
    anim.begin()
    anim.interpolate(alpha)
    obj = b if kind.startswith("copy") else a
    result = {"kind": kind, "alpha": alpha, "arc": arc, "axis": axis,
              "points": obj.points.tolist()}
    if kind in ["arc", "copy_square_rate", "clockwise", "replacement"] and alpha in [.25, .5, .75]:
        camera = Camera(pixel_width=128, pixel_height=96, frame_width=10, frame_height=7.5,
                        background_color="#000000")
        camera.capture_mobjects([obj])
        filename = f"{kind}-{alpha}.png"
        destination = out / filename
        assert not destination.exists()
        Image.fromarray(camera.pixel_array).save(destination)
        result["frame"] = filename
        result["frame_sha256"] = hashlib.sha256(destination.read_bytes()).hexdigest()
    return result


records = []
with tempconfig({"dry_run": True, "verbosity": "ERROR", "media_dir": str(out / "_media")}):
    for kind, arc, axis in [
        ("straight", 0., [0, 0, 1]), ("tiny", .009, [0, 0, 1]),
        ("threshold", .01, [0, 0, 1]), ("arc", np.pi/2, [0, 0, 1]),
        ("negative", -2.2, [0, 0, 3]), ("large", 4.5, [0, 0, 1]),
        ("oblique", 1.7, [1, 2, 3]), ("zero_axis", 1.7, [0, 0, 0]),
        ("replacement", 1.4, [0, 0, 1]), ("copy", 1.4, [0, 0, 1]),
        ("copy_oblique", 1.4, [1, 2, 3]), ("copy_square_rate", 1.4, [0, 0, 1]),
        ("clockwise", -np.pi, [0, 0, 1]), ("counterclockwise", np.pi, [0, 0, 1]),
        ("overshoot", 1.4, [0, 0, 1]),
    ]:
        for alpha in [0., .125, .25, .5, .75, .875, 1.]:
            # copy_oblique needs the reversed source implementation too.
            if kind == "copy_oblique":
                item = capture("copy", alpha, arc, axis)
                item["kind"] = kind
            else:
                item = capture(kind, alpha, arc, axis)
            records.append(item)

manifest = {
    "source": {"version": manim.__version__, "commit": COMMIT,
               "sha256": source_hashes,
               "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
    "api": ["Animation.WithPathArc(float64)", "Animation.WithPathArcAxis(geom.Vec3)",
            "ClockwiseTransform(*Object,*Object)", "CounterclockwiseTransform(*Object,*Object)"],
    "bounds": {"abs_arc_max": "2*pi; abs(sin(arc/2)) >= 1e-6 for nonstraight arcs",
               "straight": "abs(arc) < 0.01", "axis": "finite, robust normalization; zero -> OUT",
               "endpoint_coordinates": 1e9, "points_in_both_trees": 1048576,
               "alignment_work": 16777216, "abs_rate_output": 1000000,
               "clips": "planar z=0 outlines, OUT/IN axis; corresponding-point arc interpolation",
               "gradients": "OUT/IN axis, matching kind/stops and identical 2x2 basis; origins follow point arc",
               "bitmaps": "matching assets and affine image planes, validated at every sampled frame",
               "modes": "Transform/ReplacementTransform/TransformFromCopy leaves only"},
    "checks": {"geometry_states": len(records), "independent_cairo_frames": sum("frame" in r for r in records),
               "required": ["focused geometry and Cairo mask agreement", "atomic errors", "identity/cleanup",
                            "windows/groups/Succession", "reflected copy clock and reversed point path",
                            "clip/gradient frame semantics", "concurrent scene and frozen replay snapshots",
                            "noasm central suite", "noasm vet", "purecheck", "focused race", "benchmem"]},
    "limits": "Equal-topology rectangles establish pinned point paths and selected interior frames. Group, window, cleanup, clip, gradient and replay contracts additionally use Go invariant tests. No arbitrary path_func/path_arc_centers, multi-turn paths, gradient basis morphing, tilted paint or comprehensive Manim parity claim. Axial displacement retains upstream starting-axis coordinate, including at final Transform state; ReplacementTransform cleanup installs actual target.",
    "cases": records,
}
with (out / "manifest.json").open("x") as stream:
    json.dump(manifest, stream, indent=2)
    stream.write("\n")
