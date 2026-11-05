#!/usr/bin/env python3
"""Independent development-only Cairo evenodd clip references.

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
 /usr/local/bin/python3.11 tests/reference/clip_evenodd.py

CairoSVG 2.7.1 has its clip-rule handling commented out in surface.py. It is
recorded as an unsuitable evenodd oracle, never patched or silently trusted.
This script authors the geometry directly and sets Cairo's fill rule explicitly.
SVG sources separately exercise the importer, including CSS/use inheritance.
"""
import base64
import hashlib
import io
import json
import math
from pathlib import Path
import platform
import re

import cairocffi as cairo
import cairosvg
from PIL import Image, __version__ as pillow_version

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/render/clip-evenodd'
OUT.mkdir(parents=True, exist_ok=True)
W, H = 128, 96


def rect(x, y, w, h):
    return [('M', x, y), ('L', x+w, y), ('L', x+w, y+h), ('L', x, y+h), ('Z',)]


def circle(x, y, r):
    k = 4/3*math.tan(math.pi/8)*r
    return [('M', x+r, y), ('C', x+r, y+k, x+k, y+r, x, y+r),
            ('C', x-k, y+r, x-r, y+k, x-r, y),
            ('C', x-r, y-k, x-k, y-r, x, y-r),
            ('C', x+k, y-r, x+r, y-k, x+r, y), ('Z',)]


def d(commands):
    return ' '.join(c[0] + ' '.join(format(x, '.17g') for x in c[1:]) for c in commands)


def clip(commands, rule='evenodd', matrix=(1, 0, 0, 1, 0, 0)):
    return dict(commands=commands, d=d(commands), rule=rule, matrix=matrix)


outer = rect(8.25, 9.5, 110.5, 75.25)
inner = rect(35.25, 29.75, 54.5, 36.5)
ring = circle(64, 48, 39) + circle(64, 48, 17)
bitmap = io.BytesIO()
Image.new('RGBA', (16, 16), (224, 112, 32, 180)).save(bitmap, format='PNG')
uri = 'data:image/png;base64,' + base64.b64encode(bitmap.getvalue()).decode()
items = {
    'compound': [dict(clips=[clip(outer+inner)], color=[64, 160, 240, 255], opacity=.8)],
    'duplicate': [dict(clips=[clip(outer+outer)], color=[255, 255, 255, 255])],
    'curve': [dict(clips=[clip(ring, matrix=(-1, .06, .12, .9, 122, 1))], color=[32, 192, 224, 255])],
    'crossings': [dict(clips=[clip([('M', 4, 4), ('L', 5, 5), ('L', 4, 5), ('L', 5, 4), ('Z',),
      ('M', 18.2, 9.3), ('L', 99.6, 85.1), ('L', 18.2, 85.1), ('L', 99.6, 9.3), ('Z',)]
      + rect(41.25, 25.75, 66.5, 46.25))], color=[64, 128, 192, 255])],
    'thin': [dict(clips=[clip(rect(12, 40+1/256, 100, 1/32))], color=[255, 255, 255, 255])],
    'nested-bitmap': [dict(clips=[clip([('M', 6, 3), ('L', 123, 11), ('L', 117, 90), ('L', 10, 87), ('Z',)], 'nonzero'), clip(ring)], color=[224, 112, 32, 180], opacity=.6, bitmap=True)],
    'gradient': [dict(clips=[clip(outer+inner)], gradient=True)],
    'css-use': [dict(clips=[clip(rect(0, 0, 40, 40)+rect(10, 10, 20, 20), matrix=(1,0,0,1,8,24))], color=[48,160,224,255]),
                dict(clips=[clip(rect(0, 0, 40, 40)+rect(10, 10, 20, 20), 'nonzero', (1,0,0,1,76,24))], color=[48,160,224,255])],
}


def source(name, entries):
    defs, body = [], []
    for i, entry in enumerate(entries):
        for j, c in enumerate(entry['clips']):
            ident = f'c{i}-{j}'
            matrix = ' '.join(format(v, '.17g') for v in c['matrix'])
            defs.append(f'<clipPath id="{ident}" clipPathUnits="userSpaceOnUse"><path clip-rule="{c["rule"]}" transform="matrix({matrix})" d="{c["d"]}"/></clipPath>')
            body.append(f'<g clip-path="url(#{ident})">')
        if entry.get('gradient'):
            defs.append('<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="128" y2="0"><stop offset="0" stop-color="red" stop-opacity=".7"/><stop offset="1" stop-color="blue" stop-opacity=".7"/></linearGradient>')
            body.append('<rect width="128" height="96" fill="url(#g)"/>')
        elif entry.get('bitmap'):
            body.append(f'<image width="128" height="96" preserveAspectRatio="none" opacity="{entry["opacity"]}" href="{uri}"/>')
        else:
            r,g,b,a = entry['color']
            body.append(f'<rect width="128" height="96" fill="#{r:02x}{g:02x}{b:02x}" fill-opacity="{a/255}" opacity="{entry.get("opacity",1)}"/>')
        body.extend('</g>' for _ in entry['clips'])
    if name == 'css-use':
        # These two use elements inherit different rules for the SAME source
        # shape. A cache keyed only by the source element would get this wrong.
        shape = d(rect(0,0,40,40)+rect(10,10,20,20))
        return f'''<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96">
        <style>.clips > clipPath > use {{clip-rule:evenodd}} #source {{clip-rule:inherit}}</style>
        <defs class="clips" clip-rule="nonzero"><path id="source" d="{shape}"/>
        <clipPath id="a"><use href="#source" x="8" y="24"/></clipPath>
        <clipPath id="b"><use href="#source" x="76" y="24" style="clip-rule:initial"/></clipPath></defs>
        <rect width="128" height="96" fill="#30a0e0" clip-path="url(#a)"/>
        <rect width="128" height="96" fill="#30a0e0" clip-path="url(#b)"/></svg>'''
    return '<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96"><defs>'+''.join(defs)+'</defs>'+''.join(body)+'</svg>'


