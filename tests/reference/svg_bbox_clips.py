#!/usr/bin/env python3
"""Development-only, independently authored direct Cairo bbox-clip references.

Run from any directory with Cairo/CairoSVG/Pillow installed; these dependencies
are not used by Go runtime or tests. CairoSVG's evenodd clip support is disabled,
so it is never used as the evenodd oracle. No production geometry is imported.
"""
import base64
import hashlib
import io
import json
import math
from pathlib import Path
import platform

import cairocffi as cairo
import cairosvg
from PIL import Image, ImageChops, ImageStat, __version__ as pillow_version

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/svg/bbox-clips'
OUT.mkdir(parents=True, exist_ok=True)
W, H = 128, 96
I = (1, 0, 0, 1, 0, 0)


def rect(ctx, box):
    ctx.rectangle(*box)


def ring(ctx):
    ctx.rectangle(.05, .1, .9, .8)
    ctx.rectangle(.3, .3, .4, .4)  # same direction: parity makes a hole


def clip(ctx, box, commands, rule='nonzero', cp=I, child=I):
    old = ctx.get_matrix()
    x, y, w, h = box
    # SVG 2 supplemental bbox transform, BEFORE clipPath and child transform.
    ctx.translate(x, y)
    ctx.scale(w, h)
    ctx.transform(cairo.Matrix(*cp))
    ctx.transform(cairo.Matrix(*child))
    ctx.new_path()
    commands(ctx)
    ctx.set_matrix(old)
    ctx.set_fill_rule(cairo.FILL_RULE_EVEN_ODD if rule == 'evenodd' else cairo.FILL_RULE_WINDING)
    ctx.clip()


def paint_rect(ctx, box, rgba=(.125, .625, .875, 1)):
    ctx.set_source_rgba(*rgba)
    ctx.new_path()
    ctx.rectangle(*box)
    ctx.fill()


