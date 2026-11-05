#!/usr/bin/env python3
"""Independent authored Cairo paths for ordinary parity fills (development only).

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
 /usr/local/bin/python3.11 tests/reference/fill_evenodd.py

No native/runtime dependency is added. No Go renderer or exported path is read.
Partial curves below are independently split with de Casteljau. SVG inputs
separately exercise the importer; direct Cairo is the pixel oracle because the
installed CairoSVG does not implement evenodd clip paths.
"""
import hashlib
import io
import json
import math
from pathlib import Path
import platform

import cairocffi as cairo
from PIL import Image, __version__ as pillow_version

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/render/fill-evenodd'
OUT.mkdir(parents=True, exist_ok=True)
W, H = 128, 96
IDENTITY = [1, 0, 0, 1, 0, 0]


def rect(x, y, w, h):
    return [('M', x, y), ('L', x+w, y), ('L', x+w, y+h),
            ('L', x, y+h), ('L', x, y), ('Z',)]


def circle(x, y, r):
    k = 4/3*math.tan(math.pi/8)*r
    return [('M', x+r, y), ('C', x+r, y+k, x+k, y+r, x, y+r),
            ('C', x-k, y+r, x-r, y+k, x-r, y),
            ('C', x-r, y-k, x-k, y-r, x, y-r),
            ('C', x+k, y-r, x+r, y-k, x+r, y), ('Z',)]


def d(commands):
    return ' '.join(c[0]+' '.join(format(x, '.17g') for x in c[1:]) for c in commands)


def prefix(commands, fraction):
    if fraction == 1:
        return commands
    # All authored paths explicitly return to their start before Z. Thus the
    # cubic-parameter prefix counts exactly L/C segments, not Z commands.
    remaining = sum(c[0] in ['L', 'C'] for c in commands)*fraction
    result, current = [], None
    for command in commands:
        op, v = command[0], command[1:]
        if remaining <= 0:
            break
        if op == 'M':
            current = v
            result.append(command)
        elif op == 'Z':
            result.append(command)
        elif remaining >= 1:
            result.append(command)
            current = v[-2:]
            remaining -= 1
        else:
            def lerp(a, b):
                return tuple(x+(y-x)*remaining for x, y in zip(a, b))
            if op == 'L':
                result.append(('L', *lerp(current, v)))
            else:
                a, b, c = lerp(current, v[:2]), lerp(v[:2], v[2:4]), lerp(v[2:4], v[4:])
                e, f = lerp(a, b), lerp(b, c)
                result.append(('C', *a, *e, *lerp(e, f)))
            remaining = 0
    return result


outer, inner = rect(8.25, 9.5, 110.5, 75.25), rect(35.25, 29.75, 54.5, 36.5)
ring = circle(64, 48, 39)+circle(64, 48, 17)
crossing = [('M', 4, 4), ('L', 5, 5), ('L', 4, 5), ('L', 5, 4), ('L', 4, 4), ('Z',),
            ('M', 18.2, 9.3), ('L', 99.6, 85.1), ('L', 18.2, 85.1), ('L', 99.6, 9.3), ('L', 18.2, 9.3), ('Z',)]


def graphic(commands, **extra):
    return dict(commands=commands, d=d(commands), rule='evenodd', color=[48, 160, 224, 255],
                opacity=1, reveal=1, matrix=IDENTITY, **extra)


