"""Development-only CairoSVG clip references, independent of GoMath output.

Requires CairoSVG/Pillow only while regenerating. No runtime or Go test imports.
Run with a fresh output prefix/directory; existing references are never replaced.
"""
import base64
import hashlib
import io
import json
import math
from pathlib import Path
import sys

import cairosvg
import cairocffi
from PIL import Image, __version__ as pillow_version

destination = Path(sys.argv[1])
destination.mkdir(parents=True, exist_ok=True)

def circle(radius, reverse=False):
    # Four cubic quarter-circle control polygons, independently evaluated.
    sweep = -math.pi / 2 if reverse else math.pi / 2
    result = [f"M{radius:.17g},0"]
    for i in range(4):
        a, b = i*sweep, (i+1)*sweep
        k = 4/3*math.tan(sweep/4)*radius
        p1 = (radius*math.cos(a)-k*math.sin(a), radius*math.sin(a)+k*math.cos(a))
        p2 = (radius*math.cos(b)+k*math.sin(b), radius*math.sin(b)-k*math.cos(b))
        p3 = (radius*math.cos(b), radius*math.sin(b))
        result.append("C" + " ".join(f"{value:.17g}" for point in [p1,p2,p3] for value in point))
    return " ".join(result)+"Z"

bitmap = io.BytesIO()
Image.new("RGBA", (16,16), (230,120,40,128)).save(bitmap, format="PNG")
encoded = base64.b64encode(bitmap.getvalue()).decode("ascii")
body = {
    "clip-nested": f'''<defs>
      <clipPath id="outer" clipPathUnits="userSpaceOnUse"><path d="M-70 -45L65 -35L55 50L-60 40Z"/></clipPath>
      <clipPath id="ring" clipPathUnits="userSpaceOnUse"><path clip-rule="nonzero" d="{circle(46)} {circle(20,True)}"/></clipPath>
    </defs><g clip-path="url(#outer)"><g clip-path="url(#ring)">
      <rect x="-80" y="-55" width="160" height="110" fill="#40a0f0" opacity=".8"/>
    </g></g>''',
    "clip-bitmap": f'''<defs><clipPath id="circle" clipPathUnits="userSpaceOnUse"><path d="{circle(44)}"/></clipPath></defs>
    <g clip-path="url(#circle)"><image x="-60" y="-60" width="120" height="120" opacity=".6" href="data:image/png;base64,{encoded}"/></g>''',
}
records = {}
for name, content in body.items():
    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" width="192" height="144" viewBox="0 0 192 144">
    <rect width="192" height="144" fill="black"/><g transform="translate(96 72) scale(1 -1)">{content}</g></svg>'''.encode()
    svg_path, png_path = destination/(name+".svg"), destination/(name+".png")
    assert not svg_path.exists() and not png_path.exists()
    svg_path.write_bytes(svg)
    png_path.write_bytes(cairosvg.svg2png(bytestring=svg))
    records[name] = {"svg_sha256":hashlib.sha256(svg).hexdigest(),"png_sha256":hashlib.sha256(png_path.read_bytes()).hexdigest()}
provenance = {"date":"2026-11-04", "versions":{"Python":sys.version,"CairoSVG":cairosvg.__version__,"Cairo":cairocffi.cairo_version_string(),"Pillow":pillow_version},
    "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),"files":records,
    "scope":"Two independent 192x144 opaque-background scenes: nonzero ring/convex nested clip intersection on a vector fill; circular clip on the interior of a constant semitransparent 16x16 bitmap. Canonical geometry and colors defined in this script, no Go-exported oracle. Go test additionally applies/inverts a camera transform. Bitmap boundaries lie outside the clip, excluding the renderers' differing source-edge extension filters.",
    "comparison":"Premultiplied RGBA, all pixels. MAE <= .35/255, max channel delta <= 40, fraction pixels with channel delta > 8 <= .015. These thresholds bound rasterizer edge differences; not pixel identity or full SVG/Manim compatibility.",
    "runtime":"Pure Go; Python, CairoSVG and Cairo are used only to generate independent development references."}
with (destination/"clip-provenance.json").open("x") as f:
    json.dump(provenance,f,indent=2); f.write("\n")
