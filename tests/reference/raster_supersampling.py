#!/usr/bin/env python3
"""Development-only CairoSVG supersampling references, premultiplied box resolve.

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib /usr/local/bin/python3.11 tests/reference/raster_supersampling.py
"""
import base64
import hashlib
import io
import json
from pathlib import Path
import platform

import cairocffi
import cairosvg
from PIL import Image, __version__ as pillow_version

ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'tests/data/render/supersampling'
OUT.mkdir(parents=True,exist_ok=True)

image=Image.new('RGBA',(2,2))
image.putdata([(255,0,0,128),(0,255,0,255),(0,0,255,200),(255,255,0,64)])
buf=io.BytesIO();image.save(buf,format='PNG')
uri='data:image/png;base64,'+base64.b64encode(buf.getvalue()).decode()
ring='M8.2 8.3H86.7V55.4H8.2Z M25.4 22.6V42.7H68.1V22.6Z '
cases={
 'crossing': '<path fill="#4080c0" d="M4 4L5 5L4 5L5 4 M18.2 9.3L42.6 48.1L18.2 48.1L42.6 9.3Z M53.4 11.2L84.1 49.7L84.1 11.2L53.4 49.7Z"/>',
 'holes-duplicate':f'<path fill="#80b030" fill-opacity=".5" fill-rule="nonzero" d="{ring+ring}"/>',
 'mixed':f'''<defs>
 <linearGradient id="g" gradientUnits="userSpaceOnUse" x1="5" y1="0" x2="90" y2="0"><stop offset="0" stop-color="#ff0000" stop-opacity=".4"/><stop offset="1" stop-color="#0000ff" stop-opacity=".8"/></linearGradient>
 <clipPath id="outer"><path d="M5.3 4.6H89.4V58.1H5.3Z"/></clipPath>
 <clipPath id="inner"><path d="M52 10L82 16L77 40L48 34Z"/></clipPath>
 </defs><g clip-path="url(#outer)" opacity=".8">
 <path fill="url(#g)" stroke="white" stroke-width=".7" d="M1 1H94V62H1Z M28 22V44H64V22Z"/>
 <g clip-path="url(#inner)"><image x="50" y="8" width="30" height="30" preserveAspectRatio="none" opacity=".6" href="{uri}"/></g>
 <path fill="none" stroke="#80c040" stroke-width="1.1" stroke-linecap="square" stroke-linejoin="round" stroke-dasharray="4 2" stroke-dashoffset=".5" d="M10 50L24 15L40 49"/>
 </g>'''
}
for name,body in cases.items():
 (OUT/(name+'.svg')).write_text(f'<svg xmlns="http://www.w3.org/2000/svg" width="95" height="63" viewBox="0 0 95 63">{body}</svg>\n')
# Freeze the parent's actual partial formula input. On subsequent regeneration
# this checked-in SVG is sufficient; the artifact is not a test dependency.
formula=OUT/'fractions-035.svg'
if not formula.exists():
 formula.write_bytes((ROOT/'artifacts/fractions-demo-svg-20261104/frame-00.svg').read_bytes())
names=list(cases)+['fractions-035']
for name in names:
 source=(OUT/(name+'.svg')).read_bytes()
 size=(640,360) if name=='fractions-035' else (95,63)
 for scale in [1,2,4,8]:
  png=cairosvg.svg2png(bytestring=source,scale=scale)
  im=Image.open(io.BytesIO(png)).convert('RGBA')
  assert im.size==(size[0]*scale,size[1]*scale)
  # Pillow's RGBa mode stores premultiplied channels, so the box filter
  # averages color and alpha without transparent color bleeding.
  if scale>1: im=im.convert('RGBa').resize(size,Image.Resampling.BOX).convert('RGBA')
  im.save(OUT/f'{name}-cairo-ss{scale}.png')
files=sorted(OUT.glob('*.svg'))+sorted(OUT.glob('*.png'))
p=dict(generator='tests/reference/raster_supersampling.py',generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 python=platform.python_version(),CairoSVG=cairosvg.__version__,Cairo=cairocffi.cairo_version_string(),Pillow=pillow_version,
 quality='CairoSVG renders full vector scenes at scales 1/2/4/8; premultiplied RGBa Pillow BOX downsampling, output dimensions unchanged. PNGs store straight RGBA; tests convert back to premultiplied RGBA.',
 scope='Frozen parent fractions .35 export shares scene construction; independent Cairo interpretation/rasterization only. Odd-size 95x63 handcrafted crossing/hole/duplicate/gradient/stroke/nested clip/bitmap scenes. Does not claim default signed-area AA fixed; native-size refs remain unchanged.',
 files_sha256={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in files})
(OUT/'provenance.json').write_text(json.dumps(p,indent=2)+'\n')
print('wrote independent Cairo references:',names)