def svg(defs, body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96"><defs>{defs}</defs>{body}</svg>\n'


def m(values):
    return 'matrix('+' '.join(format(x, '.17g') for x in values)+')'


sources, painters = {}, {}
box = (5, 7, 24, 18)
a = math.pi/7
matrices = [(math.cos(a), math.sin(a), -math.sin(a), math.cos(a), 16, 0),
            (1, .2, .6, 1, 65, 0), (-1.1, .1, .2, 1.3, 44, 50),
            (1.5, 0, 0, .7, 67, 57)]
cp, child = (1, 0, 0, 1, .03, .05), (1, .05, .1, 1, 0, 0)
sources['affine'] = svg('<clipPath id="c" clipPathUnits="objectBoundingBox" transform="translate(.03 .05)"><rect x=".1" y=".15" width=".75" height=".7" transform="matrix(1 .05 .1 1 0 0)"/></clipPath>', ''.join(f'<rect x="5" y="7" width="24" height="18" fill="#20a0e0" transform="{m(t)}" clip-path="url(#c)"/>' for t in matrices))


def affine(ctx):
    for matrix in matrices:
        ctx.save();ctx.transform(cairo.Matrix(*matrix))
        clip(ctx, box, lambda c: rect(c,(.1,.15,.75,.7)), cp=cp, child=child)
        paint_rect(ctx,box);ctx.restore()


painters['affine'] = affine
sources['use'] = svg('''<style>.clips > clipPath > use {clip-rule:nonzero} #s{clip-rule:inherit}</style>
<path id="s" d="M0 0H.7V1H0Z"/><clipPath id="c" clipPathUnits="objectBoundingBox"><use href="#s"/></clipPath>
<clipPath id="tiny"><rect x="8" y="8" width="24" height="18"/></clipPath>
<rect id="tile" x="4" y="6" width="32" height="24" transform="matrix(1 .2 .4 1 0 0)" clip-path="url(#tiny)"/>''', '<use href="#tile" x="12" y="14" transform="translate(4 3)" fill="#20a0e0" clip-path="url(#c)"/><use href="#tile" x="60" y="46" fill="#20a0e0" clip-path="url(#c)"/>')


def use(ctx):
    for x,y,tx,ty in [(12,14,4,3),(60,46,0,0)]:
        ctx.save();ctx.translate(tx,ty)
        # Tight host-local bbox of target rectangle after shear and use x/y.
        clip(ctx,(x+6.4,y+6.8,41.6,30.4),lambda c:rect(c,(0,0,.7,1)))
        ctx.translate(x,y);ctx.transform(cairo.Matrix(1,.2,.4,1,0,0))
        ctx.rectangle(8,8,24,18);ctx.clip()
        paint_rect(ctx,(4,6,32,24));ctx.restore()


painters['use'] = use
sources['nested'] = svg('''<clipPath id="outer"><rect x="8" y="10" width="109" height="75"/></clipPath>
<clipPath id="half" clipPathUnits="objectBoundingBox"><rect x=".1" y=".15" width=".55" height=".7"/></clipPath>
<clipPath id="c" clipPathUnits="objectBoundingBox" transform="translate(.1 .05)"><rect x=".1" y=".2" width=".8" height=".6" transform="matrix(1 .05 .1 1 0 0)" clip-path="url(#half)"/></clipPath>''', '<g clip-path="url(#outer)" transform="translate(2 3)"><rect x="4" y="6" width="112" height="78" fill="#20a0e0" clip-path="url(#c)" transform="matrix(1 0 -.08 1 3 0)"/></g>')


def nested(ctx):
    ctx.translate(2,3);ctx.rectangle(8,10,109,75);ctx.clip()
    ctx.transform(cairo.Matrix(1,0,-.08,1,3,0))
    clip(ctx,(4,6,112,78),lambda c:rect(c,(.1,.2,.8,.6)),cp=(1,0,0,1,.1,.05),child=(1,.05,.1,1,0,0))
    # Nested child bbox: targetBBox * clipPathTransform * childTransform *
    # childGeometryBBox * nestedClip. This stays in the target's local space.
    old=ctx.get_matrix();ctx.translate(4,6);ctx.scale(112,78);ctx.translate(.1,.05);ctx.transform(cairo.Matrix(1,.05,.1,1,0,0))
    clip(ctx,(.1,.2,.8,.6),lambda c:rect(c,(.1,.15,.55,.7)))
    ctx.set_matrix(old);paint_rect(ctx,(4,6,112,78))


painters['nested'] = nested
sources['gradient'] = svg('''<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="8" y1="8" x2="112" y2="8"><stop stop-color="red" stop-opacity=".7"/><stop offset="1" stop-color="blue" stop-opacity=".7"/></linearGradient>
<clipPath id="c" clipPathUnits="objectBoundingBox"><path d="M.04 .1H.96V.9H.04Z M.3 .3V.7H.7V.3Z"/></clipPath>''','<rect x="8" y="8" width="104" height="72" fill="url(#g)" transform="matrix(1 .03 .05 1 0 0)" clip-path="url(#c)"/>')


def gradient(ctx):
    ctx.transform(cairo.Matrix(1,.03,.05,1,0,0))
    def hole(c):
        c.rectangle(.04,.1,.92,.8)
        c.move_to(.3,.3);c.line_to(.3,.7);c.line_to(.7,.7);c.line_to(.7,.3);c.close_path()
    clip(ctx,(8,8,104,72),hole)
    g=cairo.LinearGradient(8,8,112,8);g.add_color_stop_rgba(0,1,0,0,.7);g.add_color_stop_rgba(1,0,0,1,.7)
    ctx.set_source(g);ctx.rectangle(8,8,104,72);ctx.fill()


painters['gradient'] = gradient
bitmap=io.BytesIO();Image.new('RGBA',(16,8),(48,160,224,180)).save(bitmap,format='PNG')
uri='data:image/png;base64,'+base64.b64encode(bitmap.getvalue()).decode()
sources['bitmap-meet']=svg('<clipPath id="c" clipPathUnits="objectBoundingBox"><rect x=".2" y=".25" width=".6" height=".3"/></clipPath>',f'<image x="10" y="6" width="100" height="80" preserveAspectRatio="xMaxYMid meet" href="{uri}" opacity=".6" clip-path="url(#c)" transform="matrix(-1 .02 .04 1 122 0)"/>')


def bitmap_meet(ctx):
    ctx.transform(cairo.Matrix(-1,.02,.04,1,122,0))
    clip(ctx,(10,6,100,80),lambda c:rect(c,(.2,.25,.6,.3)))
    # meet shrinks 100x80 viewport to 100x50, y=21; chosen clip is entirely
    # inside that asset plane, avoiding differing image-edge filter kernels.
    paint_rect(ctx,(10,21,100,50),(48/255,160/255,224/255,180/255*.6))


painters['bitmap-meet']=bitmap_meet
sources['evenodd']=svg('''<style>.odd{clip-rule:evenodd} #s{clip-rule:inherit}</style>
<path id="s" d="M.05 .1H.95V.9H.05Z M.3 .3H.7V.7H.3Z"/>
<clipPath id="odd" clipPathUnits="objectBoundingBox"><use class="odd" href="#s"/></clipPath>
<clipPath id="nz" clipPathUnits="objectBoundingBox"><use href="#s"/></clipPath>''','<rect x="6" y="12" width="50" height="70" fill="#20a0e0" clip-path="url(#odd)"/><rect x="70" y="12" width="50" height="70" fill="#20a0e0" clip-path="url(#nz)"/>')


def evenodd(ctx):
    for x,rule in [(6,'evenodd'),(70,'nonzero')]:
        ctx.save();clip(ctx,(x,12,50,70),ring,rule);paint_rect(ctx,(x,12,50,70));ctx.restore()


painters['evenodd']=evenodd
sources['degenerate']=svg('<clipPath id="empty" clipPathUnits="objectBoundingBox"/>','<line x1="12" y1="20" x2="112" y2="20" stroke="#20a0e0" stroke-width="8" clip-path="url(#empty)"/><line x1="24" y1="36" x2="24" y2="88" stroke="#20a0e0" stroke-width="8" clip-path="url(#empty)"/>')


def degenerate(ctx):
    # SVG 2 requires the bbox effect to be ignored for zero width or height,
    # regardless of the substantial stroke bbox. Deliberately no ctx.clip().
    ctx.set_source_rgb(32/255,160/255,224/255);ctx.set_line_width(8)
    ctx.move_to(12,20);ctx.line_to(112,20);ctx.stroke()
    ctx.move_to(24,36);ctx.line_to(24,88);ctx.stroke()


painters['degenerate']=degenerate
for name,source in sources.items():
    (OUT/(name+'.svg')).write_text(source)
    for scale in [1,4]:
        surface=cairo.ImageSurface(cairo.FORMAT_ARGB32,W*scale,H*scale);ctx=cairo.Context(surface);ctx.scale(scale,scale)
        painters[name](ctx);out=io.BytesIO();surface.write_to_png(out)
        im=Image.open(io.BytesIO(out.getvalue())).convert('RGBA')
        if scale>1:im=im.convert('RGBa').resize((W,H),Image.Resampling.BOX).convert('RGBA')
        im.save(OUT/f'{name}-cairo-ss{scale}.png')
# Independent oracle guard: only the parity sibling must have a hole.
im=Image.open(OUT/'evenodd-cairo-ss1.png')
assert im.getpixel((31,47))[3]==0 and im.getpixel((95,47))[3]==255
# Secondary checks interpret actual SVG only where this CairoSVG version is
# reliable. Record failures/divergence explicitly; never replace the direct
# parity/degenerate/nested oracle with that parser's unsupported behavior.
probes = {}
for name in ['affine','use','nested','gradient','bitmap-meet','degenerate']:
    try:
        actual = Image.open(io.BytesIO(cairosvg.svg2png(bytestring=sources[name].encode()))).convert('RGBA').convert('RGBa')
        expected = Image.open(OUT/f'{name}-cairo-ss1.png').convert('RGBA').convert('RGBa')
        delta = ImageChops.difference(actual, expected)
        probes[name] = dict(mae=sum(ImageStat.Stat(delta).mean)/4, maximum=max(v[1] for v in delta.getextrema()))
    except cairo.CairoError as error:
        probes[name] = dict(error=str(error))
assert probes['affine']['maximum'] <= 1
assert probes['bitmap-meet']['maximum'] <= 1
assert probes['gradient']['maximum'] <= 8
provenance=dict(generator='tests/reference/svg_bbox_clips.py',generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 versions=dict(Python=platform.python_version(),Cairo=cairo.cairo_version_string(),CairoSVG=cairosvg.__version__,Pillow=pillow_version),
 cairosvg_actual_svg_probes=probes,
 primary_spec=['https://www.w3.org/TR/SVG2/coords.html#BoundingBoxes','https://www.w3.org/TR/SVG2/coords.html#ObjectBoundingBoxUnits'],
 oracle='Directly authored Cairo geometry, affine matrices and explicit winding/parity clips, rendered at 1x/4x with premultiplied BOX resolve; no gomath output or production path code is read.',
 scope='Local geometric bbox excluding stroke, clipPath/child transforms, rotation/shear/reflection/nonuniform, nested child bbox clips, use host/source clips, gradient, meet image viewport, inherited CSS rule and degenerate no-effect lines.',
 limitations='CairoSVG 2.7.1 is not an evenodd clip oracle. Bitmap painted constant RGBA equivalent to authored immutable PNG, chosen clip strictly within meet plane to exclude unrelated filter-edge differences. Curve conversion and coverage use existing importer/renderer tolerances; no multi-shape union or container bbox claim.',
 files_sha256={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in sorted(OUT.iterdir()) if f.suffix in ['.svg','.png']})
(OUT/'provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
print('wrote',len(sources),'independent direct Cairo cases at 1x/4x')
