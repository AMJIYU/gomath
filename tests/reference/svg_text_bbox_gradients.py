#!/usr/bin/env python3
"""Development-only fontTools + direct Cairo text gradient oracle.

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
/usr/local/bin/python3 tests/reference/svg_text_bbox_gradients.py \
  --fonts /Users/eleven/go/pkg/mod/golang.org/x/image@v0.47.0/font/gofont/ttfs
Append --verify to regenerate in memory and compare all fixture hashes.
No Go source, Go exports, Go geometry or Go pixels are consumed.
"""
import argparse
import hashlib
import io
import json
import math
from pathlib import Path
import platform

import cairocffi as cairo
import fontTools
from fontTools.pens.basePen import BasePen
from fontTools.ttLib import TTFont
from PIL import Image, __version__ as pillow_version

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/svg/text-bbox-gradients'
IDENTITY = [1, 0, 0, 1, 0, 0]
NORMALIZE = [1, 0, 0, -1, -64, 48]
RED, BLUE, GREEN = [1, 0, 0], [0, 0, 1], [32/255, 160/255, 96/255]
STOPS = '<stop stop-color="red"/><stop offset="1" stop-color="blue"/>'
DEFS = '<linearGradient id="g">'+STOPS+'</linearGradient><radialGradient id="r">'+STOPS+'</radialGradient>'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def mul(a, b):
    return [a[0]*b[0]+a[2]*b[1], a[1]*b[0]+a[3]*b[1],
            a[0]*b[2]+a[2]*b[3], a[1]*b[2]+a[3]*b[3],
            a[0]*b[4]+a[2]*b[5]+a[4], a[1]*b[4]+a[3]*b[5]+a[5]]


def union(cells):
    x, y = min(c[0] for c in cells), min(c[1] for c in cells)
    return [x, y, max(c[0]+c[2] for c in cells)-x, max(c[1]+c[3] for c in cells)-y]