items = {
    'compound': [graphic(outer+inner)],
    'duplicate': [graphic(outer+outer)],
    'triple': [graphic(outer+outer+outer)],
    'curve': [graphic(ring)],
    'crossings': [graphic(crossing+rect(41.25, 25.75, 66.5, 46.25))],
    'thin': [graphic(rect(12, 40+1/256, 100, 1/32))],
    'linear': [graphic(ring, gradient='linear')],
    'radial': [graphic(outer+inner, gradient='radial')],
    'partial': [graphic(ring)],
    'nested-clip': [graphic(ring, clips=[
        dict(d=d(rect(3.5, 10.25, 120, 74)), commands=rect(3.5, 10.25, 120, 74), rule='nonzero'),
        dict(d=d(rect(0, 0, 128, 96)+rect(60.5, 25, 6.25, 45)),
             commands=rect(0, 0, 128, 96)+rect(60.5, 25, 6.25, 45), rule='evenodd')])],
    'stroke-dash': [graphic(outer+inner, stroke=[224, 64, 32, 255], width=2.5, dash=[9, 4], offset=2)],
    'css-use': [graphic(rect(0, 0, 40, 40)+rect(10, 10, 20, 20)),
                graphic(rect(0, 0, 40, 40)+rect(10, 10, 20, 20))],
}
items['compound'][0]['opacity'] = .8
items['curve'][0]['matrix'] = [-1, .06, .12, .9, 122, 1]
items['partial'][0]['reveal'] = .72
items['nested-clip'][0]['opacity'] = .6
items['css-use'][0]['matrix'] = [1, 0, 0, 1, 8, 24]
items['css-use'][1]['matrix'] = [1, 0, 0, 1, 76, 24]
items['css-use'][1]['rule'] = 'nonzero'


def outline(ctx, commands):
    ctx.new_path()
    for command in commands:
        {'M': ctx.move_to, 'L': ctx.line_to, 'C': ctx.curve_to, 'Z': ctx.close_path}[command[0]](*command[1:])


def set_rule(ctx, rule):
    ctx.set_fill_rule(cairo.FILL_RULE_EVEN_ODD if rule == 'evenodd' else cairo.FILL_RULE_WINDING)


def render(entries, scale):
    surface = cairo.ImageSurface(cairo.FORMAT_ARGB32, W*scale, H*scale)
    ctx = cairo.Context(surface)
    ctx.scale(scale, scale)
    for e in entries:
        ctx.save()
        for c in e.get('clips', []):
            outline(ctx, c['commands']); set_rule(ctx, c['rule']); ctx.clip()
        ctx.transform(cairo.Matrix(*e['matrix']))
        outline(ctx, prefix(e['commands'], e['reveal']))
        set_rule(ctx, e['rule'])
        if e.get('gradient'):
            if e['gradient'] == 'linear':
                paint = cairo.LinearGradient(0, 0, 128, 0)
            else:
                paint = cairo.RadialGradient(64, 48, 0, 64, 48, 55)
            paint.add_color_stop_rgba(0, 1, 0, 0, .7)
            paint.add_color_stop_rgba(1, 0, 0, 1, .7)
            paint.set_extend(cairo.EXTEND_PAD)
            ctx.set_source(paint)
        else:
            r, g, b, a = e['color']
            ctx.set_source_rgba(r/255, g/255, b/255, a/255*e['opacity'])
        ctx.fill_preserve()
        if e.get('stroke'):
            # Cairo stroking is independent of fill-rule; choose explicit SVG
            # butt/miter/4 values to match the native construction below.
            r, g, b, a = e['stroke']
            ctx.set_source_rgba(r/255, g/255, b/255, a/255*e['opacity'])
            ctx.set_line_width(e['width']); ctx.set_line_cap(cairo.LINE_CAP_BUTT)
            ctx.set_line_join(cairo.LINE_JOIN_MITER); ctx.set_miter_limit(4)
            ctx.set_dash(e.get('dash', []), e.get('offset', 0)); ctx.stroke()
        ctx.restore()
    output = io.BytesIO(); surface.write_to_png(output)
    im = Image.open(io.BytesIO(output.getvalue())).convert('RGBA')
    if scale > 1:
        im = im.convert('RGBa').resize((W, H), Image.Resampling.BOX).convert('RGBA')
    return im