def cairo_render(entries, scale):
    surface = cairo.ImageSurface(cairo.FORMAT_ARGB32, W*scale, H*scale)
    ctx = cairo.Context(surface)
    ctx.scale(scale,scale)
    for entry in entries:
        ctx.save()
        for c in entry['clips']:
            old = ctx.get_matrix()
            ctx.transform(cairo.Matrix(*c['matrix']))
            ctx.new_path()
            for command in c['commands']:
                op, args = command[0], command[1:]
                {'M':ctx.move_to, 'L':ctx.line_to, 'C':ctx.curve_to, 'Z':ctx.close_path}[op](*args)
            ctx.set_matrix(old)
            ctx.set_fill_rule(cairo.FILL_RULE_EVEN_ODD if c['rule']=='evenodd' else cairo.FILL_RULE_WINDING)
            ctx.clip()
        if entry.get('gradient'):
            paint=cairo.LinearGradient(0,0,128,0)
            paint.add_color_stop_rgba(0,1,0,0,.7)
            paint.add_color_stop_rgba(1,0,0,1,.7)
            ctx.set_source(paint)
        else:
            r,g,b,a=entry['color']
            ctx.set_source_rgba(r/255,g/255,b/255,a/255*entry.get('opacity',1))
        ctx.paint()
        ctx.restore()
    output=io.BytesIO();surface.write_to_png(output)
    im=Image.open(io.BytesIO(output.getvalue())).convert('RGBA')
    if scale>1:
        im=im.convert('RGBa').resize((W,H),Image.Resampling.BOX).convert('RGBA')
    return im


for name,entries in items.items():
    (OUT/(name+'.svg')).write_text(source(name,entries)+'\n')
    for scale in [1,4]:
        cairo_render(entries,scale).save(OUT/f'{name}-cairo-ss{scale}.png')
# Confirm that the chosen oracle actually distinguishes same-winding holes.
assert cairo_render(items['compound'],1).getpixel((64,48))[3] == 0
assert cairo_render(items['duplicate'],1).getbbox() is None
assert cairo_render(items['thin'],1).getpixel((20,40))[3] == 8
cairo_svg_probe = Image.open(io.BytesIO(cairosvg.svg2png(bytestring=source('compound',items['compound']).encode())))
source_file=Path(cairosvg.__file__).parent/'surface.py'
source_text=source_file.read_text()
assert "# if node.get('clip-rule') == 'evenodd':" in source_text
assert cairo_svg_probe.getpixel((64,48))[3] != 0
metadata={name:[{key:value for key,value in entry.items() if key!='clips'}|{'clips':[{key:value for key,value in c.items() if key!='commands'} for c in entry['clips']]} for entry in entries] for name,entries in items.items()}
(OUT/'cases.json').write_text(json.dumps(metadata,indent=2)+'\n')
files=sorted(OUT.glob('*.svg'))+sorted(OUT.glob('*.png'))+[OUT/'cases.json']
provenance=dict(generator='tests/reference/clip_evenodd.py',generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 versions=dict(Python=platform.python_version(),Cairo=cairo.cairo_version_string(),CairoSVG=cairosvg.__version__,Pillow=pillow_version),
 oracle='Independent, directly authored Cairo paths and FILL_RULE_EVEN_ODD/FILL_RULE_WINDING, native and 4x premultiplied BOX. SVG sources exercise importer semantics but are not interpreted by CairoSVG for the oracle.',
 cairo_svg_limitation=dict(source_sha256=hashlib.sha256(source_file.read_bytes()).hexdigest(),commented_evenodd_clip=True,compound_hole_wrong_alpha=cairo_svg_probe.getpixel((64,48))[3]),
 bitmap='Cairo paints a constant color equivalent to the independently authored solid-alpha 16x16 PNG; all image edges are outside the visible clip, excluding edge-filter differences.',
 scope='Single compound clip shapes; same-orientation holes, self-crossings, coincident duplicate contours, thin bands, affine/reflection, nested mixed rules, bitmap, gradient and CSS use contexts. No multi-shape clip union, objectBoundingBox, filters, or general fill-rule implementation.',
 files_sha256={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in files})
(OUT/'provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
print('wrote independent Cairo evenodd references:',', '.join(items))
