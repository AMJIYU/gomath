"""Development-only independent SVG paintserver importer references.

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
/usr/local/bin/python3.11 tests/reference/svg_gradients.py tests/data/svg
Append --verify to independently regenerate in memory and compare SHA256.
CairoSVG/Cairo/Pillow/fontTools are development tools, never runtime fallbacks.
No Go executable, generated Go output or Manim source is read or modified.
"""
import base64
import hashlib
import io
import json
from pathlib import Path
import sys

import cairosvg
import cairocffi
from PIL import Image, __version__ as pillow_version

destination = Path(sys.argv[1])
destination.mkdir(parents=True, exist_ok=True)
verify = len(sys.argv) == 3 and sys.argv[2] == "--verify"

redblue = '<stop stop-color="#ee3020"/><stop offset="1" stop-color="#2040ee"/>'
bitmap = io.BytesIO()
Image.new("RGBA", (8, 8), (64, 224, 96, 180)).save(bitmap, format="PNG")
encoded = base64.b64encode(bitmap.getvalue()).decode("ascii")
scenes = {
 "linear": '''<defs transform="translate(500 500)">
 <linearGradient id="g" gradientUnits="userSpaceOnUse" x1="12.5%" y1="50%" x2="87.5%" y2="50%" spreadMethod="pad" color-interpolation="sRGB">
 <stop offset="0" stop-color="#ff2010"/><stop offset=".4" stop-color="#20e050"/>
 <stop offset=".4" stop-color="#2040ef"/><stop offset=".85" stop-color="#ffffff"/>
 </linearGradient></defs><path fill="url(#g)" d="M4 6H124V90H4Z M48 30V66H80V30Z"/>''',
 "bbox-affine": f'''<defs>
 <linearGradient id="g" x1="0" y1="0" x2="1" y2=".3" gradientTransform="matrix(1 .18 .27 .8 -.15 .05)">{redblue}</linearGradient>
 <radialGradient id="r" cx=".45" cy=".5" r=".42" gradientTransform="matrix(1 .2 -.3 1 .1 0)">{redblue}</radialGradient>
 </defs><g transform="translate(5 4)"><rect x="4" y="8" width="48" height="68" fill="url(#g)" transform="matrix(1 .1 .2 1 0 0)"/>
 <rect x="68" y="8" width="42" height="68" fill="url(#r)" transform="matrix(1 -.1 .1 1 0 0)"/></g>''',
 "radial": '''<defs>
 <radialGradient id="g" gradientUnits="userSpaceOnUse" cx="0" cy="0" r="1" fx="0" fy="0" gradientTransform="matrix(43 12 9 30 64 48)">
 <stop stop-color="white"/><stop offset=".5" stop-color="#e03020"/><stop offset="1" stop-color="#2030e0"/>
 </radialGradient></defs><path d="M4 4H124V92H4Z" fill="url(#g)"/>
 ''',
 "alpha": '''<style>.defs > linearGradient {color:#e03010} .defs stop {stop-color:currentColor} #end {stop-color:#2040ee}</style>
 <defs class="defs"><linearGradient id="g" gradientUnits="userSpaceOnUse" x1="16" x2="112">
 <stop stop-opacity="0"/><stop id="end" offset="1" stop-opacity=".85"/>
 </linearGradient></defs><g opacity=".6"><rect x="6" y="8" width="116" height="80" fill="url(#g)" fill-opacity=".7"/></g>''',
 "use-clip-bitmap": f'''<style>.tile {{fill:url(#base);fill-opacity:.7}}</style><defs>
 <linearGradient id="base" x1="0" x2="1">{redblue}</linearGradient>
 <linearGradient id="copy" href="#base" gradientTransform="rotate(10 .5 .5)"/>
 <clipPath id="outer"><path d="M4 4H122V90H4Z M54 34V62H74V34Z"/></clipPath>
 <clipPath id="inner"><rect x="0" y="0" width="49" height="64"/></clipPath>
 <rect id="tile" class="tile" x="-2" y="-2" width="56" height="72"/>
 <image id="im" x="-16" y="-12" width="160" height="120" preserveAspectRatio="none" href="data:image/png;base64,{encoded}"/>
 </defs><g clip-path="url(#outer)"><rect x="8" y="5" width="110" height="5" fill="url(#copy)"/>
 <g transform="translate(9 14)" clip-path="url(#inner)"><use href="#tile"/></g>
 <g transform="translate(70 18)" clip-path="url(#inner)"><use href="#tile"/></g>
 <g opacity=".35"><use href="#im"/></g></g>''',
}
records = {}
for name, body in scenes.items():
    source = ('<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96">' + body + '</svg>').encode()
    raster = cairosvg.svg2png(bytestring=source)
    for suffix, data in [(".svg", source), (".png", raster)]:
        filename = 'gradient-import-' + name + suffix
        if verify:
            assert (destination / filename).read_bytes() == data, filename
        else:
            with (destination / filename).open('xb') as stream:
                stream.write(data)
        records[filename] = hashlib.sha256(data).hexdigest()

provenance = {
 "date": "2026-11-04",
 "generator": "tests/reference/svg_gradients.py",
 "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 "versions": {"Python": sys.version, "CairoSVG": cairosvg.__version__, "Cairo": cairocffi.cairo_version_string(), "Pillow": pillow_version},
 "files_sha256": records,
 "scope": [
  "Five hand-authored, independent transparent 128x96 SVG/PNG pairs; CairoSVG/Cairo rasterizes the source directly. No Go-generated oracle.",
  "Pad linear gradients, normalized viewport percentages, duplicate hard stops, endpoint padding and nonzero compound hole.",
  "Nonuniform objectBoundingBox affine/skew gradients (linear and radial), transformed graphics, userSpace concentric radial.",
  "Definition-tree CSS/currentColor, straight encoded-sRGB/alpha interpolation, stop-opacity * fill-opacity * parent opacity.",
  "Same-type href template reuse, defs/use CSS, nested clipping with hole and semitransparent immutable embedded PNG.",
  "CairoSVG 2.7.1 mutates href-derived gradients when the same template paint is drawn repeatedly: the second tile disappears. This reference draws the inherited gradient once on a separate strip, and uses the base gradient for repeated tile instances; repeated template use is checked analytically in Go. Bitmap edges extend outside all clips to isolate gradient/alpha/clipping from source-edge filtering.",
  "CairoSVG 2.7.1 template inheritance/CSS behavior is not a complete SVG2 shadow-tree oracle; template currentColor and selector-boundary behavior are tested separately with explicit computed-stop assertions.",
  "Text gradients use real existing outlines and are verified analytically; this set does not claim new independent font shaping validation.",
  "CairoSVG gradient rendering does not imply SVG-wide/browser/Pango/Manim equivalence; no full gradient-stroke, focus, spread or linear-light support."
 ],
 "comparison": "Every premultiplied RGBA channel in 0..255 units, with analytical geometry/color checks as well. Thresholds: MAE<=.45, max<=50, fraction pixels with any channel delta>8<=.025.",
 "runtime": "Pure Go. The Python/CairoSVG/Cairo/Pillow generator is development-only; Go tests decode checked-in fixtures."
}
if verify:
    saved = json.loads((destination / 'gradient-import-provenance.json').read_text())
    assert saved['files_sha256'] == records
    assert saved['generator_sha256'] == provenance['generator_sha256']
    print('PASS: regenerated 10 gradient SVG/PNG files, matching recorded SHA256')
else:
    with (destination / 'gradient-import-provenance.json').open('x') as stream:
        json.dump(provenance, stream, indent=2)
        stream.write('\n')