class CairoPen(BasePen):
    def __init__(self, glyphset, ctx):
        super().__init__(glyphset)
        self.ctx = ctx

    def _moveTo(self, p): self.ctx.move_to(*p)
    def _lineTo(self, p): self.ctx.line_to(*p)
    def _curveToOne(self, a, b, c): self.ctx.curve_to(*a, *b, *c)
    def _qCurveToOne(self, a, b):
        p = self._getCurrentPoint()
        self.ctx.curve_to(p[0]+2*(a[0]-p[0])/3, p[1]+2*(a[1]-p[1])/3,
                          b[0]+2*(a[0]-b[0])/3, b[1]+2*(a[1]-b[1])/3, *b)
    def _closePath(self): self.ctx.close_path()
    def _endPath(self): pass


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--fonts', type=Path, required=True)
    ap.add_argument('--verify', action='store_true')
    args = ap.parse_args()
    regular_path = args.fonts / 'Go-Regular.ttf'
    regular_bytes = regular_path.read_bytes()
    assert sha(regular_bytes) == '197d9f3703b4c00af609178876a8d73e396f64fe438b2c871778566632374be3'
    kern_bytes = (ROOT / 'tests/data/svg/text-kern.ttf').read_bytes()
    fonts = {'regular': TTFont(io.BytesIO(regular_bytes)), 'kern': TTFont(io.BytesIO(kern_bytes))}
    # Preserve original outlines/metrics except the A advance; this produces
    # actual ink with an exactly zero-width glyph-cell bbox.
    zero = TTFont(io.BytesIO(regular_bytes), recalcTimestamp=False)
    aname = zero.getBestCmap()[ord('A')]
    zero['hmtx'].metrics[aname] = (0, zero['hmtx'].metrics[aname][1])
    buffer = io.BytesIO()
    zero.save(buffer)
    zero_bytes = buffer.getvalue()
    fonts['zero'] = TTFont(io.BytesIO(zero_bytes))
    files = {'zero-advance.ttf': zero_bytes}

    def run(value, x, y, size, anchor='start', font='regular'):
        f = fonts[font]
        cmap, scale = f.getBestCmap(), size/f['head'].unitsPerEm
        ascent, descent = f['hhea'].ascent*scale, -f['hhea'].descent*scale
        glyphs, cursor, previous = [], 0., None
        for c in value:
            name = cmap[ord(c)]
            if previous and 'kern' in f:
                cursor += f['kern'].kernTables[0].kernTable.get((previous, name), 0)*scale
            width = f['hmtx'][name][0]*scale
            glyphs.append(dict(rune=c, x=cursor, y=y, size=size, font=font, width=width,
                               ascent=ascent, descent=descent, fill='g', alpha=1.))
            cursor += width
            previous = name
        offset = {'start': 0, 'middle': -cursor/2, 'end': -cursor}[anchor]
        for g in glyphs:
            g['x'] += x+offset
            g['cell'] = [g['x'], y-ascent, g['width'], ascent+descent]
        return glyphs

    def group(glyphs, matrix=None, gradients=None):
        return dict(glyphs=glyphs, matrix=matrix or IDENTITY,
                    bbox=union([g['cell'] for g in glyphs]),
                    gradients=gradients or {'g': dict(kind='linear'), 'r': dict(kind='radial')})

    def case(name, body, groups, defs=DEFS):
        return dict(name=name, source='<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96"><defs>'+defs+'</defs>'+body+'</svg>\n', groups=groups)

    cases = []
    cases.append(case('cells-middle', '<text x="64" y="54" font-size="22" text-anchor="middle" xml:space="preserve" fill="url(#g)">  jAVg  </text>', [group(run('  jAVg  ', 64, 54, 22, 'middle'))]))
    gs = run('AV A', 12, 58, 28, font='kern')
    gs[0]['fill'], gs[3]['fill'] = GREEN, GREEN
    cases.append(case('tspan-whole-kern', '<text x="12" y="58" font-size="28" fill="currentColor" color="#20a060">A<tspan fill="url(#g)">V </tspan>A</text>', [group(gs)]))
    gs = run('AVg ', 110, 60, 28, 'end', font='kern')
    # One relative move retains the boundary A/V kern and shifts the full
    # anchored chunk. Nested paint changes do not split a glyph metrics run.
    gs[0]['x'] -= 3
    gs[0]['cell'][0] -= 3
    for g in gs[1:]:
        g['y'] -= 4
        g['cell'][1] -= 4
    for g in gs[2:]: g['fill'] = 'r'
    cases.append(case('relative-kern', '<text x="110" y="60" font-size="28" text-anchor="end" xml:space="preserve" fill="url(#g)">A<tspan dx="3" dy="-4">V<tspan fill="url(#r)">g </tspan></tspan></text>', [group(gs)]))
    a = run('AV', 12, 32, 22)
    b = run('g  ', 112, 72, 28, 'end')
    for g in b: g['fill'] = 'r'
    cases.append(case('chunks-fonts', '<text x="12" y="32" font-size="22" xml:space="preserve" fill="url(#g)">AV<tspan x="112" y="72" font-size="28" text-anchor="end" fill="url(#r)">g  </tspan></text>', [group(a+b)]))
    gs = run('AVWgj ', 10, 54, 24)
    gs[1]['fill'], gs[2]['alpha'], gs[3]['alpha'], gs[4]['fill'] = None, 0., 0., GREEN
    cases.append(case('visibility-overrides', '<text x="10" y="54" font-size="24" xml:space="preserve" fill="url(#g)">A<tspan fill="none">V</tspan><tspan visibility="hidden">W</tspan><tspan opacity="0">g</tspan><tspan display="none" x="900">WWWW</tspan><tspan color="#20a060" fill="currentColor">j</tspan> </text>', [group(gs)]))
    gt = [1, .18, -.15, .8, .08, .08]
    matrix = mul([1, .06, .12, 1, 0, 0], [1, .12, .28, .9, 2, 2])
    cases.append(case('affine-local', '<g transform="matrix(1 .06 .12 1 0 0)"><text x="10" y="52" font-size="24" xml:space="preserve" transform="matrix(1 .12 .28 .9 2 2)" fill="url(#g)">j AV </text></g>', [group(run('j AV ', 10, 52, 24), matrix, {'g': dict(kind='linear', transform=gt)})], '<linearGradient id="g" gradientTransform="matrix(1 .18 -.15 .8 .08 .08)">'+STOPS+'</linearGradient>'))
    gs = run('AVg', 14, 50, 28)
    for g in gs: g['fill'] = 'r'
    cases.append(case('radial-reflection', '<text x="14" y="50" font-size="28" transform="matrix(-.9 .04 .16 1 118 0)" fill="url(#r)">AVg</text>', [group(gs, [-.9, .04, .16, 1, 118, 0])]))
    defs = DEFS+'<style>.host .label .accent{fill:none}.label > .accent{fill:inherit}</style><g transform="translate(900 900)" display="none"><text id="label" class="label" x="4" y="30" xml:space="preserve">AV<tspan class="accent">g </tspan></text><use id="link" href="#label" x="3" y="5" transform="scale(1.1 .9)"/></g>'
    b = run('AVg ', 4, 30, 16)
    for g in b: g['fill'] = 'r'
    cases.append(case('use-chain', '<g class="host"><use id="first" href="#link" x="6" y="8" font-size="22" fill="url(#g)"/><use id="second" href="#link" x="62" y="45" font-size="16" fill="url(#r)"/></g>', [group(run('AVg ', 4, 30, 22), [1.1, 0, 0, .9, 9.3, 12.5]), group(b, [1.1, 0, 0, .9, 65.3, 49.5])], defs))
    gs = run('AVg ', 12, 58, 28)
    spanbox = union([g['cell'] for g in gs[1:3]])
    for g in gs:
        g['clip'] = group(gs)['bbox']
    for g in gs[1:3]: g['clip'] = [spanbox[0], spanbox[1], spanbox[2]*.65, spanbox[3]]
    clips = '<clipPath id="full" clipPathUnits="objectBoundingBox"><rect width="1" height="1"/></clipPath><clipPath id="span" clipPathUnits="objectBoundingBox"><rect width=".65" height="1"/></clipPath>'
    cases.append(case('nested-clips', '<text x="12" y="58" font-size="28" xml:space="preserve" fill="url(#g)" clip-path="url(#full)">A<tspan clip-path="url(#span)">Vg</tspan> </text>', [group(gs)], DEFS+clips))
    gs = run('AVg', 16, 58, 30)
    for i, g in enumerate(gs): g['alpha'] = .6*.8*(.3 if i == 1 else .65)*.7
    cases.append(case('template-alpha', '<g opacity=".6"><text x="16" y="58" font-size="30" opacity=".8" fill-opacity=".65" fill="url(#g)">A<tspan fill-opacity=".3">V</tspan>g</text></g>', [group(gs, gradients={'g': dict(kind='linear', colors=[[1, .25, 0], BLUE])})], '<linearGradient id="base" color="green"><stop stop-color="currentColor" stop-opacity=".7"/><stop offset="1" stop-color="blue" stop-opacity=".7"/></linearGradient><linearGradient id="g" href="#base" color="rgb(100%,25%,0%)"/>'))
    gs = run('A', 20, 55, 30, font='zero')
    gs[0]['stroke'] = True
    cases.append(case('zero-width', '<text x="20" y="55" font-size="30" fill="url(#g)" stroke="black" stroke-width="1">A</text>', [group(gs)]))

    oracle = []
    for c in cases:
        files[c['name']+'.svg'] = c['source'].encode()
        expected = []
        for gr in c['groups']:
            box = gr['bbox']
            bm = [box[2], 0, 0, box[3], box[0], box[1]]
            for g in gr['glyphs']:
                # Whitespace has metric geometry but no rendered glyph object.
                if g['rune'] == ' ': continue
                record = dict(rune=g['rune'], bbox=box, cell=g['cell'], alpha=g['alpha'])
                fill = g['fill']
                if isinstance(fill, str) and box[2] and box[3]:
                    spec = gr['gradients'][fill]
                    primitive = IDENTITY if spec['kind'] == 'linear' else [.5, 0, 0, .5, .5, .5]
                    frame = mul(gr['matrix'], mul(bm, mul(spec.get('transform', IDENTITY), primitive)))
                    record.update(kind=spec['kind'], matrix=mul(NORMALIZE, frame))
                    # template alpha includes graphics opacity only in raster;
                    # native stop alpha excludes parent and text opacity.
                    record['stop_alpha'] = (.3 if g['rune']=='V' else .65)*.7 if c['name']=='template-alpha' else 1.
                    record['colors'] = spec.get('colors', [RED, BLUE])
                else:
                    record['solid'] = fill if isinstance(fill, list) else [0, 0, 0, 0]
                expected.append(record)
        oracle.append(dict(name=c['name'], font='kern' if c['name'] in ['tspan-whole-kern', 'relative-kern'] else 'zero' if c['name']=='zero-width' else 'regular', glyphs=expected))

    def render(c, ss, motion=None):
        surf = cairo.ImageSurface(cairo.FORMAT_ARGB32, 128*ss, 96*ss)
        ctx = cairo.Context(surf)
        ctx.scale(ss, ss)
        if motion: ctx.transform(cairo.Matrix(*motion))
        for gr in c['groups']:
            ctx.save()
            ctx.transform(cairo.Matrix(*gr['matrix']))
            x, y, w, h = gr['bbox']
            for g in gr['glyphs']:
                if g['rune']==' ': continue
                ctx.save()
                if 'clip' in g:
                    ctx.rectangle(*g['clip'])
                    ctx.clip()
                f = fonts[g['font']]
                glyphset, name = f.getGlyphSet(), f.getBestCmap()[ord(g['rune'])]
                ctx.save()
                ctx.translate(g['x'], g['y'])
                scale = g['size']/f['head'].unitsPerEm
                ctx.scale(scale, -scale)
                glyphset[name].draw(CairoPen(glyphset, ctx))
                ctx.restore()
                fill = g['fill']
                if isinstance(fill, str) and w and h:
                    spec = gr['gradients'][fill]
                    pattern = cairo.LinearGradient(0, 0, 1, 0) if spec['kind']=='linear' else cairo.RadialGradient(.5, .5, 0, .5, .5, .5)
                    frame = mul([w, 0, 0, h, x, y], spec.get('transform', IDENTITY))
                    inv = cairo.Matrix(*frame)
                    inv.invert()
                    pattern.set_matrix(inv)
                    pattern.set_extend(cairo.EXTEND_PAD)
                    colors = spec.get('colors', [RED, BLUE])
                    for offset, color in zip([0, 1], colors): pattern.add_color_stop_rgba(offset, *color, g['alpha'])
                    ctx.set_source(pattern)
                elif isinstance(fill, list): ctx.set_source_rgba(*fill, g['alpha'])
                else: ctx.set_source_rgba(0, 0, 0, 0)
                if g.get('stroke'):
                    ctx.fill_preserve()
                    ctx.set_source_rgb(0, 0, 0)
                    ctx.set_line_width(1)
                    ctx.stroke()
                else: ctx.fill()
                ctx.restore()
            ctx.restore()
        buffer = io.BytesIO()
        surf.write_to_png(buffer)
        im = Image.open(io.BytesIO(buffer.getvalue())).convert('RGBA')
        if ss > 1: im = im.convert('RGBa').resize((128, 96), Image.Resampling.BOX).convert('RGBA')
        result = io.BytesIO()
        im.save(result, format='PNG')
        return result.getvalue()

    for c in cases:
        for ss in [1, 4]: files[f'{c["name"]}-cairo-ss{ss}.png'] = render(c, ss)
    for tick in [0, 20, 50, 80, 100]:
        t = tick/100
        motion = [1-.18*t, .10*t, -.12*t, 1+.05*t, 10*t, -3*t]
        for ss in [1, 4]: files[f'animated-{tick:03d}-cairo-ss{ss}.png'] = render(next(c for c in cases if c['name']=='affine-local'), ss, motion)
    files['oracle.json'] = (json.dumps(dict(cases=oracle), indent=2)+'\n').encode()
    records = {name:sha(data) for name,data in sorted(files.items())}
    provenance = dict(generator='tests/reference/svg_text_bbox_gradients.py', generator_sha256=sha(Path(__file__).read_bytes()),
        versions=dict(Python=platform.python_version(), Cairo=cairo.cairo_version_string(), fontTools=fontTools.__version__, Pillow=pillow_version),
        font_sha256={'Go-Regular.ttf':sha(regular_bytes), 'text-kern.ttf':sha(kern_bytes), 'zero-advance.ttf':sha(zero_bytes)},
        pinned_manim=dict(version='0.22.1', commit='16ea443e2a5f1d79db0a721161411ce916d6e733', claim='source context only; independent SVG evidence, no new Manim parity claim'),
        primary_spec=['https://www.w3.org/TR/SVG11/text.html#ObjectBoundingBoxUnitsTextObjects', 'https://www.w3.org/TR/SVG2/coords.html#BoundingBoxes', 'https://www.w3.org/TR/SVG2/pservers.html#LinearGradientElementGradientUnitsAttribute'],
        oracle='Original fontTools hmtx/kern/hhea + original glyph outline CairoPen; manually authored scope/affine/paint state, direct Cairo patterns. No SVG text parser, Go values, exports or pixels consumed.',
        text_policy='Entire enclosing text full glyph-cell bbox even for tspan fills. Paint/visibility/opacity do not shrink geometry; display:none excluded. Separate existing tspan clip geometry retained.',
        animation='Affine-local source, linearly interpolated affine geometry and gradient frame at 0,.2,.5,.8,1. Go scene sampling is checked independently, including interiors and camera inversion.',
        limitations='Bounded horizontal text; original native limits unchanged. No nested viewport/container symbol bbox claim. Font shaping/bidi not added. Direct Cairo avoids CairoSVG text-bbox/parser behavior.',
        files_sha256=records)
    if args.verify:
        for name,data in files.items(): assert (OUT/name).read_bytes()==data, name
        saved = json.loads((OUT/'provenance.json').read_text())
        assert saved==provenance, 'provenance changed'
        print(f'PASS: {len(files)} independent files byte-identical; generator and provenance hashes match')
    else:
        OUT.mkdir(parents=True,exist_ok=True)
        for name,data in files.items():
            with (OUT/name).open('xb') as stream: stream.write(data)
        with (OUT/'provenance.json').open('x') as stream: stream.write(json.dumps(provenance,indent=2)+'\n')
        print(f'Wrote {len(files)} independent files and provenance')


if __name__ == '__main__': main()
