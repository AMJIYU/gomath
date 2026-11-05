"""Development-only pinned Manim reversal, force-direction audit and Transform.

Use an existing installed 0.22.1 environment. No source modification, installs,
reference process or Python dependency is needed by the Go product or tests.
Output is exclusive. Each imported Manim source file is checked against both
the local checkout and its Git blob before any numerical reference is made.
"""
import hashlib
import json
import platform
import subprocess
import sys
from pathlib import Path

import manim
import numpy as np
from manim import ORIGIN, Transform, VMobject, linear

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT.parent / "manim"
COMMIT = "16ea443e2a5f1d79db0a721161411ce916d6e733"
assert manim.__version__ == "0.22.1"
assert subprocess.check_output(["git", "-C", str(SOURCE), "rev-parse", "HEAD"], text=True).strip() == COMMIT
assert not subprocess.check_output(["git", "-C", str(SOURCE), "status", "--porcelain"])

# Check every loaded upstream module, not just the two reversal definitions.
hashes = {}
installed = Path(manim.__file__).resolve().parent
for module in list(sys.modules.values()):
    filename = getattr(module, "__file__", None)
    if not filename:
        continue
    file = Path(filename).resolve()
    if file.suffix != ".py" or not file.is_relative_to(installed):
        continue
    name = "manim/" + str(file.relative_to(installed))
    if name in hashes:
        continue
    blob = subprocess.check_output(["git", "-C", str(SOURCE), "show", f"{COMMIT}:{name}"])
    assert file.read_bytes() == (SOURCE / name).read_bytes() == blob, name
    hashes[name] = hashlib.sha256(blob).hexdigest()
for name in ["manim/mobject/mobject.py", "manim/mobject/types/vectorized_mobject.py",
             "manim/utils/space_ops.py", "manim/animation/transform.py",
             "manim/animation/animation.py", "manim/utils/bezier.py", "manim/utils/paths.py"]:
    assert name in hashes, name


def contour(points, closed=False):
    return {"points": points, "closed": closed}


OPEN = contour([
    [-2, 1, -1], [-3, 4, 2], [1, -2, 3], [2, 1, 0],
    [2, 1, 0], [2.1, 5, -4], [4, -1, 6], [5, 2, 3],
])
CLOSED = contour([
    [8, 0, 1], [10, 4, -2], [11, -3, 3], [12, 2, 1],
    [12, 2, 1], [7, 5, 4], [6, -2, -1], [8, 0, 1],
], True)
LINE = contour([[1, 2, 3], [2, 2, 3], [3, 2, 3], [4, 2, 3]])
CONSTANT = contour([[9, -2, 3]] * 4, True)
POINT = contour([[20, -3, 7]])
NEAR = contour([[0, 0, 0], [1, 3, 1], [-2, 2, -1], [1e-12, -1e-12, 1e-12]])


def object_for(contours):
    points = [point for c in contours for point in c["points"]]
    return VMobject().set_points(np.array(points, dtype=float).reshape((-1, 3)))


def tree_for(nodes):
    objects = [object_for(n["contours"]) for n in nodes]
    for i, node in enumerate(nodes[1:], 1):
        objects[node["parent"]].add(objects[i])
    return objects[0], objects


def affine(obj):
    return obj.scale(1.08, about_point=ORIGIN).rotate(.27, about_point=ORIGIN).shift(np.array([.4, -.3, .1]))


paths = []
for name, contours in [
    ("empty", []), ("point_3d", [POINT]), ("open_3d", [OPEN]),
    ("closed_curved_3d", [CLOSED]), ("constant", [CONSTANT]),
    ("near_closed_open", [NEAR]), ("multi_open_closed", [OPEN, CLOSED]),
    ("multi_unequal", [OPEN, LINE, CLOSED]),
]:
    obj = object_for(contours)
    a, b = obj.copy(), obj.copy()
    assert a.reverse_points() is a and b.reverse_direction() is b
    assert np.array_equal(a.points, b.points)
    states = []
    if contours and all(len(c["points"]) > 1 for c in contours) and len({len(c["points"]) for c in contours}) == 1:
        current = obj.copy()
        target = affine(b.copy())
        animation = Transform(current, target, rate_func=linear)
        animation.begin()
        for alpha in [0., .125, .375, .5, .875, 1.]:
            animation.interpolate(alpha)
            states.append({"alpha": alpha, "points": current.points.tolist()})
    paths.append({"name": name, "contours": contours, "reversed": b.points.tolist(), "states": states})

