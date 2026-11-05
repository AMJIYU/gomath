"""Development-only actual Manim 0.22.1 numerical and Transform reference.

Run with the already installed pinned Manim environment; no dependency install
is necessary. Output is exclusive: existing fixtures are never overwritten.
Python and subprocess are never used by Go tests or the product runtime.
"""
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys

import manim
import numpy as np
from manim import Transform, VMobject, linear, tempconfig
from manim.utils.bezier import get_smooth_cubic_bezier_handle_points

COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT.parent / "manim"
assert manim.__version__ == "0.22.1"
assert subprocess.check_output(["git", "-C", str(SOURCE), "rev-parse", "HEAD"], text=True).strip() == COMMIT
hashes = {}
for name in ["utils/bezier.py", "utils/paths.py", "mobject/types/vectorized_mobject.py",
             "mobject/mobject.py", "animation/transform.py", "animation/animation.py"]:
    blob = subprocess.check_output(["git", "-C", str(SOURCE), "show", f"{COMMIT}:manim/{name}"])
    assert blob == (Path(manim.__file__).parent / name).read_bytes(), name
    assert blob == (SOURCE / "manim" / name).read_bytes(), name
    hashes["manim/" + name] = hashlib.sha256(blob).hexdigest()

out = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "tests/data/path/curve-edit"
out.mkdir(parents=True, exist_ok=True)


def corners(anchors):
    return VMobject().set_points_as_corners(np.array(anchors, dtype=float))


anchor_inputs = {
    "single": [[2, -3, 4]],
    "two": [[-2, 1, 3], [4, -2, -1]],
    "open_two_curves": [[0, 0, 0], [1, 3, 2], [4, -1, 5]],
    "open_3d": [[-2, 1, -1], [-1, 3, 2], [2, -2, 1], [4, 1, 0], [5, 4, -3]],
    "closed_two_curves": [[0, 0, 0], [3, 2, 1], [0, 0, 0]],
    "closed_triangle": [[0, 0, 0], [3, 1, 2], [1, 4, -2], [0, 0, 0]],
    "closed_square": [[-1, -1, 0], [1, -1, 0], [1, 1, 0], [-1, 1, 0], [-1, -1, 0]],
    "repeated_interior": [[0, 0, 0], [1, 2, 0], [1, 2, 0], [3, -1, 0]],
    "constant": [[2, 3, -4]] * 5,
    "near_closed_origin": [[0, 0, 0], [2, 3, 1], [-1, 2, -1], [5e-9, -5e-9, 5e-9]],
    "relative_closed": [[100, -200, 50], [103, -197, 53], [99, -198, 49], [100.0005, -200.001, 50.0003]],
    "outside_closure": [[0, 0, 0], [2, 3, 1], [-1, 2, -1], [2e-8, 0, 0]],
}
anchor_cases = []
for name, anchors in anchor_inputs.items():
    for mode in ["jagged", "smooth"]:
        a = np.array(anchors, dtype=float)
        obj = corners(anchors)
        if mode == "smooth":
            obj.make_smooth()
            if len(a) > 1:
                h1, h2 = get_smooth_cubic_bezier_handle_points(a)
                assert np.allclose(obj.points[1::4], h1, rtol=0, atol=1e-12)
                assert np.allclose(obj.points[2::4], h2, rtol=0, atol=1e-12)
        anchor_cases.append({"name": name + "_" + mode, "mode": mode,
                             "anchors": anchors, "points": obj.points.tolist()})

base = np.array([
    [0, 0, 0], [0, 3, 1], [2, -1, 2], [2, 1, 0],
    [2, 1, 0], [2.1, 1, 0], [2.2, 1, 0], [2.3, 1, 0],
    [2.3, 1, 0], [4, 6, -2], [-3, -2, 1], [5, 0, 3],
], dtype=float)
insertion_cases = []
for n, targets in [(1, [1, 2, 3, 7, 31]), (2, [2, 3, 5, 8]), (3, [3, 4, 5, 8, 10])]:
    points = base[:4*n]
    for total in targets:
        obj = VMobject().set_points(points.copy()).insert_n_curves(total-n)
        insertion_cases.append({"name": f"curves_{n}_to_{total}", "input": points.tolist(),
                                "total": total, "points": obj.points.tolist()})
for name, points, total in [
    ("ten_to_fifteen", corners([[i, (i*i) % 7, i % 3] for i in range(11)]).points, 15),
    ("closed_to_nine", corners(anchor_inputs["closed_square"]).points, 9),
    ("constant_to_seven", np.tile([2., 3., -1.], (4, 1)), 7),
    ("point_to_five", np.array([[2., -1., 3.]]), 5),
]:
    result = VMobject().insert_n_curves_to_point_list(total-len(points)//4, points)
    insertion_cases.append({"name": name, "input": points.tolist(), "total": total,
                            "points": result.tolist()})

transform_cases = []
with tempconfig({"dry_run": True, "verbosity": "ERROR", "media_dir": str(out / "_media")}):
    pairs = [
        ("unequal_two_five", base[:8], corners([[-3, -2, 0], [-2, 2, 1], [0, -1, 0], [1, 3, 2], [3, 1, -1], [5, 2, 0]]).points),
        ("unequal_three_eight", base, corners([[i-3, (i % 3)-1, (i % 2)*.25] for i in range(9)]).make_smooth().points),
        ("one_to_closed", base[:4], corners(anchor_inputs["closed_square"]).make_smooth().points),
        ("smooth_to_jagged", corners(anchor_inputs["open_3d"]).make_smooth().points, corners(np.array(anchor_inputs["open_3d"])+[1, -2, .5]).points),
    ]
    for name, start, target in pairs:
        a, b = VMobject().set_points(start.copy()), VMobject().set_points(target.copy())
        animation = Transform(a, b, rate_func=linear)
        animation.begin()
        states = []
        for alpha in [0., .125, .375, .75, 1.]:
            animation.interpolate(alpha)
            states.append({"alpha": alpha, "points": a.points.tolist()})
        transform_cases.append({"name": name, "source": start.tolist(), "target": target.tolist(),
                                "total": max(len(start), len(target))//4, "states": states})

manifest = {
    "source": {"version": manim.__version__, "commit": COMMIT, "sha256": hashes,
               "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest()},
    "environment": {"python": platform.python_version(), "numpy": np.__version__},
    "scope": "Actual VMobject insertion and handle construction, and linear Transform geometry at interior times. No raster parity or automatic Go unequal-topology alignment claim. Single anchor is a Go point-contour extension; point-only insertion matches the point-list helper.",
    "geometry_absolute_tolerance": 1e-10,
    "anchors": anchor_cases, "insertions": insertion_cases, "transforms": transform_cases,
}
with (out / "reference.json").open("x") as stream:
    json.dump(manifest, stream, indent=2, allow_nan=False)
    stream.write("\n")
print(f"wrote {len(anchor_cases)} anchor, {len(insertion_cases)} insertion and {len(transform_cases)*5} Transform states")