def source(name, entries):
    if name == 'css-use':
        shape = entries[0]['d']
        return f'''<svg viewBox="0 0 128 96"><style>.odd {{fill-rule:evenodd}} #source {{fill-rule:inherit}}</style>
        <defs fill-rule="nonzero"><path id="source" d="{shape}"/></defs>
        <use class="odd" href="#source" x="8" y="24" fill="#30a0e0"/>
        <use style="fill-rule:initial" href="#source" x="76" y="24" fill="#30a0e0"/></svg>'''
    defs, body = [], []
    for i, e in enumerate(entries):
        for j, c in enumerate(e.get('clips', [])):
            ident = f'c{i}-{j}'
            defs.append(f'<clipPath id="{ident}"><path clip-rule="{c["rule"]}" d="{c["d"]}"/></clipPath>')
            body.append(f'<g clip-path="url(#{ident})">')
        matrix = ' '.join(format(v, '.17g') for v in e['matrix'])
        fill = '#%02x%02x%02x' % tuple(e['color'][:3])
        if e.get('gradient'):
            tag = e['gradient']+'Gradient'
            attrs = 'x1="0" y1="0" x2="128" y2="0"' if e['gradient']=='linear' else 'cx="64" cy="48" r="55"'
            defs.append(f'<{tag} id="g{i}" gradientUnits="userSpaceOnUse" {attrs}><stop offset="0" stop-color="red" stop-opacity=".7"/><stop offset="1" stop-color="blue" stop-opacity=".7"/></{tag}>')
            fill = f'url(#g{i})'
        stroke = ''
        if e.get('stroke'):
            color = '#%02x%02x%02x' % tuple(e['stroke'][:3])
            dash = ' '.join(str(x) for x in e.get('dash', []))
            stroke = f' stroke="{color}" stroke-width="{e["width"]}" stroke-linecap="butt" stroke-linejoin="miter" stroke-dasharray="{dash}" stroke-dashoffset="{e["offset"]}"'
        body.append(f'<path d="{d(prefix(e["commands"],e["reveal"]))}" transform="matrix({matrix})" fill="{fill}" fill-rule="{e["rule"]}" opacity="{e["opacity"]}"{stroke}/>')
        body.extend('</g>' for _ in e.get('clips', []))
    return '<svg viewBox="0 0 128 96"><defs>'+''.join(defs)+'</defs>'+''.join(body)+'</svg>'


for name, entries in items.items():
    (OUT/(name+'.svg')).write_text(source(name, entries)+'\n')
    for scale in [1, 4]:
        render(entries, scale).save(OUT/f'{name}-cairo-ss{scale}.png')
assert render(items['compound'], 1).getpixel((64, 48))[3] == 0
assert render(items['duplicate'], 1).getbbox() is None
assert render(items['thin'], 1).getpixel((20, 40))[3] == 8
metadata = {name: [{k: v for k, v in e.items() if k != 'commands' and k != 'clips'} |
                   {'clips': [{k: v for k, v in c.items() if k != 'commands'} for c in e.get('clips', [])]}
                   for e in entries] for name, entries in items.items()}
(OUT/'cases.json').write_text(json.dumps(metadata, indent=2)+'\n')
files = sorted(OUT.glob('*.svg'))+sorted(OUT.glob('*.png'))+[OUT/'cases.json']
provenance = dict(generator='tests/reference/fill_evenodd.py',
                  generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                  versions=dict(Python=platform.python_version(), Cairo=cairo.cairo_version_string(), Pillow=pillow_version),
                  oracle='Direct independently authored Cairo paths, explicit parity/winding fill and clip rules; 4x premultiplied RGBA BOX resolve. No Go output is consumed.',
                  geometry='Explicit line vertices and cubic circle controls; independent de Casteljau ordered cubic-parameter prefixes. Not a Manim/Pango parity claim.',
                  scope='Ordinary solid/linear/radial fills, opacity, same-winding holes, duplicates, affine curved paths, selfcrossings, thin bands, partial cubics, mixed ancestor clips, dashed stroke, CSS/use. No group isolation, bbox clips or general SVG oracle.',
                  files_sha256={p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in files})
(OUT/'provenance.json').write_text(json.dumps(provenance, indent=2)+'\n')
print('wrote direct Cairo ordinary fill references:', ', '.join(items))