trees = []
for form, nodes in [
    ("nested_root_empty", [
        {"parent": -1, "contours": []}, {"parent": 0, "contours": [OPEN, CLOSED]},
        {"parent": 0, "contours": []}, {"parent": 2, "contours": [OPEN]},
        {"parent": 2, "contours": [CLOSED]},
    ]),
    ("nested_root_points", [
        {"parent": -1, "contours": [OPEN, CLOSED]}, {"parent": 0, "contours": [CLOSED]},
        {"parent": 0, "contours": []}, {"parent": 2, "contours": [OPEN]},
        {"parent": 2, "contours": [CONSTANT]},
    ]),
    ("nested_point_only", [
        {"parent": -1, "contours": [POINT]}, {"parent": 0, "contours": [OPEN]},
        {"parent": 0, "contours": []}, {"parent": 2, "contours": [POINT]},
    ]),
]:
    for operation in ["reverse_points", "reverse_direction"]:
        obj, objects = tree_for(nodes)
        result = getattr(obj, operation)()
        assert result is obj
        points = [v.points.tolist() for v in objects]
        states = []
        if form != "nested_point_only":
            current, _ = tree_for(nodes)
            target = affine(obj.copy())
            animation = Transform(current, target, rate_func=linear)
            animation.begin()
            for alpha in [0., .125, .375, .5, .875, 1.]:
                animation.interpolate(alpha)
                states.append({"alpha": alpha, "nodes": [v.points.tolist() for v in current.get_family()]})
        trees.append({"name": form + "/" + operation, "operation": operation, "nodes": nodes, "reversed": points, "states": states})

force = []
for name, contours in [("empty", []), ("point", [POINT]), ("line", [LINE]),
                       ("open", [OPEN]), ("closed", [CLOSED]), ("multi", [OPEN, CLOSED])]:
    for direction in ["CW", "CCW", "bad"]:
        obj = object_for(contours)
        before = obj.get_direction()
        try:
            assert obj.force_direction(direction) is obj
            error = ""
        except ValueError as exc:
            error = str(exc)
        force.append({"name": name, "target": direction, "before": before,
                      "after": obj.get_direction(), "error": error, "points": obj.points.tolist()})
# A group has no start anchors. Its force_direction does not orient its children.
group = VMobject().add(object_for([OPEN]), object_for([CLOSED]))
before = [v.points.copy() for v in group.submobjects]
group.force_direction("CW")
assert all(np.array_equal(a, b.points) for a, b in zip(before, group.submobjects))
assert object_for([LINE]).force_direction("CW").get_direction() == "CCW"

# Refuse any lazy-loaded upstream module that escaped the pre-generation check.
for module in list(sys.modules.values()):
    filename = getattr(module, "__file__", None)
    if filename:
        file = Path(filename).resolve()
        if file.suffix == ".py" and file.is_relative_to(installed):
            assert "manim/" + str(file.relative_to(installed)) in hashes, file

out = Path(sys.argv[1]) if len(sys.argv) > 1 else ROOT / "tests/data/path/direction/reference.json"
out.parent.mkdir(parents=True, exist_ok=True)
manifest = {
    "source": {"version": manim.__version__, "commit": COMMIT, "files_sha256": dict(sorted(hashes.items())),
               "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
               "installed_equals_local_equals_git": True},
    "environment": {"python": platform.python_version(), "numpy": np.__version__, "executable": sys.executable},
    "scope": "CPU/Cairo VMobject numerical controls; no raster/OpenGL parity. Authored contour boundaries are authoritative in Go. Force-direction results audit source semantics only; no Go ForceDirection API.",
    "paths": paths, "trees": trees, "force_audit": force,
}
with out.open("x") as stream:
    json.dump(manifest, stream, indent=2, allow_nan=False)
    stream.write("\n")
print(f"verified {len(hashes)} installed==local==git source files at {COMMIT}")
print(f"wrote {len(paths)} paths, {len(trees)} tree operations, {len(force)} force-direction audit probes")
print(f"actual Transform states: path={sum(len(p['states']) for p in paths)}, tree={sum(len(t['states']) for t in trees)}")
