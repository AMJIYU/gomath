#!/usr/bin/env python3
"""Development-only fontTools / manual operand / CairoSVG reference.

Independent source outlines and analytical boxes under the documented finite
policy. No Go output read; no full TeX/Manim layout or animation oracle claim.
The Go fonts retain the Bigelow & Holmes notices in LICENSES/go-fonts.txt.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import cairocffi
import cairosvg
import fontTools
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

FILES = ['Go-Regular.ttf','Go-Bold.ttf','Go-Italic.ttf','Go-Bold-Italic.ttf']

def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--gofont-dir', type=Path, required=True)
    args = ap.parse_args()
    out = Path(__file__).resolve().parents[1] / 'data/mathtex/radical_boxes'
    out.mkdir(parents=True, exist_ok=True)
    records, fonts = [], []
    for style, filename in enumerate(FILES):
        source = args.gofont_dir/'ttfs'/filename
        font = TTFont(source)
        glyphs, upem = font.getGlyphSet(), font['head'].unitsPerEm
        fonts.append(dict(file=filename, sha256=hashlib.sha256(source.read_bytes()).hexdigest(), units_per_em=upem))
        operands = {}
        for letter in 'ngAB':
            name = font.getBestCmap()[ord(letter)]
            p, b = SVGPathPen(glyphs), BoundsPen(glyphs)
            glyphs[name].draw(p); glyphs[name].draw(b)
            # sfnt normalizes x to hmtx LSB rather than arbitrary glyf.xMin.
            origin = (font['hmtx'][name][1]-font['glyf'][name].xMin)/upem
            bbox = [b.bounds[0]/upem+origin,b.bounds[1]/upem,b.bounds[2]/upem+origin,b.bounds[3]/upem]
            advance = font['hmtx'][name][0]/upem
            left = max(0,-bbox[0])
            operands[letter] = dict(path=p.getCommands(),bbox=bbox,origin=origin,left=left,
                width=max(advance,bbox[2])+left,height=max(0,bbox[3]),depth=max(0,-bbox[1]))
        for kind in ['indexed','boxed','phantom','hphantom','vphantom']:
            paths, placements = [], []
            def glyph(letter, ratio, x, y, color, role):
                g=operands[letter]
                dx=x+(g['left']+g['origin'])*ratio
                paths.append(f'<path fill="#{color}" transform="translate({dx:.17g} {y:.17g}) scale({ratio/upem:.17g})" d="{g["path"]}"/>')
                placements.append(dict(role=role,letter=letter,ratio=ratio,color=color,x=x,baseline=y,
                    bbox=[x+(g['left']+g['bbox'][0])*ratio,y+g['bbox'][1]*ratio,
                          x+(g['left']+g['bbox'][2])*ratio,y+g['bbox'][3]*ratio]))
            g=operands['g']
            if kind=='indexed':
                n=operands['n'];iw=n['width']*.5;idepth=n['depth']*.5
                start=.0225+max(0,iw-.18);top=max(.4,g['height']+.1)
                ix=start+.18-iw;iy=max(.28,.55*top)+idepth;left=start+.48
                width=left+g['width']+.05+.0225;height=max(top+.0225,iy+n['height']*.5);depth=g['depth']+.0825
                points=[(start,.12),(start+.12,.18),(start+.24,-g['depth']-.06),(start+.43,top),(left+g['width']+.05,top)]
                d='M'+' L'.join(f'{x:.17g} {y:.17g}' for x,y in points)
                paths.append(f'<path d="{d}" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linejoin="round" stroke-linecap="round"/>')
                glyph('g',1,left,0,'0000ff','Radicand');glyph('n',.5,ix,iy,'ff0000','RootIndex')
                formula=r'\color{green}\sqrt[\textcolor{red}{n}]{\textcolor{blue}{g}}'
            elif kind=='boxed':
                width=g['width']+.49;height=g['height']+.245;depth=g['depth']+.245
                paths.append(f'<rect x=".0225" y="{-depth+.0225:.17g}" width="{width-.045:.17g}" height="{height+depth-.045:.17g}" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linejoin="round"/>')
                glyph('g',1,.245,0,'0000ff','BoxedBody')
                formula=r'\color{green}\boxed{\textcolor{blue}{g}}'
            else:
                a,b=operands['A'],operands['B']
                pw=0 if kind=='vphantom' else g['width']
                ph=0 if kind=='hphantom' else g['height'];pd=0 if kind=='hphantom' else g['depth']
                width=a['width']+.08+pw+b['width'];height=max(a['height'],ph,b['height']);depth=max(a['depth'],pd,b['depth'])
                glyph('A',1,0,0,'ff0000','A');glyph('B',1,a['width']+.08+pw,0,'0000ff','B')
                formula=r'\textcolor{red}{A}'+chr(92)+kind+r'{g}\textcolor{blue}{B}'
            name=f'{kind}-{style}'
            svg=('<svg xmlns="http://www.w3.org/2000/svg" width="512" height="384" viewBox="0 0 512 384">'
                 '<rect width="512" height="384" fill="#000000"/><g transform="translate(64 192) scale(128 -128)">'+''.join(paths)+'</g></svg>\n').encode()
            png=cairosvg.svg2png(bytestring=svg)
            (out/(name+'.svg')).write_bytes(svg);(out/(name+'.png')).write_bytes(png)
            records.append(dict(name=name,kind=kind,style=style,source=formula,width=width,height=height,depth=depth,
                placements=placements,svg_sha256=hashlib.sha256(svg).hexdigest(),png_sha256=hashlib.sha256(png).hexdigest()))
    manifest=dict(provenance=__doc__,generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        versions=dict(Python=platform.python_version(),fontTools=fontTools.__version__,CairoSVG=cairosvg.__version__,Cairo=cairocffi.cairo_version_string()),
        fonts=fonts,coordinates=dict(width=512,height=384,ppem=128,origin=[64,192]),
        policy=dict(box_padding=.2,stroke=.045,index_ratio=.5,index_right=.18,index_bottom_min=.28,index_top_ratio=.55),records=records)
    (out/'reference.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print('Wrote 20 analytical source-font SVG/Cairo PNG references, four real faces.')
if __name__=='__main__':
    main()
