#!/usr/bin/env python3
"""Development-only Skia pathops + CairoSVG Boolean fixtures; never a runtime dependency.

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib /usr/local/bin/python3.11 tests/reference/path_boolean.py
"""
import hashlib
import json
import pathlib
import platform
import subprocess

import cairocffi
import cairosvg
import pathops

ROOT = pathlib.Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/data/path"
MANIM = ROOT.parent / "manim"
SOURCE = MANIM / "manim/mobject/geometry/boolean_ops.py"


def rect(x0, y0, x1, y1):
    return [[x0, y0], [x1, y0], [x1, y1], [x0, y1]]


def outline(contours, rule):
    p = pathops.Path()
    p.fillType = pathops.FillType.EVEN_ODD if rule == "evenodd" else pathops.FillType.WINDING
    for contour in contours:
        p.moveTo(*contour[0])
        for point in contour[1:]:
            p.lineTo(*point)
        p.close()
    return p


def polygons(p):
    result = []
    for verb, args in p:
        if verb == pathops.PathVerb.MOVE:
            result.append([list(args[0])])
        elif verb == pathops.PathVerb.LINE:
            result[-1].append(list(args[0]))
        elif verb != pathops.PathVerb.CLOSE:
            raise ValueError(f"unexpected curved oracle output: {verb}")
    return result


def svg_path(contours):
    return " ".join("M" + " L".join(f"{x:.17g},{y:.17g}" for x, y in c) + " Z" for c in contours)


CASES = [
    ("hole-overlap", [rect(1, 1, 12, 13), rect(4, 4, 9, 10)[::-1]],
     [[[7, 2], [15, 7], [8, 15], [3, 9]]], "nonzero", "nonzero"),
    ("crossing-triangles", [[[0, 0], [10, 7], [4, 4]]],
     [[[2, 5], [11, 0], [5, 9]]], "nonzero", "nonzero"),
    ("collinear-contact", [rect(1, 1, 6, 5)], [rect(4, 1, 10, 3)], "nonzero", "nonzero"),
    ("edge-contact", [rect(1, 1, 6, 5)], [rect(6, 1, 10, 5)], "nonzero", "nonzero"),
    ("point-contact", [rect(1, 1, 6, 5)], [rect(6, 5, 10, 9)], "nonzero", "nonzero"),
    ("same-direction-nesting", [rect(1, 1, 13, 13), rect(4, 4, 10, 10)],
     [rect(6, 2, 15, 8)], "nonzero", "nonzero"),
    ("evenodd-nesting", [rect(1, 1, 13, 13), rect(4, 4, 10, 10)],
     [rect(6, 2, 15, 8)], "evenodd", "nonzero"),
    ("self-crossing", [[[1, 1], [12, 12], [1, 12], [12, 1]]],
     [rect(3, 2, 9, 13)], "evenodd", "nonzero"),
    ("winding-cancellation", [rect(1, 1, 9, 9), rect(1, 1, 9, 9)[::-1]],
     [rect(4, 4, 12, 12)], "nonzero", "nonzero"),
    ("double-evenodd", [rect(1, 1, 9, 9), rect(1, 1, 9, 9)],
     [rect(4, 4, 12, 12)], "evenodd", "evenodd"),
    ("identical-reversed", [rect(.5, 1.5, 10.25, 8.75)],
     [rect(.5, 1.5, 10.25, 8.75)[::-1]], "nonzero", "nonzero"),
    ("empty", [], [rect(1, 1, 9, 9)], "nonzero", "nonzero"),
]
OPS = {"union": pathops.PathOp.UNION, "intersection": pathops.PathOp.INTERSECTION,
       "difference": pathops.PathOp.DIFFERENCE, "xor": pathops.PathOp.XOR}
# Offset grid avoids the edges in these cases; independently query each oracle.
SAMPLES = [[x + .137, y + .331] for y in range(-1, 17) for x in range(-1, 17)]
OUT.mkdir(parents=True, exist_ok=True)
fixtures = []
for name, a, b, ra, rb in CASES:
    pa, pb = outline(a, ra), outline(b, rb)
    entry = dict(name=name, a=a, b=b, rule_a=ra, rule_b=rb, operations={})
    for op, mode in OPS.items():
        result = pathops.op(pa, pb, mode, fix_winding=True, keep_starting_points=True, clockwise=False)
        contours = polygons(result)
        entry["operations"][op] = dict(area=result.area, contours=contours,
                                      membership=[result.contains(tuple(v)) for v in SAMPLES])
        if name == "hole-overlap":
            svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 16 16">'
                   f'<path transform="translate(0 16) scale(1 -1)" fill="#4080c0" fill-rule="nonzero" d="{svg_path(contours)}"/></svg>')
            stem = OUT / f"boolean-{op}"
            stem.with_suffix(".svg").write_text(svg + "\n")
            cairosvg.svg2png(bytestring=svg.encode(), write_to=str(stem.with_suffix(".png")))
    fixtures.append(entry)
(OUT / "boolean-oracle.json").write_text(json.dumps(dict(samples=SAMPLES, cases=fixtures), indent=2) + "\n")
files = [OUT / "boolean-oracle.json"] + sorted(OUT.glob("boolean-*.png")) + sorted(OUT.glob("boolean-*.svg"))
provenance = dict(
    generator="tests/reference/path_boolean.py",
    generator_sha256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),
    source="manim/manim/mobject/geometry/boolean_ops.py",
    source_sha256=hashlib.sha256(SOURCE.read_bytes()).hexdigest(),
    manim_commit=subprocess.check_output(["git", "-C", str(MANIM), "rev-parse", "HEAD"], text=True).strip(),
    python=platform.python_version(), pathops=pathops.__version__, cairosvg=cairosvg.__version__,
    cairo=cairocffi.cairo_version_string(),
    method="Skia pathops filled polygon operations; output polygons rasterized independently by CairoSVG/Cairo, 128x128 straight sRGB fill #4080c0 on transparency, y-up view [-8,8] after translation.",
    scope="Source-pinned Manim backend comparison only, not a Manim animation or Bezier parity claim. Skia uses float32; exact rational/subnormal/rounding failures use separate Go tests. Moderate integer/dyadic fixtures and off-boundary sample grid avoid oracle precision ambiguity.",
    fixtures_sha256={p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in files},
)
(OUT / "boolean-provenance.json").write_text(json.dumps(provenance, indent=2) + "\n")
print(f"wrote {len(fixtures)} cases x 4 ops and 4 CairoSVG frames to {OUT}")
