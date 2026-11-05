"""Development-only independent SVG importer clipping references.

Run: DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
     /usr/local/bin/python3.11 tests/reference/svg_clipping.py OUTPUT_DIRECTORY
Requires CairoSVG/Cairo/Pillow only for regeneration. Does not read Go output.
Existing reference files are never overwritten. No Go test invokes this script.
Append --verify to regenerate in memory and compare the ten fixed files.
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

bitmap = io.BytesIO()
Image.new("RGBA", (16, 16), (224, 112, 32, 180)).save(bitmap, format="PNG")
encoded = base64.b64encode(bitmap.getvalue()).decode("ascii")

scenes = {
    "clip-import-transform": '''<defs transform="translate(800 800)">
      <clipPath id="c" clipPathUnits="userSpaceOnUse" transform="translate(4 3) rotate(11 35 25)">
        <path d="M4 3H68V49H4Z M20 15V35H49V15Z" transform="matrix(1 .1 .18 1 0 0)"/>
      </clipPath></defs>
      <g transform="translate(9 10) scale(1.25 .9)" clip-path="url(#c)">
        <rect x="-10" y="-10" width="110" height="90" fill="#3070df"/>
      </g>''',
    "clip-import-nested": '''<defs>
      <clipPath id="outer"><path d="M12 6L114 17L102 88L4 72Z"/></clipPath>
      <clipPath id="ring" transform="translate(60 48)">
        <path d="M-36 -28H36V28H-36Z M-14 -12V12H14V-12Z"/>
      </clipPath></defs>
      <g clip-path="url(#outer)"><g clip-path="url(#ring)">
        <rect width="128" height="96" fill="#d04090" opacity=".7"/>
      </g></g>''',
    "clip-import-bitmap": f'''<defs>
      <clipPath id="c" transform="translate(64 48) rotate(-17)">
        <path d="M-36 -24H36V24H-36Z M-12 -9V9H12V-9Z"/>
      </clipPath>
      <image id="im" x="-70" y="-55" width="140" height="110"
        href="data:image/png;base64,{encoded}"/>
      </defs><g clip-path="url(#c)" opacity=".6">
        <use href="#im" transform="translate(64 48) matrix(1 .06 .1 1 0 0)"/>
      </g>''',
    "clip-import-use-positions": '''<defs>
      <rect id="outline" x="1" y="2" width="32" height="28" transform="scale(1.2 .9)"/>
      <clipPath id="c" transform="translate(3 4)"><use href="#outline" x="2" y="1" transform="rotate(8)"/></clipPath>
      <rect id="tile" width="60" height="50" clip-path="url(#c)" fill="#30a0e0"/>
      <clipPath id="host"><rect width="60" height="45"/></clipPath></defs>
      <use href="#tile" x="12" y="10" transform="translate(3 4)" clip-path="url(#host)"/>''',
    "clip-import-use": '''<style>.tile {clip-path:url(#c)} .def > clipPath > .outline {clip-rule:nonzero}</style>
      <defs class="def">
        <path id="outline" d="M0 0H32V28H0Z M10 8V20H22V8Z" transform="scale(1.15 .9)"/>
        <clipPath id="c" transform="translate(3 4)"><use class="outline" href="#outline" transform="translate(2 1)"/></clipPath>
        <rect id="tile" class="tile" x="-2" y="-2" width="48" height="40" fill="#10a070"/>
        <clipPath id="host"><rect x="3" y="3" width="90" height="36"/></clipPath>
      </defs>
      <use href="#tile" x="12" y="10" transform="translate(2 4)" clip-path="url(#host)"/>
      <use href="#tile" x="70" y="56" fill="red"/>''',
}

records = {}
for name, body in scenes.items():
    svg = ('''<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96">'''
           + body + "</svg>").encode()
    png = cairosvg.svg2png(bytestring=svg)
    for suffix, data in [(".svg", svg), (".png", png)]:
        if verify:
            assert (destination / (name + suffix)).read_bytes() == data, name + suffix
        else:
            with (destination / (name + suffix)).open("xb") as out:
                out.write(data)
        records[name + suffix] = hashlib.sha256(data).hexdigest()

provenance = {
    "date": "2026-11-04",
    "versions": {"Python": sys.version, "CairoSVG": cairosvg.__version__,
                 "Cairo": cairocffi.cairo_version_string(), "Pillow": pillow_version},
    "generator": "tests/reference/svg_clipping.py",
    "generator_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
    "files_sha256": records,
    "scope": [
        "Five transparent 128x96 independent CairoSVG PNGs; canonical SVG source authored in this script, no Go-exported oracle.",
        "Transformed compound nonzero clip with a hole, affine/reflected transforms tested analytically in Go as well.",
        "Nested graphics clips, constant semitransparent embedded PNG under affine use, CSS plus clipPath/use and clipped use host.",
        "Bitmap image edges lie outside the clip: isolates clip/image alpha from differing source-edge extension filters.",
        "CairoSVG 2.7.1 ignores opacity on leaf use/image and mutates clipPath use x/y after first draw. Bitmap alpha is set on its parent g; repeated clipPath use uses an equivalent translate, and a separate single-use fixture tests x/y. Analytical Go tests cover native instance independence and opacity directly.",
        "CairoSVG nested clipPath-on-clipPath semantics are not used as an oracle; native intersections are checked analytically instead.",
        "No SVG-wide, Pango, Manim animation or full browser compatibility claim."
    ],
    "comparison": "All pixels, premultiplied RGBA channel units 0..255. MAE <= .35, max <= 45, fraction with any channel delta>8 <= .02. Bounds/holes/source placement additionally checked analytically.",
    "runtime": "Pure Go. Python/CairoSVG/Cairo/Pillow only regenerate fixed development references; Go tests read files.",
}
if verify:
    saved = json.loads((destination / "clip-import-provenance.json").read_text())
    assert saved["files_sha256"] == records
    assert saved["generator_sha256"] == provenance["generator_sha256"]
    print("PASS: independently regenerated 10 SVG/PNG files and matched all recorded SHA256 hashes")
else:
    with (destination / "clip-import-provenance.json").open("x") as out:
        json.dump(provenance, out, indent=2)
        out.write("\n")
