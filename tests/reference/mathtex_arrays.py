#!/usr/bin/env python3
"""Development-only independent fontTools/manual operand/Cairo array references.
No gomath output or TeX layout read. This finite policy is not full TeX/amsmath.
Original Go fonts keep their notices in LICENSES/go-fonts.txt. Runtime/tests do
not start this program or depend on its Python/native development libraries.
"""
import argparse
import hashlib
import json
import math
from pathlib import Path
import platform
import cairocffi
import cairosvg
import fontTools
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont

FILES=['Go-Regular.ttf','Go-Bold.ttf','Go-Italic.ttf','Go-Bold-Italic.ttf']
ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'tests/data/mathtex/arrays'

def combine(boxes):
 b=[x for x in boxes if x is not None]
 return [min(x[0] for x in b),min(x[1] for x in b),max(x[2] for x in b),max(x[3] for x in b)] if b else None

def move(b,x,y):return [b[0]+x,b[1]+y,b[2]+x,b[3]+y] if b else None

def main():
 ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--gofont-dir',required=True,type=Path);args=ap.parse_args();OUT.mkdir(parents=True,exist_ok=True)
 records=[];fonts=[];baseline_svg=None
 for style,filename in enumerate(FILES):
  file=args.gofont_dir/'ttfs'/filename;font=TTFont(file);glyphset=font.getGlyphSet();upem=font['head'].unitsPerEm;glyphs={}
  fonts.append(dict(file=filename,sha256=hashlib.sha256(file.read_bytes()).hexdigest(),units_per_em=upem))
  for letter in 'ngWiA':
   name=font.getBestCmap()[ord(letter)];pen,bounds=SVGPathPen(glyphset),BoundsPen(glyphset);glyphset[name].draw(pen);glyphset[name].draw(bounds)
   origin=(font['hmtx'][name][1]-font['glyf'][name].xMin)/upem;b=[bounds.bounds[0]/upem+origin,bounds.bounds[1]/upem,bounds.bounds[2]/upem+origin,bounds.bounds[3]/upem];left=max(0,-b[0]);advance=font['hmtx'][name][0]/upem
   glyphs[letter]=dict(path=pen.getCommands(),origin=origin,left=left,bbox=b,width=max(advance,b[2])+left,height=max(0,b[3]),depth=max(0,-b[1]))
  def glyph(letter,color):
   g=glyphs[letter];frag=f'<path fill="#{color}" transform="translate({g["origin"]+g["left"]:.17g} 0) scale({1/upem:.17g})" d="{g["path"]}"/>'
   return dict(width=g['width'],height=g['height'],depth=g['depth'],fragments=[frag],bbox=move(g['bbox'],g['left'],0),source=r'\textcolor[HTML]{'+color+'}{'+letter+'}')
  def phantom(letter,kind='phantom'):
   g=glyphs[letter];return dict(width=0 if kind=='vphantom' else g['width'],height=0 if kind=='hphantom' else g['height'],depth=0 if kind=='hphantom' else g['depth'],fragments=[],bbox=None,source='\\'+kind+'{'+letter+'}')
  def fraction():
   num,den=glyph('n','ff0000'),glyph('g','0000ff');ratio=.8;width=max(num['width'],den['width'])*ratio+.25;ny=.32+num['depth']*ratio;dy=-den['height']*ratio
   nx,dx=(width-num['width']*ratio)/2,(width-den['width']*ratio)/2
   fragments=[f'<g transform="translate({nx:.17g} {ny:.17g}) scale(.8)">'+''.join(num['fragments'])+'</g>',f'<g transform="translate({dx:.17g} {dy:.17g}) scale(.8)">'+''.join(den['fragments'])+'</g>',f'<path d="M0 .16H{width:.17g}" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linecap="round"/>']
   nb=move([v*ratio for v in num['bbox']],nx,ny);db=move([v*ratio for v in den['bbox']],dx,dy)
   return dict(width=width,height=max(ny+num['height']*ratio,.1825),depth=max(-dy+den['depth']*ratio,0),fragments=fragments,bbox=combine([nb,db,[0,.16,width,.16]]),source=r'\frac{\textcolor{red}{n}}{\textcolor{blue}{g}}')
  grids=[(f'lcr-rules-{style}','|l|c|r|',[[glyph('n','ff0000'),glyph('g','0000ff'),glyph('W','8040c0')],[glyph('W','8040c0'),glyph('i','ff8000'),glyph('g','0000ff')],[glyph('i','ff8000'),glyph('W','ff0000'),glyph('n','8040c0')]],[0,1,3])]
  if style==0:
   grids += [('plain-lcr','lcr',[[glyph('n','ff0000'),glyph('g','0000ff'),glyph('W','8040c0')],[glyph('W','8040c0'),glyph('i','ff8000'),glyph('g','0000ff')]],[]),('fraction-phantom','|l|r|',[[fraction(),glyph('W','8040c0')],[phantom('W'),glyph('g','0000ff')],[glyph('i','ff8000'),phantom('g','hphantom')]],[0,1,2,3]),('empty-phantom','|l|c|r|',[[phantom('W'),phantom('g','hphantom'),phantom('n','vphantom')],[phantom('g','hphantom'),phantom('n','vphantom'),phantom('W')]],[0,1,2])]
  for name,spec,rows,hline in grids:
   aligns=[c for c in spec if c in 'lcr'];vertical=[False]*(len(aligns)+1);j=0
   for char in spec:
    if char=='|':vertical[j]=True
    else:j+=1
   widths=[max(.25,*(r[j]['width'] for r in rows)) for j in range(len(aligns))];ascents=[max(.7,*(c['height'] for c in r)) for r in rows];depths=[max(.2,*(c['depth'] for c in r)) for r in rows]
   starts=[];vlines=[];cursor=0
   for j,w in enumerate(widths):
    if vertical[j]:vlines.append((j,cursor+.0225));cursor+=.045
    starts.append(cursor+.25);cursor+=w+.5
   if vertical[-1]:vlines.append((len(aligns),cursor+.0225));cursor+=.045
   width=cursor;total=sum(ascents)+sum(depths)+.3*(len(rows)-1)+.3+.045*len(hline);top=.16+total/2;depth=total-top
   rules=[dict(axis='V',index=j,start=[x,top],end=[x,-depth]) for j,x in vlines];hy={};y=top
   if 0 in hline:hy[0]=y-.0225;y-=.045
   y-=.15;cells=[];fragments=[];lines=[]
   for i,row in enumerate(rows):
    y-=ascents[i]
    for j,c in enumerate(row):
     dx=0 if aligns[j]=='l' else widths[j]-c['width'] if aligns[j]=='r' else (widths[j]-c['width'])/2;x=starts[j]+dx
     cells.append(dict(row=i,column=j,x=x,baseline=y,width=c['width'],height=c['height'],depth=c['depth'],bbox=move(c['bbox'],x,y)))
     fragments.append(f'<g transform="translate({x:.17g} {y:.17g})">'+''.join(c['fragments'])+'</g>')
    y-=depths[i]
    if i+1<len(rows):
     if i+1 in hline:hy[i+1]=y-.15-.0225;y-=.045
     y-=.3
   y-=.15
   if len(rows) in hline:hy[len(rows)]=y-.0225
   for i,yy in sorted(hy.items()):rules.append(dict(axis='H',index=i,start=[0,yy],end=[width,yy]))
   for rule in rules:
    x0,y0=rule['start'];x1,y1=rule['end'];lines.append(f'<path d="M{x0:.17g} {y0:.17g}L{x1:.17g} {y1:.17g}" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linecap="butt"/>')
   body=[]
   for i,row in enumerate(rows):
    body.append((r'\hline ' if i in hline else '')+'&'.join(c['source'] for c in row))
   formula=r'\color{green}\begin{array}{'+spec+'}'+r'\\'.join(body)+(r'\\\hline' if len(rows) in hline else '')+r'\end{array}'
   content=''.join(lines+fragments)
   svg=('<svg xmlns="http://www.w3.org/2000/svg" width="768" height="768" viewBox="0 0 768 768"><rect width="768" height="768" fill="#000000"/><g transform="translate(64 384) scale(128 -128)">'+content+'</g></svg>\n').encode();png=cairosvg.svg2png(bytestring=svg)
   (OUT/f'{name}.svg').write_bytes(svg);(OUT/f'{name}.png').write_bytes(png)
   records.append(dict(name=name,style=style,source=formula,rows=len(rows),columns=len(aligns),width=width,height=top,depth=depth,cells=cells,rules=rules,svg_sha256=hashlib.sha256(svg).hexdigest(),png_sha256=hashlib.sha256(png).hexdigest()))
   if name=='lcr-rules-0':baseline_svg=content
 # Independent affine morph of the complete first array. Native geometry is
 # interpolated between original and an affine copy; stroke widths stay fixed.
 for time,label in [(.35,'035'),(.7,'070')]:
  sx,sy=1-.2*time,1+.1*time;tx,ty=.2*time,.3*time
  # Transform geometry individually; counter-scale SVG stroke width through
  # vector-effect is not supported by CairoSVG. Map line endpoints explicitly.
  import re
  parts=[]
  for elem in re.findall(r'<path d="M([^"]+)" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linecap="butt"/>',baseline_svg):
   nums=[float(v) for v in elem.replace('L',' ').split()];x0,y0,x1,y1=nums;parts.append(f'<path d="M{x0*sx+tx:.17g} {y0*sy+ty:.17g}L{x1*sx+tx:.17g} {y1*sy+ty:.17g}" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linecap="butt"/>')
  body=re.sub(r'<path d="M[^"]+" fill="none" stroke="#00ff00" stroke-width=".045" stroke-linecap="butt"/>','',baseline_svg)
  parts.append(f'<g transform="translate({tx:.17g} {ty:.17g}) scale({sx:.17g} {sy:.17g})">'+body+'</g>')
  svg=('<svg xmlns="http://www.w3.org/2000/svg" width="768" height="768"><rect width="768" height="768" fill="#000000"/><g transform="translate(64 384) scale(128 -128)">'+''.join(parts)+'</g></svg>\n').encode();(OUT/f'morph-{label}.svg').write_bytes(svg);(OUT/f'morph-{label}.png').write_bytes(cairosvg.svg2png(bytestring=svg))
 metadata=dict(provenance=__doc__,generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),versions=dict(Python=platform.python_version(),fontTools=fontTools.__version__,CairoSVG=cairosvg.__version__,Cairo=cairocffi.cairo_version_string()),fonts=fonts,coordinates=dict(width=768,height=768,ppem=128,origin=[64,384]),policy=dict(column_padding=.25,row_padding=.15,row_gap=.3,strut_ascent=.7,strut_depth=.2,axis=.16,rule_thickness=.045,rule_cap='butt',rule_color='00ff00',styles='real regular/bold/italic/bold-italic',display='inherited and cell-local'),records=records,morph_times=[.35,.7],files_sha256={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in sorted(OUT.iterdir()) if f.suffix in ['.svg','.png']},limitations='Original source-font quadratic contours and analytically authored operands, not a TeX/amsmath or Manim layout oracle. Native sfnt extrema may differ by one design unit. Morph uses linear rate, affine geometry and fixed-width rules. No independent partial-glyph Write AA claim.')
 (OUT/'reference.json').write_text(json.dumps(metadata,indent=2)+'\n');print('wrote',len(records),'font/operand/Cairo arrays and2 independent affine intermediate frames')
if __name__=='__main__':main()
