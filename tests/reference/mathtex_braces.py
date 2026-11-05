#!/usr/bin/env python3
"""Development-only fontTools/manual finite brace layout + Cairo reference.

Reads original Go fonts and pinned Manim source, never gomath output. This is
an independent implementation of the documented em policy, not TeX/Manim
layout parity. Original fonts retain LICENSES/go-fonts.txt. Manim CE's MIT
license is retained in the reference manifest; no Manim geometry is copied.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import subprocess  # development-only: read-only pin/cleanliness check
import cairocffi
import cairosvg
import fontTools
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/mathtex/braces'
PIN = '16ea443e2a5f1d79db0a721161411ce916d6e733'
FILES = ['Go-Regular.ttf', 'Go-Bold.ttf', 'Go-Italic.ttf', 'Go-Bold-Italic.ttf']


def sha(data):
    return hashlib.sha256(data).hexdigest()


def moved(b, x, y):
    return [b[0]+x, b[1]+y, b[2]+x, b[3]+y] if b else None


def union(boxes):
    b = [b for b in boxes if b]
    return [min(v[0] for v in b), min(v[1] for v in b),
            max(v[2] for v in b), max(v[3] for v in b)] if b else None


def box(source='', width=0, height=0, depth=0, bbox=None, svg=''):
    return dict(source=source, width=width, height=height, depth=depth, bbox=bbox, svg=svg)


def translated(svg, x, y):
    return f'<g transform="translate({x:.17g} {y:.17g})">{svg}</g>'


def scaled(b, ratio):
    return box(b['source'], b['width']*ratio, b['height']*ratio, b['depth']*ratio,
               [v*ratio for v in b['bbox']] if b['bbox'] else None,
               f'<g transform="scale({ratio:.17g})">{b["svg"]}</g>')


def row(*items):
    x = 0
    boxes, fragments = [], []
    for i, b in enumerate(items):
        if i:
            x += .04
        boxes.append(moved(b['bbox'], x, 0))
        fragments.append(translated(b['svg'], x, 0))
        x += b['width']
    return box(''.join(b['source'] for b in items), x, max(b['height'] for b in items),
               max(b['depth'] for b in items), union(boxes), ''.join(fragments))


def ribbon(width, bold):
    h, t = .28, .060 if bold else .045
    # Analytically authored ribbon, in em. Independent from Go path export.
    controls = [(0,h/2,.04,h/2,.1,h/2),(.36,h/2,.46,h/2,.5,h),
                (.54,h/2,.64,h/2,.9,h/2),(.96,h/2,1,h/2,1,0),
                (1,h/2-t,.96,h/2-t,.9,h/2-t),(.64,h/2-t,.54,h/2-t,.5,h-t),
                (.46,h/2-t,.36,h/2-t,.1,h/2-t),(.04,h/2-t,0,h/2-t,0,0)]
    return 'M0 0 ' + ' '.join('C'+' '.join(f'{v*width if i%2==0 else v:.17g}'
                               for i,v in enumerate(c)) for c in controls) + ' Z'


def brace(body, command='overbrace', sup=None, sub=None, style=0, color='00ff00'):
    w = max(body['width'], .6)
    top, bottom = body['height'], -body['depth']
    x = (body['width']-w)/2
    is_under = command == 'underbrace'
    y = bottom-.1 if is_under else top+.1
    mb = [x,y-.28,x+w,y] if is_under else [x,y,x+w,y+.28]
    mark = f'<path fill="#{color}" transform="translate({x:.17g} {y:.17g}) scale(1 {-1 if is_under else 1})" d="{ribbon(w,style&1)}"/>'
    left = min(0, mb[0], body['bbox'][0] if body['bbox'] else 0)
    right = max(body['width'], mb[2], body['bbox'][2] if body['bbox'] else 0)
    svg = translated(body['svg']+mark, -left, 0)
    bbox = moved(union([body['bbox'],mb]), -left, 0)
    width, height, depth = right-left, max(top,mb[3]), max(-bottom,-mb[1])
    source = '\\'+command+'{'+body['source']+'}'
    markbox = moved(mb,-left,0)
    if sup is not None or sub is not None:
        spans, fragments = [bbox], [svg]
        lo, hi = 0, width
        baseh, based = height, depth
        for i, label in enumerate([sup,sub]):
            if label is None:
                continue
            source += ('^' if i==0 else '_')+'{'+label['source']+'}'
            label = scaled(label,.65)
            center = (label['bbox'][0]+label['bbox'][2])/2 if label['bbox'] else label['width']/2
            dx = width/2-center
            dy = baseh+.12+label['depth'] if i==0 else -based-.12-label['height']
            lo,hi = min(lo,dx),max(hi,dx+label['width'])
            height,depth = max(height,dy+label['height']),max(depth,-dy+label['depth'])
            spans.append(moved(label['bbox'],dx,dy));fragments.append(translated(label['svg'],dx,dy))
        svg = translated(''.join(fragments),-lo,0)
        bbox = moved(union(spans),-lo,0)
        markbox = moved(markbox,-lo,0)
        width = hi-lo
    result = box(source,width,height,depth,bbox,svg)
    result['mark_bbox'] = markbox
    return result


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--gofont-dir', required=True, type=Path)
    ap.add_argument('--manim-dir', required=True, type=Path)
    args = ap.parse_args()
    pin = subprocess.check_output(['git','-C',str(args.manim_dir),'rev-parse','HEAD'],text=True).strip()
    clean = subprocess.check_output(['git','-C',str(args.manim_dir),'status','--porcelain'],text=True)
    if pin != PIN or clean:
        raise ValueError('Manim source must be clean at the exact requested pin')
    source_files = ['manim/mobject/text/tex_mobject.py','manim/utils/tex_file_writing.py','manim/animation/transform.py','LICENSE']
    manim = dict(version='0.22.1',commit=pin,clean=True,
                 files_sha256={f:sha((args.manim_dir/f).read_bytes()) for f in source_files},
                 license=(args.manim_dir/'LICENSE').read_text(),executed=False,
                 note='Source-pinned API/TeX pipeline inspection only; no TeX/Manim pixel parity claim.')
    OUT.mkdir(parents=True,exist_ok=True)
    fonts, faces = [], []
    for file in FILES:
        path = args.gofont_dir/'ttfs'/file
        f=TTFont(path);gs=f.getGlyphSet();upem=f['head'].unitsPerEm;face={}
        fonts.append(dict(file=file,sha256=sha(path.read_bytes()),units_per_em=upem))
        for rune in 'ngWi+x':
            name=f.getBestCmap()[ord(rune)];pen=SVGPathPen(gs);bp=BoundsPen(gs)
            gs[name].draw(pen);gs[name].draw(bp)
            origin=(f['hmtx'][name][1]-f['glyf'][name].xMin)/upem
            b=[bp.bounds[0]/upem+origin,bp.bounds[1]/upem,bp.bounds[2]/upem+origin,bp.bounds[3]/upem]
            left=max(0,-b[0]);width=max(f['hmtx'][name][0]/upem,b[2])+left
            face[rune]=dict(width=width,height=max(0,b[3]),depth=max(0,-b[1]),bbox=moved(b,left,0),
                svg=f'<path transform="translate({origin+left:.17g} 0) scale({1/upem:.17g})" d="{pen.getCommands()}"/>')
        faces.append(face)
    def glyph(rune, style=0, color='ffffff'):
        g=faces[style][rune]
        return box(r'\textcolor[HTML]{'+color+'}{'+rune+'}',g['width'],g['height'],g['depth'],g['bbox'],
                   f'<g fill="#{color}">{g["svg"]}</g>')
    records=[]
    for style in range(4):
        body=row(glyph('n',style,'ff0000'),glyph('+',style,'00ff00'),glyph('g',style,'0000ff'))
        b=brace(body,sup=glyph('W',style,'ffff00'),sub=glyph('i',style,'ff00ff'),style=style)
        b['source']=r'\color{green}'+b['source']
        records.append((f'labels-style-{style}',style,b))
    inner=brace(glyph('g',0,'ff0000'),'underbrace',sub=glyph('n',0,'0000ff'))
    nested=brace(inner,sup=glyph('i',0,'ffff00'));nested['source']=r'\color{green}'+nested['source']
    records.append(('nested',0,nested))
    zero=brace(box(),sup=glyph('W'));zero['source']=r'\color{green}'+zero['source'];records.append(('zero',0,zero))
    g=faces[0]['g']
    hidden=box(r'\phantom{g}',g['width'],g['height'],g['depth'])
    phantom=brace(hidden,'underbrace',sub=glyph('n',0,'ff0000'));phantom['source']=r'\color{green}'+phantom['source'];records.append(('phantom',0,phantom))
    # A brace nested inside the upper label scales every local em length.
    label=brace(glyph('g',0,'ff0000'),'underbrace',sub=glyph('n',0,'0000ff'))
    script=brace(glyph('W'),sup=label);script['source']=r'\color{green}'+script['source'];records.append(('script-brace',0,script))
    def save(name,svg):
        data=('<svg xmlns="http://www.w3.org/2000/svg" width="768" height="768"><rect width="768" height="768" fill="#000000"/><g transform="translate(64 384) scale(128 -128)">'+svg+'</g></svg>\n').encode()
        (OUT/(name+'.svg')).write_bytes(data)
        (OUT/(name+'.png')).write_bytes(cairosvg.svg2png(bytestring=data))
    results=[]
    for name,style,b in records:
        save(name,b['svg'])
        results.append(dict(name=name,style=style,**{k:v for k,v in b.items() if k!='svg'}))
    for time,label in [(.35,'035'),(.7,'070')]:
        save('transform-'+label,f'<g transform="translate({.2*time:.17g} {.3*time:.17g}) scale({1-.2*time:.17g} {1+.1*time:.17g})">{records[0][2]["svg"]}</g>')
    metadata=dict(provenance=__doc__,generator_sha256=sha(Path(__file__).read_bytes()),manim=manim,fonts=fonts,
        versions=dict(Python=platform.python_version(),fontTools=fontTools.__version__,CairoSVG=cairosvg.__version__,Cairo=cairocffi.cairo_version_string()),
        coordinates=dict(width=768,height=768,ppem=128,origin=[64,384]),
        policy=dict(minimum_width=.6,height=.28,thickness=.045,bold_thickness=.060,body_gap=.1,label_gap=.12,label_scale=.65),
        records=results,transform_times=[.35,.7],
        files_sha256={f.name:sha(f.read_bytes()) for f in sorted(OUT.iterdir()) if f.suffix in ['.svg','.png']})
    (OUT/'reference.json').write_text(json.dumps(metadata,indent=2)+'\n')
    print('wrote 8 independent font/manual/Cairo static cases and 2 affine intermediate frames')


if __name__=='__main__':
    main()
