#!/usr/bin/env python3
"""Development-only partial glyph fill-AA investigation. No production fallback.

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib python3.11 tests/reference/partial_glyph_aa.py \
  artifacts/fractions-demo-svg-20261104/frame-00.svg
"""
import hashlib
import io
import json
import math
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET

import cairocffi
import cairosvg
import pathops
from PIL import Image

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/render/partial-glyph-aa'
SOURCE = Path(sys.argv[1])
root = ET.fromstring(SOURCE.read_bytes())


def flatten(a, b, c, d, output, depth=0):
    dx, dy = d[0]-a[0], d[1]-a[1]
    length = math.hypot(dx, dy)
    err = max(abs((p[0]-a[0])*dy-(p[1]-a[1])*dx)/length for p in [b, c]) if length else max(math.dist(a, p) for p in [b, c])
    if err <= 1e-6 or depth == 24:
        output.append(d)
        return
    mid = lambda p, q: ((p[0]+q[0])/2, (p[1]+q[1])/2)
    ab, bc, cd = mid(a, b), mid(b, c), mid(c, d)
    abc, bcd = mid(ab, bc), mid(bc, cd)
    m = mid(abc, bcd)
    flatten(a, ab, abc, m, output, depth+1)
    flatten(m, bcd, cd, d, output, depth+1)


def clipped_signed_area(points):
    # Sutherland-Hodgman preserves the oriented area of this self-crossing
    # outline clipped to the chosen pixel; it does not take abs per lobe.
    for axis, boundary, sign in [(0, 4, 1), (0, 5, -1), (1, 4, 1), (1, 5, -1)]:
        out = []
        for p, q in zip(points, points[1:]+points[:1]):
            pin, qin = sign*(p[axis]-boundary) >= 0, sign*(q[axis]-boundary) >= 0
            if pin != qin:
                t = (boundary-p[axis])/(q[axis]-p[axis])
                out.append(tuple(p[i]+t*(q[i]-p[i]) for i in range(2)))
            if qin:
                out.append(q)
        points = out
    return sum(p[0]*q[1]-p[1]*q[0] for p, q in zip(points, points[1:]+points[:1]))/2


OUT.mkdir(parents=True, exist_ok=True)
cases = []
for index, pixel in [(70, (293, 70)), (84, (293, 111)), (115, (389, 108))]:
    e = root[index]
    assert e.get('aria-label') == '2' and e.get('stroke-width') == '0' and e.get('stroke-opacity') == '0'
    nums = list(map(float, re.findall(r'[-+]?(?:\d*\.\d+|\d+)(?:[eE][-+]?\d+)?', e.get('d'))))
    xy = [(320+45*x-(pixel[0]-4), 180-45*y-(pixel[1]-4)) for x, y in zip(nums[::2], nums[1::2])]
    p = pathops.Path()
    p.moveTo(*xy[0])
    poly = [xy[0]]
    d = 'M' + ','.join(f'{v:.17g}' for v in xy[0])
    for i in range(1, len(xy), 3):
        p.cubicTo(*xy[i], *xy[i+1], *xy[i+2])
        flatten(poly[-1], *xy[i:i+3], poly)
        d += 'C' + ' '.join(','.join(f'{v:.17g}' for v in q) for q in xy[i:i+3])
    p.close()  # SVG implicitly closes the open contour for fill only.
    cases.append(dict(name=f'partial-2-{index}', d_pixel=d, source_pixel=pixel,
                      path=p, poly=poly, label='2', stroke_width=0))

p = pathops.Path()
p.moveTo(4, 4); p.lineTo(5, 5); p.lineTo(4, 5); p.lineTo(5, 4); p.close()
cases.append(dict(name='one-pixel-bowtie', d_pixel='M4 4L5 5L4 5L5 4', source_pixel=None,
                  path=p, poly=[(4, 4), (5, 5), (4, 5), (5, 4)], label='minimal linear repro', stroke_width=0))
box = pathops.Path()
box.moveTo(4, 4); box.lineTo(5, 4); box.lineTo(5, 5); box.lineTo(4, 5); box.close()
for c in cases:
    c['skia_pixel_area'] = pathops.op(c.pop('path'), box, pathops.PathOp.INTERSECTION).area
    c['signed_pixel_area'] = clipped_signed_area(c.pop('poly'))
    svg = (f'<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16">'
           f'<path fill="white" fill-rule="nonzero" stroke="none" d="{c["d_pixel"]}"/></svg>\n')
    (OUT / (c['name']+'.svg')).write_text(svg)
    for scale in [1, 32]:
        png = cairosvg.svg2png(bytestring=svg.encode(), scale=scale)
        im = Image.open(io.BytesIO(png)).convert('RGBA')
        alpha = sum(v[3] for v in im.crop((4*scale, 4*scale, 5*scale, 5*scale)).getdata())/scale**2
        c[f'cairo_alpha_x{scale}'] = alpha
        if scale == 1:
            (OUT / (c['name']+'.png')).write_bytes(png)
(OUT / 'cases.json').write_text(json.dumps(cases, indent=2)+'\n')
files = [OUT/'cases.json'] + sorted(OUT.glob('*.svg')) + sorted(OUT.glob('*.png'))
p = dict(generator='tests/reference/partial_glyph_aa.py', generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
         source_svg=str(SOURCE), source_sha256=hashlib.sha256(SOURCE.read_bytes()).hexdigest(),
         scene='fractions', time=.35, source_dimensions=[640,360],
         pathops=pathops.__version__, cairosvg=cairosvg.__version__, cairo=cairocffi.cairo_version_string(),
         coordinates='Parent exported cubic coordinates mapped to 16x16 pixel-space crops; selected source pixel is crop (4,4). No glyph, animation or exporter code is used by the reproduction.',
         scope='Fill only, white on transparency, nonzero winding, stroke none. Native low-resolution signed-area mask differs from integration of the nonzero region at a self-crossing within a pixel. 32x area averages and Skia pixel intersection are independent diagnostic evidence, not a threshold relaxation or a renderer fix.',
         files_sha256={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in files})
(OUT/'provenance.json').write_text(json.dumps(p,indent=2)+'\n')
print(json.dumps([{k:v for k,v in c.items() if k!='d_pixel'} for c in cases],indent=2))
