#!/usr/bin/env python3
"""Fresh development-only Cairo check; never rewrites old oracle fixtures.

Run after performance sampling:
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
 /usr/local/bin/python3.11 tests/reference/raster_perf_references.py
"""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import cairocffi
import cairosvg
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT/'tests/data/render/raster_perf'
OUT = DATA/'independent-cairo'
OUT.mkdir(exist_ok=True)


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


# This generator authors the clipped vector/bitmap inputs independently of Go.
clips = OUT/'clips'
if clips.exists():
    raise RuntimeError('use a fresh output directory; do not overwrite evidence')
subprocess.run([sys.executable, str(ROOT/'tests/reference/clipping.py'), str(clips)],
               cwd=ROOT, check=True, env=dict(os.environ, PYTHONDONTWRITEBYTECODE='1'))
records = []
for name in ['clip-nested', 'clip-bitmap', 'linear-hard-hole', 'linear-alpha', 'radial-affine-clip']:
    if name.startswith('clip-'):
        svg, png = clips/(name+'.svg'), clips/(name+'.png')
        previous_svg, previous_png = ROOT/'tests/data/render'/(name+'.svg'), ROOT/'tests/data/render'/(name+'.png')
    else:
        previous_svg, previous_png = ROOT/'tests/data/render/gradients'/(name+'.svg'), ROOT/'tests/data/render/gradients'/(name+'.png')
        svg, png = OUT/(name+'.svg'), OUT/(name+'.png')
        svg.write_bytes(previous_svg.read_bytes())
        png.write_bytes(cairosvg.svg2png(bytestring=svg.read_bytes()))
    new, old = [Image.open(p).convert('RGBA') for p in [png,previous_png]]
    if sha(svg) != sha(previous_svg) or new.size != old.size or new.tobytes() != old.tobytes():
        raise RuntimeError(f'independent reference changed: {name}')
    records.append(dict(name=name, old_svg_sha256=sha(previous_svg), old_png_sha256=sha(previous_png),
                        fresh_svg_sha256=sha(svg), fresh_png_sha256=sha(png),
                        pixels=new.width*new.height, complete_RGBA_equal=True))
record = dict(date='2026-11-05', Cairo=cairocffi.cairo_version_string(), CairoSVG=cairosvg.__version__,
              python=sys.version, generator_sha256=sha(Path(__file__)),
              scope='fresh complete independent vector/bitmap clip and gradient rasters; old fixtures untouched',
              limitation='Cairo comparison uses the existing documented edge tolerances in Go tests; this is not Go/Cairo byte identity',
              records=records)
(DATA/'independent-cairo.json').write_text(json.dumps(record,indent=2)+'\n')
print('PASS: five freshly rendered independent references equal old full RGBA oracles')
